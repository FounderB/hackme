package hunt

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCopyCorpusFilesHardlinkSameFS(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	payload := []byte("seed-body-aaaa")
	if err := os.WriteFile(filepath.Join(src, "id:000001"), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "id:000002"), []byte("seed-body-bbbb"), 0o644); err != nil {
		t.Fatal(err)
	}
	n := copyCorpusFiles(src, dst, 32)
	if n != 2 {
		t.Fatalf("copied=%d want 2", n)
	}
	dest := filepath.Join(dst, "afl-id:000001")
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("content=%q", got)
	}
	stSrc, err := os.Stat(filepath.Join(src, "id:000001"))
	if err != nil {
		t.Fatal(err)
	}
	stDst, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	// On Unix same-FS hardlink: same inode. Windows / cross-device falls back to copy.
	if runtime.GOOS != "windows" {
		if si, ok := fileInode(stSrc); ok {
			if di, ok2 := fileInode(stDst); ok2 && si != di {
				t.Fatalf("expected hardlink same inode got src=%d dst=%d", si, di)
			}
		}
	}
	mode := stDst.Mode().Perm()
	if mode&0o777 != 0o600 {
		t.Fatalf("dest mode=%o want 0600", mode)
	}
}

func TestCopyCorpusFilesSkipsExistingAndOversized(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "keep"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	big := make([]byte, (1<<20)+8)
	if err := os.WriteFile(filepath.Join(src, "huge"), big, 0o600); err != nil {
		t.Fatal(err)
	}
	pre := filepath.Join(dst, "afl-keep")
	if err := os.WriteFile(pre, []byte("already"), 0o600); err != nil {
		t.Fatal(err)
	}
	n := copyCorpusFiles(src, dst, 32)
	if n != 0 {
		t.Fatalf("existing dest must skip; oversized ignored; got n=%d", n)
	}
	got, _ := os.ReadFile(pre)
	if string(got) != "already" {
		t.Fatalf("must not overwrite existing: %q", got)
	}
	if _, err := os.Stat(filepath.Join(dst, "afl-huge")); err == nil {
		t.Fatal("oversized file must not be imported")
	}
}

func TestCopyCorpusFilesRespectsMaxAndStableOrder(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	for _, name := range []string{"c", "a", "b"} {
		if err := os.WriteFile(filepath.Join(src, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	n := copyCorpusFiles(src, dst, 2)
	if n != 2 {
		t.Fatalf("n=%d want 2", n)
	}
	// Lexicographic: a, b (c dropped by max).
	if _, err := os.Stat(filepath.Join(dst, "afl-a")); err != nil {
		t.Fatal("want afl-a")
	}
	if _, err := os.Stat(filepath.Join(dst, "afl-b")); err != nil {
		t.Fatal("want afl-b")
	}
	if _, err := os.Stat(filepath.Join(dst, "afl-c")); err == nil {
		t.Fatal("c must be beyond max")
	}
}

func TestCopyCorpusFilesCrashPrefix(t *testing.T) {
	src := t.TempDir()
	// Parent path contains "crashes" so names get afl-crash- prefix.
	crashDir := filepath.Join(src, "crashes")
	if err := os.MkdirAll(crashDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	if err := os.WriteFile(filepath.Join(crashDir, "id:crash1"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	n := copyCorpusFiles(crashDir, dst, 8)
	if n != 1 {
		t.Fatalf("n=%d", n)
	}
	if _, err := os.Stat(filepath.Join(dst, "afl-crash-id:crash1")); err != nil {
		t.Fatal(err)
	}
}

func TestPlaceCorpusFileFallbackCopy(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	srcFile := filepath.Join(src, "seed")
	if err := os.WriteFile(srcFile, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dst, "out")
	if !placeCorpusFile(srcFile, dest) {
		t.Fatal("place failed")
	}
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != "body" {
		t.Fatalf("got %q err=%v", got, err)
	}
}
