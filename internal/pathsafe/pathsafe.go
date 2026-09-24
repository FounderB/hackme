// Package pathsafe provides CodeQL-recognized barriers for path-injection sinks.
// Regex MatchString + rebuild is the pattern GitHub code scanning accepts
// (Rel/HasPrefix alone is often not enough for go/path-injection).
package pathsafe

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Absolute paths with only safe path segments (Unix). Used after Clean+Abs.
var reSafeAbs = regexp.MustCompile(`^(/[A-Za-z0-9._+-]+)+$`)

// Single relative filename (no separators).
var reSafeBase = regexp.MustCompile(`^[A-Za-z0-9._+-]+$`)

// Allow returns p only when it is a clean absolute path matching reSafeAbs.
// Callers must pass the returned string to filesystem sinks.
func Allow(p string) (string, bool) {
	p = filepath.Clean(strings.TrimSpace(p))
	if p == "" || p == "." || p == "/" {
		return "", false
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", false
	}
	abs = filepath.Clean(abs)
	if !reSafeAbs.MatchString(abs) {
		return "", false
	}
	return abs, true
}

// Base returns a single path segment sanitized via filepath.Base + allowlist.
func Base(name string) (string, bool) {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == ".." {
		return "", false
	}
	if !reSafeBase.MatchString(name) {
		return "", false
	}
	return name, true
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
		joined = filepath.Join(append([]string{absRoot}, parts...)...)
	}
	joined = filepath.Clean(joined)
	rel, err := filepath.Rel(absRoot, joined)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", false
	}
	out := filepath.Join(absRoot, rel)
	if out != absRoot && !strings.HasPrefix(out, absRoot+string(os.PathSeparator)) {
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
	if !filepath.IsAbs(full) {
		full = filepath.Join(absRoot, full)
	}
	full = filepath.Clean(full)
	rel, err := filepath.Rel(absRoot, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", false
	}
	out := filepath.Join(absRoot, rel)
	if out != absRoot && !strings.HasPrefix(out, absRoot+string(os.PathSeparator)) {
		return "", false
	}
	return Allow(out)
}
