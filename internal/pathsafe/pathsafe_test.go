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
	want := filepath.ToSlash(filepath.Join(root, "a", "b.c"))
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestWithinRoot(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "x.dat")
	got, ok := WithinRoot(root, inside)
	if !ok || got != filepath.ToSlash(inside) {
		t.Fatalf("inside: ok=%v got=%q", ok, got)
	}
	if _, ok := WithinRoot(root, filepath.Join(root, "..", "outside")); ok {
		t.Fatal("expected outside reject")
	}
}

func TestAllowWindowsDriveForm(t *testing.T) {
	// Simulate ToSlash drive path matching (logic unit; Abs still OS-native).
	slash := "C:/Users/hackme/data"
	if m := AbsRE.FindString(slash); m != slash {
		t.Fatalf("windows drive form rejected: %q", slash)
	}
	slashUnix := "/home/kapa/Desktop/HackMe"
	if m := AbsRE.FindString(slashUnix); m != slashUnix {
		t.Fatalf("unix form rejected: %q", slashUnix)
	}
}

func TestBase(t *testing.T) {
	got, ok := Base("seed-01.bin")
	if !ok || got != "seed-01.bin" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
	got, ok = Base("../etc/passwd")
	if !ok || got != "passwd" {
		t.Fatalf("Base should collapse to passwd, got %q ok=%v", got, ok)
	}
}
