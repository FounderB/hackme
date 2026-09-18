package nodecrypto

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLoadOrCreateSign(t *testing.T) {
	dir := t.TempDir()
	s1, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatal(err)
	}
	msg := []byte(`{"id":"x","reward_hmc":0.01}`)
	sig := s1.SignHex(msg)
	if len(sig) != 128 {
		t.Fatalf("sig len %d", len(sig))
	}
	s2, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s2.PublicKeyHex() != s1.PublicKeyHex() {
		t.Fatal("pub mismatch")
	}
	if s2.SignHex(msg) != sig {
		t.Fatal("deterministic sign")
	}
	if s1.Address() != s2.Address() {
		t.Fatal("address mismatch")
	}
	if !strings.HasPrefix(s1.Address(), "HMC-") || len(s1.Address()) != 20 {
		t.Fatalf("unexpected address format: %q", s1.Address())
	}
	_ = filepath.Join(dir, seedFile)
}

func TestLoadOrCreateRepairsWorldReadable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows FileMode.Perm is not unix-faithful")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, seedFile)
	if err := os.WriteFile(p, []byte(strings.Repeat("ab", 32)), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s == nil || s.Address() == "" {
		t.Fatal("expected signer after repair")
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		t.Fatalf("want 0600 after repair, got %04o", fi.Mode().Perm())
	}
}

func TestLoadOrCreateRepairsGroupReadable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows FileMode.Perm is not unix-faithful")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, seedFile)
	if err := os.WriteFile(p, []byte(strings.Repeat("cd", 32)), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreate(dir); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		t.Fatalf("want 0600 after repair, got %04o", fi.Mode().Perm())
	}
}

func TestLoadOrCreateRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "elsewhere.seed")
	if err := os.WriteFile(target, []byte(strings.Repeat("ef", 32)), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, seedFile)
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreate(dir); err == nil {
		t.Fatal("expected reject of symlink seed")
	}
}

func TestReadSeedFileOK(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "ok.seed")
	want := []byte(strings.Repeat("11", 32))
	if err := os.WriteFile(p, want, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadSeedFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestReadSeedFileRepairs0666(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "x.seed")
	if err := os.WriteFile(p, []byte(strings.Repeat("aa", 32)), 0o666); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadSeedFile(p); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		t.Fatalf("want repaired 0600, got %04o", fi.Mode().Perm())
	}
}
