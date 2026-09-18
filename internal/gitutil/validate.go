package gitutil

import (
	"errors"
	"regexp"
	"strings"
)

// Allowlists — FindString returns are CodeQL command-injection barriers when used at argv sinks.
var (
	reGitHTTPS = regexp.MustCompile(`^https://[A-Za-z0-9._~:/?#\[\]@!$&'()*+,;=%-]+$`)
	reGitSSH   = regexp.MustCompile(`^git@[A-Za-z0-9.-]+:[A-Za-z0-9._~/-]+\.git$`)
	reGitRef   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,200}$`)
)

// SanitizeURL returns an allowlisted git remote URL or an error.
func SanitizeURL(gitURL string) (string, error) {
	gitURL = strings.TrimSpace(gitURL)
	if gitURL == "" {
		return "", errors.New("gitutil: empty git_url")
	}
	if strings.ContainsAny(gitURL, " \t\n\r;|&$`\\\"'") {
		return "", errors.New("gitutil: git_url contains forbidden characters")
	}
	if s := reGitHTTPS.FindString(gitURL); s != "" && s == gitURL {
		return s, nil
	}
	if s := reGitSSH.FindString(gitURL); s != "" && s == gitURL {
		return s, nil
	}
	return "", errors.New("gitutil: git_url must be https://… or git@host:path.git")
}

// SanitizeRef returns an allowlisted git refname or an error.
func SanitizeRef(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", errors.New("gitutil: empty git ref")
	}
	if strings.HasPrefix(ref, "-") {
		return "", errors.New("gitutil: git ref must not start with -")
	}
	if strings.ContainsAny(ref, " \t\n\r;|&$`\\\"'") {
		return "", errors.New("gitutil: git ref contains forbidden characters")
	}
	if s := reGitRef.FindString(ref); s != "" && s == ref {
		return s, nil
	}
	return "", errors.New("gitutil: git ref rejected")
}
