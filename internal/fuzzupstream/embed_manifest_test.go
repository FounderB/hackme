package fuzzupstream

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadManifestFallsBackToEmbed(t *testing.T) {
	dir := t.TempDir()
	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest empty root: %v", err)
	}
	if m == nil || len(m.Targets) == 0 {
		t.Fatal("expected embedded catalog targets")
	}
	up := filepath.Join(dir, "upstream")
	if err := os.MkdirAll(up, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(up, "oss_cve_targets.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(dir); err == nil {
		t.Fatal("expected JSON error for corrupt on-disk manifest")
	}
}
