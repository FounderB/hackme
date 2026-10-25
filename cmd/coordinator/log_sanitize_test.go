package main

import "testing"

func TestSanitizeLogTokenStripsControls(t *testing.T) {
	got := sanitizeLogToken("rig-1\nINFO fake")
	if got != "rig-1_INFO fake" {
		t.Fatalf("got %q", got)
	}
	if sanitizeLogToken("") != "?" {
		t.Fatal("empty")
	}
	if sanitizeLogToken("  ok  ") != "ok" {
		t.Fatal("trim")
	}
}
