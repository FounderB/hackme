package hunt

import (
	"path/filepath"
	"testing"
)

func TestSafeJoinUnderRejectsEscape(t *testing.T) {
	root := t.TempDir()
	if _, err := SafeJoinUnder(root, "..", "etc", "passwd"); err == nil {
		t.Fatal("expected escape reject")
	}
	got, err := SafeJoinUnder(root, "a", "b.c")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "a", "b.c")
	if got != want {
		// Abs may differ by symlink; compare Rel
		rel, err := filepath.Rel(root, got)
		if err != nil || rel != filepath.Join("a", "b.c") {
			t.Fatalf("got %q want under %q", got, want)
		}
	}
}

func TestValidateGitURLAndRef(t *testing.T) {
	if err := ValidateGitURL("https://github.com/foo/bar.git"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateGitURL("https://evil;rm -rf /"); err == nil {
		t.Fatal("expected reject")
	}
	if err := ValidateGitRef("main"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateGitRef("-rf"); err == nil {
		t.Fatal("expected reject leading dash")
	}
	if err := ValidateHexHash("d5a1703fcfaf5f296993b4b6373e9cb9"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateHexHash("../x"); err == nil {
		t.Fatal("expected reject")
	}
}
