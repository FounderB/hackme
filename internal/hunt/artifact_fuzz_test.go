package hunt

import "testing"

func FuzzSafeHarnessFetchURL(f *testing.F) {
	f.Add("/api/fuzz/pool/hunt/harness/abc12345")
	f.Add("//api/fuzz/pool/hunt/harness/00000000")
	f.Add("https://evil.example/api/fuzz/pool/hunt/harness/abc12345")
	f.Add("/pool/coordinator/api/fuzz/pool/hunt/harness/deadbeef")
	f.Fuzz(func(t *testing.T, raw string) {
		ok := SafeHarnessFetchURL(raw)
		// Scheme-relative must never pass.
		if len(raw) >= 2 && raw[0] == '/' && raw[1] == '/' && ok {
			t.Fatalf("scheme-relative accepted: %q", raw)
		}
	})
}
