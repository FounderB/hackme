// Package pathsafe provides CodeQL-recognized barriers for path-injection sinks.
// Regex FindString + return of that match is the pattern GitHub code scanning accepts
// (Rel/HasPrefix alone is often not enough for go/path-injection).
package pathsafe

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Absolute paths with only safe path segments (Unix and Windows drive form).
// Evaluated on filepath.ToSlash output after Abs+Clean.
var reSafeAbs = regexp.MustCompile(`^([A-Za-z]:)?(/[A-Za-z0-9._+-]+)+$`)

// Single relative filename (no separators).
var reSafeBase = regexp.MustCompile(`^[A-Za-z0-9._+-]+$`)

// Allow returns p only when it is a clean absolute path matching reSafeAbs.
// The returned string is the FindString match itself (CodeQL barrier) — do not rebuild.
func Allow(p string) (string, bool) {
	p = filepath.Clean(strings.TrimSpace(p))
	if p == "" || p == "." {
		return "", false
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", false
	}
	slash := filepath.ToSlash(filepath.Clean(abs))
	if slash == "/" || slash == "" {
		return "", false
	}
	m := reSafeAbs.FindString(slash)
	if m == "" || m != slash {
		return "", false
	}
	return m, true
}

// Base returns a single path segment sanitized via filepath.Base + FindString.
func Base(name string) (string, bool) {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == ".." {
		return "", false
	}
	m := reSafeBase.FindString(name)
	if m == "" || m != name {
		return "", false
	}
	return m, true
}

// JoinUnder joins Base(elem...) under root and returns an Allow-listed absolute path.
func JoinUnder(root string, elem ...string) (string, bool) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", false
	}
	absRoot, ok := Allow(root)
	if !ok {
		return "", false
	}
	parts := make([]string, 0, len(elem))
	for _, e := range elem {
		b, ok := Base(e)
		if !ok {
			return "", false
		}
		parts = append(parts, b)
	}
	joined := absRoot
	if len(parts) > 0 {
		joined = filepath.ToSlash(filepath.Join(append([]string{absRoot}, parts...)...))
	}
	joined = filepath.ToSlash(filepath.Clean(joined))
	rel, err := filepath.Rel(filepath.FromSlash(absRoot), filepath.FromSlash(joined))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || strings.HasPrefix(rel, "../") {
		return "", false
	}
	out := filepath.ToSlash(filepath.Join(absRoot, filepath.ToSlash(rel)))
	rootSlash := strings.TrimSuffix(absRoot, "/")
	if out != rootSlash && out != absRoot && !strings.HasPrefix(out, rootSlash+"/") {
		return "", false
	}
	return Allow(out)
}

// WithinRoot rebuilds path under root if it resolves inside root (Allow-listed).
func WithinRoot(root, path string) (string, bool) {
	absRoot, ok := Allow(root)
	if !ok {
		return "", false
	}
	full := filepath.Clean(path)
	if !filepath.IsAbs(full) && !strings.HasPrefix(filepath.ToSlash(full), "/") && !hasDrive(full) {
		full = filepath.Join(filepath.FromSlash(absRoot), full)
	}
	fullSlash := filepath.ToSlash(filepath.Clean(full))
	rel, err := filepath.Rel(filepath.FromSlash(absRoot), filepath.FromSlash(fullSlash))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || strings.HasPrefix(rel, "../") {
		return "", false
	}
	out := filepath.ToSlash(filepath.Join(absRoot, filepath.ToSlash(rel)))
	rootSlash := strings.TrimSuffix(absRoot, "/")
	if out != rootSlash && out != absRoot && !strings.HasPrefix(out, rootSlash+"/") {
		return "", false
	}
	return Allow(out)
}

func hasDrive(p string) bool {
	p = filepath.ToSlash(p)
	return len(p) >= 2 && p[1] == ':'
}
