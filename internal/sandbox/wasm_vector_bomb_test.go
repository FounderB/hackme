package sandbox

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRejectWasmInflatedExportCount builds a tiny module whose export-section
// LEB count claims ~213M entries (the wazero OOM shape) and asserts we reject
// before CompileModule. Does not ship the auditor's hang_input.bin.
func TestRejectWasmInflatedExportCount(t *testing.T) {
	bomb := []byte{
		0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, // header
		0x01, 0x04, 0x01, 0x60, 0x00, 0x00, // type: 1 empty func type
		0x03, 0x02, 0x01, 0x00, // function: 1 → type 0
		0x07, 0x08, // export section size=8
		0x82, 0xfd, 0xed, 0x65, // LEB export_count = 213614210
		0x00, 0x00, 0x00, 0x00, // remaining bytes << count
	}
	if err := rejectWasmHostileSections(bomb); err == nil {
		t.Fatal("expected inflated export count rejection")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := ValidateCheckWasm(ctx, bomb); err == nil {
		t.Fatal("ValidateCheckWasm must reject before wazero CompileModule")
	}
}

func TestRejectWasmInflatedExportCountFromAuditorFixture(t *testing.T) {
	p := os.Getenv("HACKME_WASM_BOMB_FIXTURE")
	if p == "" {
		candidates := []string{
			"/tmp/user/1000/claude-1000/-home-kapa/74254822-ec3a-4dcf-b54f-dc9642b48941/scratchpad/audit/hang_input.bin",
		}
		for _, c := range candidates {
			if st, err := os.Stat(c); err == nil && st.Size() > 0 {
				p = c
				break
			}
		}
	}
	if p == "" {
		t.Skip("no local hang_input fixture")
	}
	raw, err := os.ReadFile(filepath.Clean(p))
	if err != nil || len(raw) < 8 {
		t.Skip(err)
	}
	if err := rejectWasmHostileSections(raw); err == nil {
		t.Fatal("fixture must be rejected by section precheck")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	err = ValidateCheckWasm(ctx, raw)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("ValidateCheckWasm must reject fixture")
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("rejection too slow (%s) — may have hit CompileModule", elapsed)
	}
}

func TestRejectWasmValidMinimalStillOK(t *testing.T) {
	raw, err := hex.DecodeString(MinimalGateWasmHex)
	if err != nil {
		t.Fatal(err)
	}
	if err := rejectWasmHostileSections(raw); err != nil {
		t.Fatalf("minimal gate rejected: %v", err)
	}
}
