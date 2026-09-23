package sandbox

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

// Tight check(n) loop. Timeout must release the slot (report #15).
const slowCheckWasmHex = "0061736d0100000001060160017e017f0302010007090105636865636b00000a170115000340200042017d210020004200550d000b41010b"

func TestWasmLoopHonorsCheckTimeout(t *testing.T) {
	raw, err := hex.DecodeString(strings.ReplaceAll(slowCheckWasmHex, " ", ""))
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateCheckWasm(context.Background(), raw); err != nil {
		t.Fatalf("module rejected: %v", err)
	}
	if _, err := InvokeCheck(context.Background(), raw, 1000); err != nil {
		t.Fatalf("fast invoke: %v", err)
	}
	start := time.Now()
	_, err = InvokeCheckOutcome(context.Background(), raw, uint64(1)<<28)
	dur := time.Since(start)
	if err == nil {
		t.Fatalf("huge loop returned success in %v", dur)
	}
	if dur > 3*time.Second {
		t.Fatalf("timeout did not release the slot: %v err=%v", dur, err)
	}
}
