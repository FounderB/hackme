package gitutil

import "testing"

func TestSanitizeURLRejectsPrivateAndUserinfo(t *testing.T) {
	if _, err := SanitizeURL("https://github.com/foo/bar.git"); err != nil {
		t.Fatalf("public github: %v", err)
	}
	for _, bad := range []string{
		"https://127.0.0.1/repo.git",
		"https://10.0.0.1/repo.git",
		"https://169.254.169.254/latest/meta-data/",
		"https://user:pass@github.com/foo/bar.git",
		"https://evil@127.0.0.1/x.git",
		"git@127.0.0.1:foo/bar.git",
	} {
		if _, err := SanitizeURL(bad); err == nil {
			t.Fatalf("expected reject for %q", bad)
		}
	}
}

func TestSanitizeURLAllowPrivateEgress(t *testing.T) {
	t.Setenv("HACKME_ALLOW_PRIVATE_EGRESS", "1")
	if _, err := SanitizeURL("https://10.0.0.5/repo.git"); err != nil {
		t.Fatalf("lab override should allow private: %v", err)
	}
}
