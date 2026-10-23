package hunt

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"hackme/internal/fuzzengine"
)

func TestBuildInventoryAFLHarnessShowmapEdges(t *testing.T) {
	if _, err := exec.LookPath("afl-clang-fast"); err != nil {
		t.Skip("afl-clang-fast missing")
	}
	if _, err := LookAFLShowmap(); err != nil {
		t.Skip("afl-showmap missing")
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "fuzz"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "include"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "include", "hw.h"), []byte(`
#pragma once
#include <stddef.h>
int hw_touch(const unsigned char *d, size_t n);
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "hw.c"), []byte(`
#include "hw.h"
int hw_touch(const unsigned char *d, size_t n) {
  if (n > 0 && d[0] == 'A') return 1;
  if (n > 1 && d[1] == 'B') return 2;
  return 0;
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fuzz", "fuzz_parse.c"), []byte(`
#include "hw.h"
#include <stdint.h>
#include <stddef.h>
int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size) {
  return hw_touch(data, size);
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	bin, err := BuildInventoryAFLHarness(context.Background(), repo, HarnessBuildRequest{
		Pin:       &RepoPinResult{Path: dir},
		SourceRel: "fuzz/fuzz_parse.c",
	})
	if err != nil {
		t.Fatal(err)
	}
	path, instrumented, err := ResolveCoverageHarness(context.Background(), repo, "inv_fuzz_parse_c", CoverageHarnessOpts{
		PinPath:   dir,
		SourceRel: "fuzz/fuzz_parse.c",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !instrumented || path != bin {
		t.Fatalf("instrumented=%v path=%s bin=%s", instrumented, path, bin)
	}
	// Unit CI may lack userns; production coordinator keeps STRICT=1 (showmap requires bwrap).
	t.Setenv("HACKME_HUNT_ISOLATE_STRICT", "0")
	bm, err := ShowmapCoverageBitmap(context.Background(), bin, []byte("AB"))
	if err != nil {
		t.Fatalf("ShowmapCoverageBitmap: %v (bin=%s)", err, bin)
	}
	if !fuzzengine.BitmapHasCoverageSignal(bm) {
		t.Fatalf("expected showmap edges, map_len=%d", len(bm))
	}
}
