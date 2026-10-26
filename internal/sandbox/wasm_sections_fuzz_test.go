package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func FuzzRejectWasmHostileSections(f *testing.F) {
	f.Add([]byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00})
	f.Add([]byte{
		0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
		0x07, 0x08, 0x82, 0xfd, 0xed, 0x65, 0x00, 0x00, 0x00, 0x00,
	})
	// Inflated type params seed.
	body := appendLEB([]byte{0x01, 0x60}, 50_000_000)
	body = append(body, 0x00)
	sec := appendLEB([]byte{0x01}, uint32(len(body)))
	sec = append(sec, body...)
	f.Add(append([]byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}, sec...))

	f.Fuzz(func(t *testing.T, wasm []byte) {
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		_ = rejectWasmHostileSections(wasm)
		runtime.ReadMemStats(&after)
		// Pre-parser must not allocate hundreds of MB on crafted input.
		const maxDelta = 64 << 20 // 64 MiB
		if after.HeapAlloc > before.HeapAlloc {
			delta := after.HeapAlloc - before.HeapAlloc
			if delta > maxDelta {
				t.Fatalf("rejectWasmHostileSections heap delta %d > %d", delta, maxDelta)
			}
		}
	})
}

// TestValidateCheckWasmPeakMemoryChild is an opt-in soak: builds the real
// node binary child path against nested bombs and asserts RSS stays bounded.
func TestValidateCheckWasmPeakMemoryChild(t *testing.T) {
	if os.Getenv("HACKME_WASM_MEM_SOAK") != "1" {
		t.Skip("set HACKME_WASM_MEM_SOAK=1 for child RSS soak")
	}
	t.Setenv("HACKME_WASM_VALIDATE_CHILD", "1")
	// Floor is minWasmValidateASMB (Go VAS); bombs must still be rejected fast.
	t.Setenv("HACKME_WASM_VALIDATE_AS_MB", "1024")
	bombs := [][]byte{
		func() []byte {
			body := appendLEB([]byte{0x01, 0x60}, 80_000_000)
			body = append(body, make([]byte, 16)...)
			sec := appendLEB([]byte{0x01}, uint32(len(body)))
			sec = append(sec, body...)
			return append([]byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}, sec...)
		}(),
		func() []byte {
			fn := appendLEB(nil, 40_000_000)
			fn = append(fn, 0x00)
			w := appendLEB(nil, 1)
			w = appendLEB(w, uint32(len(fn)))
			w = append(w, fn...)
			sec := appendLEB([]byte{0x0a}, uint32(len(w)))
			sec = append(sec, w...)
			return append([]byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}, sec...)
		}(),
	}
	dir := t.TempDir()
	for i, bomb := range bombs {
		p := filepath.Join(dir, "bomb.bin")
		_ = os.WriteFile(p, bomb, 0o600)
		start := time.Now()
		err := ValidateCheckWasm(context.Background(), bomb)
		if err == nil {
			t.Fatalf("bomb %d accepted", i)
		}
		if time.Since(start) > 5*time.Second {
			t.Fatalf("bomb %d too slow: %s", i, time.Since(start))
		}
	}
}
