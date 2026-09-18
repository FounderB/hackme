package hunt

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"hackme/internal/gitutil"
)

// Local FindString allowlists (same file as exec sinks) for CodeQL command-injection.
var (
	reRepoHTTPS   = regexp.MustCompile(`^https://[A-Za-z0-9._~:/?#\[\]@!$&'()*+,;=%-]+$`)
	reRepoSSH     = regexp.MustCompile(`^git@[A-Za-z0-9.-]+:[A-Za-z0-9._~/-]+\.git$`)
	reRepoRef     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,200}$`)
	reRepoAbsPath = regexp.MustCompile(`^(/[A-Za-z0-9._+-]+)+$`)
)

// RepoPinRequest pins a local path or git clone at ref.
type RepoPinRequest struct {
	Path   string `json:"path,omitempty"`
	GitURL string `json:"git_url,omitempty"`
	Ref    string `json:"ref,omitempty"`
}

// RepoPinResult is a pinned repo root for Hunt inventory/build.
type RepoPinResult struct {
	Path      string `json:"path"`
	GitURL    string `json:"git_url,omitempty"`
	Ref       string `json:"ref,omitempty"`
	CommitSHA string `json:"commit_sha,omitempty"`
	PinnedAt  int64  `json:"pinned_at"`
}

// PinRepo resolves a local directory or shallow git clone for Hunt.
func PinRepo(ctx context.Context, repoRoot string, req RepoPinRequest) (*RepoPinResult, error) {
	if repoRoot == "" {
		repoRoot = RepoRoot()
	}
	gitURL := strings.TrimSpace(req.GitURL)
	ref := strings.TrimSpace(req.Ref)
	if ref == "" {
		ref = "main"
	}
	now := time.Now().Unix()
	if gitURL != "" {
		gitURL, err := ValidateGitURL(gitURL)
		if err != nil {
			return nil, err
		}
		ref, err = ValidateGitRef(ref)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256([]byte(gitURL))
		dest, err := SafeJoinUnder(repoRoot, ".cache", "hunt-repos", hex.EncodeToString(sum[:8]))
		if err != nil {
			return nil, err
		}
		if err := cloneOrUpdate(ctx, gitURL, ref, dest); err != nil {
			return nil, err
		}
		sha, _ := gitHead(ctx, dest)
		return &RepoPinResult{Path: dest, GitURL: gitURL, Ref: ref, CommitSHA: sha, PinnedAt: now}, nil
	}
	path := strings.TrimSpace(req.Path)
	if path == "" {
		return nil, errors.New("hunt pin: path or git_url required")
	}
	abs, err := resolveInventoryRoot(repoRoot, path)
	if err != nil {
		return nil, err
	}
	sha, _ := gitHead(ctx, abs)
	return &RepoPinResult{Path: abs, Ref: ref, CommitSHA: sha, PinnedAt: now}, nil
}

func cloneOrUpdate(ctx context.Context, gitURL, ref, dest string) error {
	var err error
	gitURL, err = ValidateGitURL(gitURL)
	if err != nil {
		return err
	}
	ref, err = ValidateGitRef(ref)
	if err != nil {
		return err
	}
	// FindString must be in this function (not via helpers) for CodeQL argv barriers.
	safeURL := reRepoHTTPS.FindString(gitURL)
	if safeURL == "" {
		safeURL = reRepoSSH.FindString(gitURL)
	}
	safeRef := reRepoRef.FindString(ref)
	safeDest := reRepoAbsPath.FindString(filepath.Clean(dest))
	if safeURL == "" || safeRef == "" || safeDest == "" {
		return fmt.Errorf("hunt pin: sanitized git args empty")
	}
	if _, err := os.Stat(filepath.Join(safeDest, ".git")); err == nil {
		return checkoutCloneRef(ctx, safeDest, safeRef)
	}
	if err := os.MkdirAll(filepath.Dir(safeDest), 0o755); err != nil {
		return err
	}
	cloneCtx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cloneCtx, "git", "clone", "--depth", "1", "--branch", safeRef, "--", safeURL, safeDest)
	gitutil.IsolateCmd(cmd)
	if err := cmd.Run(); err != nil {
		_ = os.RemoveAll(safeDest)
		cmd2 := exec.CommandContext(cloneCtx, "git", "clone", "--depth", "1", "--", safeURL, safeDest)
		gitutil.IsolateCmd(cmd2)
		if err2 := cmd2.Run(); err2 != nil {
			return fmt.Errorf("hunt pin: git clone: %w", err)
		}
		return checkoutCloneRef(ctx, safeDest, safeRef)
	}
	return nil
}

func checkoutCloneRef(ctx context.Context, dest, ref string) error {
	if ref == "" {
		return nil
	}
	var err error
	ref, err = ValidateGitRef(ref)
	if err != nil {
		return err
	}
	safeRef := reRepoRef.FindString(ref)
	safeDest := reRepoAbsPath.FindString(filepath.Clean(dest))
	if safeRef == "" || safeDest == "" {
		return fmt.Errorf("hunt pin: sanitized checkout args empty")
	}
	checkCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	refs := []string{safeRef}
	if safeRef == "master" {
		refs = append(refs, "main")
	} else if safeRef == "main" {
		refs = append(refs, "master")
	}
	for _, cand := range refs {
		r := reRepoRef.FindString(cand)
		if r == "" {
			continue
		}
		d := reRepoAbsPath.FindString(safeDest)
		if d == "" {
			continue
		}
		fetch := exec.CommandContext(checkCtx, "git", "-C", d, "fetch", "--depth", "1", "origin", r)
		gitutil.IsolateCmd(fetch)
		_ = fetch.Run()
		co := exec.CommandContext(checkCtx, "git", "-C", d, "checkout", "--force", "--", r)
		gitutil.IsolateCmd(co)
		if co.Run() == nil {
			return nil
		}
	}
	d := reRepoAbsPath.FindString(safeDest)
	if d == "" {
		return fmt.Errorf("hunt pin: git checkout %s failed", safeRef)
	}
	rev := exec.CommandContext(checkCtx, "git", "-C", d, "rev-parse", "HEAD")
	gitutil.IsolateCmd(rev)
	if rev.Run() == nil {
		return nil
	}
	return fmt.Errorf("hunt pin: git checkout %s failed", safeRef)
}

func gitHead(ctx context.Context, dir string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(string(out)), nil
}
