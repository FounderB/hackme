// Package pathsafe provides CodeQL-recognized barriers for path-injection sinks.
// Barrier form that GitHub code scanning accepts: MatchString guard on the same
// string later passed to the filesystem sink (see TaintedPath::RegexpCheck).
package pathsafe

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// AbsRE matches absolute paths with only safe segments (Unix and Windows drive form).
// Callers should guard sinks with AbsRE.MatchString(p) on the path they pass to os.*.
var AbsRE = regexp.MustCompile(`^([A-Za-z]:)?(/[A-Za-z0-9._+-]+)+$`)

// BaseRE matches a single relative filename (no separators).
var BaseRE = regexp.MustCompile(`^[A-Za-z0-9._+-]+$`)

// Allow cleans+Abs a path and returns it only when AbsRE matches (slash-normalized).
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
	if !AbsRE.MatchString(slash) {
		return "", false
	}
	return slash, true
}

// Guard reports whether p is an allowlisted absolute path (CodeQL barrier when used as if-guard).
func Guard(p string) bool {
	return AbsRE.MatchString(p)
}

// Base returns a single path segment sanitized via filepath.Base + BaseRE guard.
func Base(name string) (string, bool) {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == ".." {
		return "", false
	}
	if !BaseRE.MatchString(name) {
		return "", false
	}
	return name, true
}

// DirAllow returns filepath.Dir(p) after Allow+Guard (for MkdirAll sinks).
func DirAllow(p string) (string, bool) {
	if !Guard(p) {
		return "", false
	}
	d := filepath.ToSlash(filepath.Dir(p))
	if !AbsRE.MatchString(d) {
		return "", false
	}
	return d, true
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
	if !AbsRE.MatchString(out) {
		return "", false
	}
	return out, true
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
	if !AbsRE.MatchString(out) {
		return "", false
	}
	return out, true
}

func hasDrive(p string) bool {
	p = filepath.ToSlash(p)
	return len(p) >= 2 && p[1] == ':'
}
