package gitutil

import (
	"errors"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"hackme/internal/netutil"
)

// Allowlists — FindString returns are CodeQL command-injection barriers when used at argv sinks.
var (
	reGitHTTPS = regexp.MustCompile(`^https://[A-Za-z0-9._~:/?#\[\]@!$&'()*+,;=%-]+$`)
	reGitSSH   = regexp.MustCompile(`^git@[A-Za-z0-9.-]+:[A-Za-z0-9._~/-]+\.git$`)
	reGitRef   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,200}$`)
	reAbsPath  = regexp.MustCompile(`^(/[A-Za-z0-9._+-]+)+$`)
)

// SanitizeURL returns an allowlisted git remote URL or an error.
// Rejects userinfo and hosts that resolve to loopback/private/link-local/metadata
// (Hunt pin SSRF). Lab override: HACKME_ALLOW_PRIVATE_EGRESS=1.
func SanitizeURL(gitURL string) (string, error) {
	gitURL = strings.TrimSpace(gitURL)
	if gitURL == "" {
		return "", errors.New("gitutil: empty git_url")
	}
	if strings.ContainsAny(gitURL, " \t\n\r;|&$`\\\"'") {
		return "", errors.New("gitutil: git_url contains forbidden characters")
	}
	if s := reGitHTTPS.FindString(gitURL); s != "" && s == gitURL {
		if err := assertHTTPSSafe(s); err != nil {
			return "", err
		}
		return s, nil
	}
	if s := reGitSSH.FindString(gitURL); s != "" && s == gitURL {
		if err := assertSSHSafe(s); err != nil {
			return "", err
		}
		return s, nil
	}
	return "", errors.New("gitutil: git_url must be https://… or git@host:path.git")
}

func assertHTTPSSafe(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return errors.New("gitutil: invalid https git_url")
	}
	if u.User != nil && u.User.String() != "" {
		return errors.New("gitutil: git_url must not include userinfo")
	}
	if err := netutil.AssertHostSafeForEgress(u.Hostname(), false); err != nil {
		return errors.New("gitutil: " + err.Error())
	}
	return nil
}

func assertSSHSafe(raw string) error {
	// git@host:path.git
	rest := strings.TrimPrefix(raw, "git@")
	host, _, ok := strings.Cut(rest, ":")
	if !ok || strings.TrimSpace(host) == "" {
		return errors.New("gitutil: invalid ssh git_url")
	}
	if err := netutil.AssertHostSafeForEgress(host, false); err != nil {
		return errors.New("gitutil: " + err.Error())
	}
	return nil
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

// MustAllowlistedURL returns FindString match or "" (CodeQL argv barrier helper).
func MustAllowlistedURL(gitURL string) string {
	gitURL = strings.TrimSpace(gitURL)
	if s := reGitHTTPS.FindString(gitURL); s != "" && s == gitURL {
		if assertHTTPSSafe(s) != nil {
			return ""
		}
		return s
	}
	if s := reGitSSH.FindString(gitURL); s != "" && s == gitURL {
		if assertSSHSafe(s) != nil {
			return ""
		}
		return s
	}
	return ""
}

// MustAllowlistedRef returns FindString match or "".
func MustAllowlistedRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if s := reGitRef.FindString(ref); s != "" && s == ref {
		return s
	}
	return ""
}

// MustAllowlistedAbsPath returns FindString match for an absolute path or "".
func MustAllowlistedAbsPath(p string) string {
	p = filepath.Clean(strings.TrimSpace(p))
	if s := reAbsPath.FindString(p); s != "" && s == p {
		return s
	}
	return ""
}
