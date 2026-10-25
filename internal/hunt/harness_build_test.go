package hunt

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPreviewTemplateNeedsAccept(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "plain.c")
	body := `int parse(const unsigned char *d, int n) { if (n > 0 && d[0] == 0xff) return *(int*)d; return 0; }`
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	prev, err := PreviewTemplate(dir, dir, "plain.c")
	if err != nil {
		t.Fatal(err)
	}
	if prev.HasFuzzEntry {
		t.Fatal("expected no fuzz entry")
	}
	if !prev.NeedsAccept {
		t.Fatal("expected needs_accept")
	}
}

func TestBuildInventoryHarnessWithTemplate(t *testing.T) {
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang required")
	}
	t.Setenv("HACKME_HUNT_ISOLATE_STRICT", "0") // desk userns often denies bwrap
	t.Setenv("HACKME_HUNT_BUILD_SANDBOX", "0")
	dir := t.TempDir()
	src := filepath.Join(dir, "fuzz_target.c")
	body := `int LLVMFuzzerTestOneInput(const unsigned char *d, unsigned long n) {
		if (n > 4 && d[0]=='c' && d[1]=='r' && d[2]=='a' && d[3]=='s' && d[4]=='h') {
			*(volatile int*)0 = 1;
		}
		return 0;
	}`
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	pin := &RepoPinResult{Path: dir, CommitSHA: "testsha"}
	res, err := BuildInventoryHarness(context.Background(), dir, HarnessBuildRequest{
		Pin:            pin,
		SourceRel:      "fuzz_target.c",
		TemplateAccept: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.HarnessHash == "" || res.BinaryPath == "" {
		t.Fatalf("build=%+v", res)
	}
	if _, err := os.Stat(res.BinaryPath); err != nil {
		t.Fatal(err)
	}
}

func TestInventoryHarnessHashStable(t *testing.T) {
	content := []byte("int LLVMFuzzerTestOneInput(const unsigned char *d, unsigned long n) { return 0; }")
	h1 := InventoryHarnessHash("abc", "foo.c", content)
	h2 := InventoryHarnessHash("abc", "foo.c", content)
	if h1 != h2 || h1 == "" {
		t.Fatalf("hash=%q", h1)
	}
}

// Length-confusion OOB reads past the logical seed must trip ASAN. A static
// stdin buffer of 64KiB previously hid these (heavywire BUG-HEAP class).
func TestBuildInventoryHarnessDetectsInputOOBRead(t *testing.T) {
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang required")
	}
	t.Setenv("HACKME_HUNT_ISOLATE_STRICT", "0")
	t.Setenv("HACKME_HUNT_BUILD_SANDBOX", "0")
	dir := t.TempDir()
	src := filepath.Join(dir, "fuzz_oob.c")
	// Use an explicit byte loop (not memcpy into a stack tmp): some clang/ASAN
	// builds elide the stack-tmp pattern; the loop matches length-confusion reads.
	body := `
#include <stdint.h>
#include <stddef.h>
int LLVMFuzzerTestOneInput(const uint8_t *d, size_t n) {
	if (n < 2) return 0;
	size_t claim = d[0];
	volatile uint8_t sink = 0;
	for (size_t i = 0; i < claim; i++) {
		sink ^= d[1 + i]; /* OOB read when claim > n-1 */
	}
	return (int)sink;
}
`
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	pin := &RepoPinResult{Path: dir, CommitSHA: "oobsha"}
	res, err := BuildInventoryHarness(context.Background(), dir, HarnessBuildRequest{
		Pin: pin, SourceRel: "fuzz_oob.c",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Seed: claim=64, only 3 payload bytes → ASAN heap-buffer-overflow on read.
	seed := []byte{64, 'A', 'B', 'C'}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, res.BinaryPath)
	cmd.Stdin = bytes.NewReader(seed)
	// symbolize=0 — llvm-symbolizer can hang CombinedOutput under load (seen on soak hosts).
	cmd.Env = append(os.Environ(), "ASAN_OPTIONS=abort_on_error=1:detect_leaks=0:symbolize=0")
	out, runErr := cmd.CombinedOutput()
	if runErr == nil {
		t.Fatalf("expected ASAN crash, got ok output=%q", string(out))
	}
	s := string(out)
	if !strings.Contains(s, "AddressSanitizer") && !strings.Contains(s, "heap-buffer-overflow") {
		t.Fatalf("expected ASAN signature, err=%v out=%q", runErr, s)
	}
}
