package fuzzupstream

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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
	clean := filepath.Clean(binPath)
	if clean != binPath && filepath.Clean(binPath) != clean {
		// normalize
	}
	abs, err := filepath.Abs(clean)
	if err != nil {
		return "", err
	}
	// Reject null bytes and ".." remnants after Clean (should be gone).
	if strings.Contains(abs, string(filepath.Separator)+".."+string(filepath.Separator)) || strings.HasSuffix(abs, string(filepath.Separator)+"..") {
		return "", errors.New("fuzzupstream: binary path escapes")
	}
	st, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("fuzzupstream: binary: %w", err)
	}
	if !st.Mode().IsRegular() {
		return "", errors.New("fuzzupstream: binary must be a regular file")
	}
	return abs, nil
}
