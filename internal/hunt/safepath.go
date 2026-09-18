package hunt

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"hackme/internal/gitutil"
)

// Hex allowlist for cache ids (git URL/ref live in gitutil).
var (
	reHexHash     = regexp.MustCompile(`^[a-fA-F0-9]{8,128}$`)
	reSafeAbsPath = regexp.MustCompile(`^(/[A-Za-z0-9._+-]+)+$`)
)

// allowlistedAbs rebuilds an absolute path via FindString so filesystem sinks
// see a CodeQL-recognized path-injection barrier.
func allowlistedAbs(p string) (string, error) {
	p = filepath.Clean(strings.TrimSpace(p))
	if p == "" || p == "." || p == "/" {
		return "", errors.New("hunt: empty path")
	}
	if s := reSafeAbsPath.FindString(p); s != "" && s == p {
		return s, nil
	}
	return "", fmt.Errorf("hunt: path rejected by allowlist")
}

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
	if rel == "." {
		return allowlistedAbs(absRoot)
	}
	out := filepath.Join(absRoot, rel)
	if !pathUnderRoot(absRoot, out) {
		return "", fmt.Errorf("hunt: path escapes root: %s", out)
	}
	return allowlistedAbs(out)
}

func pathUnderRoot(absRoot, absPath string) bool {
	if absPath == absRoot {
		return true
	}
	sep := string(os.PathSeparator)
	return strings.HasPrefix(absPath, absRoot+sep)
}

// MustUnderRoot returns a path rebuilt under root if path is inside root.
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
	if rel == "." {
		return allowlistedAbs(absRoot)
	}
	out := filepath.Join(absRoot, rel)
	if !pathUnderRoot(absRoot, out) {
		return "", fmt.Errorf("hunt: path escapes root: %s", out)
	}
	return allowlistedAbs(out)
}

// SafeReadFileUnder reads a file only after confining path under root (CodeQL path-injection barrier).
func SafeReadFileUnder(root, path string) ([]byte, error) {
	abs, err := confineUnderRoot(root, path)
	if err != nil {
		return nil, err
	}
	safe := reSafeAbsPath.FindString(abs)
	if safe == "" || safe != abs {
		return nil, fmt.Errorf("hunt: path rejected by allowlist")
	}
	return os.ReadFile(safe)
}

// SafeStatUnder stats a path only after confining it under root.
func SafeStatUnder(root, path string) (os.FileInfo, error) {
	abs, err := confineUnderRoot(root, path)
	if err != nil {
		return nil, err
	}
	safe := reSafeAbsPath.FindString(abs)
	if safe == "" || safe != abs {
		return nil, fmt.Errorf("hunt: path rejected by allowlist")
	}
	return os.Stat(safe)
}

// SafeReadDirUnder lists a directory confined under root.
func SafeReadDirUnder(root, path string) ([]os.DirEntry, error) {
	abs, err := confineUnderRoot(root, path)
	if err != nil {
		return nil, err
	}
	safe := reSafeAbsPath.FindString(abs)
	if safe == "" || safe != abs {
		return nil, fmt.Errorf("hunt: path rejected by allowlist")
	}
	return os.ReadDir(safe)
}

// SafeWriteFileUnder writes data to path confined under root.
func SafeWriteFileUnder(root, path string, data []byte, perm os.FileMode) error {
	abs, err := confineUnderRoot(root, path)
	if err != nil {
		return err
	}
	safe := reSafeAbsPath.FindString(abs)
	if safe == "" || safe != abs {
		return fmt.Errorf("hunt: path rejected by allowlist")
	}
	return os.WriteFile(safe, data, perm)
}

// SafeRenameUnder renames oldpath to newpath; both must confine under root.
func SafeRenameUnder(root, oldpath, newpath string) error {
	fromAbs, err := confineUnderRoot(root, oldpath)
	if err != nil {
		return err
	}
	from := reSafeAbsPath.FindString(fromAbs)
	if from == "" || from != fromAbs {
		return fmt.Errorf("hunt: path rejected by allowlist")
	}
	toAbs, err := confineUnderRoot(root, newpath)
	if err != nil {
		return err
	}
	to := reSafeAbsPath.FindString(toAbs)
	if to == "" || to != toAbs {
		return fmt.Errorf("hunt: path rejected by allowlist")
	}
	return os.Rename(from, to)
}

// confineUnderRoot joins relative paths under root; absolute paths must already be under root.
func confineUnderRoot(root, path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("hunt: empty path")
	}
	if filepath.IsAbs(path) {
		return MustUnderRoot(root, path)
	}
	return SafeJoinUnder(root, path)
}

// ValidateGitURL rejects non-https / non-git@ SSH URLs and shell metacharacters.
// Returns the allowlisted URL string for use at exec argv sinks (CodeQL barrier).
func ValidateGitURL(gitURL string) (string, error) {
	return gitutil.SanitizeURL(gitURL)
}

// ValidateGitRef allows only safe refnames (no leading dash / shell meta).
// Returns the allowlisted ref for use at exec argv sinks (CodeQL barrier).
func ValidateGitRef(ref string) (string, error) {
	return gitutil.SanitizeRef(ref)
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
