package hunt

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Git/ref allowlists — CodeQL command-injection barriers (argv still used; values must be constrained).
var (
	reGitHTTPS = regexp.MustCompile(`^https://[A-Za-z0-9._~:/?#\[\]@!$&'()*+,;=%-]+$`)
	reGitSSH   = regexp.MustCompile(`^git@[A-Za-z0-9.-]+:[A-Za-z0-9._~/-]+\.git$`)
	reGitRef   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,200}$`)
	reHexHash  = regexp.MustCompile(`^[a-fA-F0-9]{16,128}$`)
)

// SafeJoinUnder joins elem under root and rejects path escape (CodeQL path-injection barrier).
func SafeJoinUnder(root string, elem ...string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", errors.New("hunt: empty root path")
	}
	absRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, len(elem))
	for _, e := range elem {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if filepath.IsAbs(e) {
			return "", fmt.Errorf("hunt: absolute path segment rejected: %s", e)
		}
		if e == ".." || strings.Contains(e, ".."+string(os.PathSeparator)) || strings.HasPrefix(e, ".."+string(os.PathSeparator)) {
			// still allow Clean to resolve, Rel check is authoritative
		}
		parts = append(parts, e)
	}
	joined := absRoot
	if len(parts) > 0 {
		joined = filepath.Join(append([]string{absRoot}, parts...)...)
	}
	joined = filepath.Clean(joined)
	rel, err := filepath.Rel(absRoot, joined)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("hunt: path escapes root: %s", joined)
	}
	return joined, nil
}

// MustUnderRoot returns cleaned abs if path is inside root.
func MustUnderRoot(root, path string) (string, error) {
	root = strings.TrimSpace(root)
	path = strings.TrimSpace(path)
	if root == "" || path == "" {
		return "", errors.New("hunt: root and path required")
	}
	absRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	absPath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("hunt: path escapes root: %s", absPath)
	}
	return absPath, nil
}

// ValidateGitURL rejects non-https / non-git@ SSH URLs and shell metacharacters.
func ValidateGitURL(gitURL string) error {
	gitURL = strings.TrimSpace(gitURL)
	if gitURL == "" {
		return errors.New("hunt: empty git_url")
	}
	if strings.ContainsAny(gitURL, " \t\n\r;|&$`\\\"'") {
		return errors.New("hunt: git_url contains forbidden characters")
	}
	if reGitHTTPS.MatchString(gitURL) || reGitSSH.MatchString(gitURL) {
		return nil
	}
	return errors.New("hunt: git_url must be https://… or git@host:path.git")
}

// ValidateGitRef allows only safe refnames (no leading dash / shell meta).
func ValidateGitRef(ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return errors.New("hunt: empty git ref")
	}
	if strings.HasPrefix(ref, "-") {
		return errors.New("hunt: git ref must not start with -")
	}
	if strings.ContainsAny(ref, " \t\n\r;|&$`\\\"'") {
		return errors.New("hunt: git ref contains forbidden characters")
	}
	if !reGitRef.MatchString(ref) {
		return errors.New("hunt: git ref rejected")
	}
	return nil
}

// ValidateHexHash requires a hex harness/cache id.
func ValidateHexHash(h string) error {
	h = strings.TrimSpace(h)
	if !reHexHash.MatchString(h) {
		return errors.New("hunt: invalid hex hash")
	}
	return nil
}

// SafeCacheFile joins repoRoot/.cache/<bucket>/<hash>.<ext> after validating hash.
func SafeCacheFile(repoRoot, bucket, hash, ext string) (string, error) {
	if err := ValidateHexHash(hash); err != nil {
		return "", err
	}
	bucket = strings.TrimSpace(bucket)
	if bucket == "" || strings.Contains(bucket, "..") || strings.ContainsAny(bucket, `/\`) {
		return "", errors.New("hunt: invalid cache bucket")
	}
	name := hash
	if ext != "" {
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		name = hash + ext
	}
	return SafeJoinUnder(repoRoot, ".cache", bucket, name)
}
