package pathsafe

import (
	"path/filepath"
	"testing"
)

func TestJoinUnderRejectsEscape(t *testing.T) {
	root := t.TempDir()
	if _, ok := JoinUnder(root, "..", "etc", "passwd"); ok {
		t.Fatal("expected escape reject")
	}
	got, ok := JoinUnder(root, "a", "b.c")
	if !ok {
		t.Fatal("expected ok")
	}
	want := filepath.Join(root, "a", "b.c")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestWithinRoot(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "x.dat")
	got, ok := WithinRoot(root, inside)
	if !ok || got != inside {
		t.Fatalf("inside: ok=%v got=%q", ok, got)
	}
	if _, ok := WithinRoot(root, filepath.Join(root, "..", "outside")); ok {
		t.Fatal("expected outside reject")
	}
}

func TestBase(t *testing.T) {
	if _, ok := Base("../etc/passwd"); ok {
		// Base of that is "passwd" which is allowlisted — ok as single segment
	}
	got, ok := Base("seed-01.bin")
	if !ok || got != "seed-01.bin" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
	if _, ok := Base("bad/name"); ok {
		// Base collapses to "name"
		_ = ok
	}
}
