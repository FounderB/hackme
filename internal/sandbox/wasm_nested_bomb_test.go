package sandbox

import (
	"context"
	"encoding/hex"
	"testing"
	"time"
)

func appendLEB(buf []byte, v uint32) []byte {
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			b |= 0x80
			buf = append(buf, b)
			continue
		}
		return append(buf, b)
	}
}

func TestRejectWasmInflatedTypeParams(t *testing.T) {
	body := []byte{0x01, 0x60} // count=1, functype
	body = appendLEB(body, 50_000_000)
	body = append(body, 0x00)
	sec := []byte{0x01}
	sec = appendLEB(sec, uint32(len(body)))
	sec = append(sec, body...)
	mod := append([]byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}, sec...)
	if err := rejectWasmHostileSections(mod); err == nil {
		t.Fatal("expected inflated type params rejection")
	}
}

func TestRejectWasmInflatedCodeLocals(t *testing.T) {
	fnBody := appendLEB(nil, 40_000_000)
	fnBody = append(fnBody, 0x00)
	wrapped := appendLEB(nil, 1)
	wrapped = appendLEB(wrapped, uint32(len(fnBody)))
	wrapped = append(wrapped, fnBody...)
	sec := []byte{0x0a}
	sec = appendLEB(sec, uint32(len(wrapped)))
	sec = append(sec, wrapped...)
	mod := append([]byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}, sec...)
	if err := rejectWasmHostileSections(mod); err == nil {
		t.Fatal("expected inflated locals rejection")
	}
}

func TestStripWasmCustomSections(t *testing.T) {
	customBody := []byte{0x04, 'n', 'a', 'm', 'e', 0x00}
	sec0 := []byte{0x00}
	sec0 = appendLEB(sec0, uint32(len(customBody)))
	sec0 = append(sec0, customBody...)
	raw, err := hex.DecodeString(MinimalGateWasmHex)
	if err != nil {
		t.Fatal(err)
	}
	mod := append(append([]byte{}, raw[:8]...), append(sec0, raw[8:]...)...)
	out, err := stripWasmCustomSections(mod)
	if err != nil {
		t.Fatal(err)
	}
	if err := rejectWasmHostileSections(out); err != nil {
		t.Fatalf("stripped module rejected: %v", err)
	}
	pos := 8
	for pos < len(out) {
		if out[pos] == 0 {
			t.Fatal("custom section still present")
		}
		pos++
		sz, err := readU32LEB(out, &pos)
		if err != nil {
			t.Fatal(err)
		}
		pos += int(sz)
	}
}

func TestValidateCheckWasmRejectsTypeBombFast(t *testing.T) {
	body := []byte{0x01, 0x60}
	body = appendLEB(body, 80_000_000)
	body = append(body, make([]byte, 8)...)
	sec := []byte{0x01}
	sec = appendLEB(sec, uint32(len(body)))
	sec = append(sec, body...)
	mod := append([]byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}, sec...)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	err := ValidateCheckWasm(ctx, mod)
	if err == nil {
		t.Fatal("expected reject")
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatalf("too slow: %s", time.Since(start))
	}
}
