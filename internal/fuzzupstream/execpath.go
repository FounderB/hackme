package fuzzupstream

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Absolute harness path allowlist — FindString return is a CodeQL command/path barrier.
var reAbsBinPath = regexp.MustCompile(`^(/[A-Za-z0-9._+-]+)+$`)

// ValidateBinPath ensures an ASAN harness binary path is a cleaned absolute regular file
// with no path escape / shell metacharacters (CodeQL command-injection barrier).
func ValidateBinPath(binPath string) (string, error) {
	binPath = strings.TrimSpace(binPath)
	if binPath == "" {
		return "", errors.New("fuzzupstream: empty binary path")
	}
	if strings.ContainsAny(binPath, " \t\n\r;|&$`\\\"'") {
		return "", errors.New("fuzzupstream: binary path contains forbidden characters")
	}
	if !filepath.IsAbs(binPath) {
		return "", errors.New("fuzzupstream: binary path must be absolute")
	}
	abs, err := filepath.Abs(filepath.Clean(binPath))
	if err != nil {
		return "", err
	}
	if strings.Contains(abs, string(filepath.Separator)+".."+string(filepath.Separator)) || strings.HasSuffix(abs, string(filepath.Separator)+"..") {
		return "", errors.New("fuzzupstream: binary path escapes")
	}
	// Rebuild via allowlist so sinks use a non-tainted string.
	safe := reAbsBinPath.FindString(abs)
	if safe == "" || safe != abs {
		return "", fmt.Errorf("fuzzupstream: binary path rejected by allowlist")
	}
	st, err := os.Stat(safe)
	if err != nil {
		return "", fmt.Errorf("fuzzupstream: binary: %w", err)
	}
	if !st.Mode().IsRegular() {
		return "", errors.New("fuzzupstream: binary must be a regular file")
	}
	return safe, nil
}
