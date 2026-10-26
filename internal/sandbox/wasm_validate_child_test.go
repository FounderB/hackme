package sandbox

import (
	"context"
	"encoding/hex"
	"testing"
	"time"
)

// TestValidateCheckWasmChildAcceptsMinimalGate is the regression that catches
// "default AS too low for Go runtime" — real child re-exec must return nil.
func TestValidateCheckWasmChildAcceptsMinimalGate(t *testing.T) {
	t.Setenv("HACKME_WASM_VALIDATE_CHILD", "1")
	t.Setenv("HACKME_WASM_VALIDATE_AS_MB", "2048")
	t.Setenv("HACKME_WASM_VALIDATE_CHILD_BUDGET_MB", "384")
	raw, err := hex.DecodeString(MinimalGateWasmHex)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := ValidateCheckWasm(ctx, raw); err != nil {
		t.Fatalf("minimal gate must pass real child validate: %v", err)
	}
}

func TestValidateCheckWasmChildRejectsNameBomb(t *testing.T) {
	t.Setenv("HACKME_WASM_VALIDATE_CHILD", "1")
	t.Setenv("HACKME_WASM_VALIDATE_AS_MB", "2048")
	t.Setenv("HACKME_WASM_VALIDATE_CHILD_BUDGET_MB", "384")
	// Section 0 custom with huge declared name vector — hostile pre-parser or child.
	bomb := append([]byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00},
		// custom section id=0, size large claim via LEB — keep payload small
		0x00, 0x0b, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x01, 0x80, 0x80, 0x80, 0x40,
	)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := ValidateCheckWasm(ctx, bomb); err == nil {
		t.Fatal("name/custom bomb must be rejected")
	}
}

func TestStripWasmCustomOnlyErrors(t *testing.T) {
	// magic + single empty-ish custom section only
	mod := []byte{
		0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
		0x00, 0x04, 0x01, 0x61, 0x00, 0x00, // custom name "a", empty payload
	}
	if _, err := stripWasmCustomSections(mod); err == nil {
		t.Fatal("custom-only module must error, not return original")
	}
}
