package logsafe

import "testing"

func TestIDStripsControls(t *testing.T) {
	got := ID("rig-01\nINFO fake")
	if stringsContainsNL(got) {
		t.Fatalf("newline leaked: %q", got)
	}
	if got == "" || got == "-" {
		t.Fatalf("empty: %q", got)
	}
}

func TestIDEmpty(t *testing.T) {
	if ID("  ") != "-" {
		t.Fatal(ID("  "))
	}
}

func stringsContainsNL(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 {
			return true
		}
	}
	return false
}
