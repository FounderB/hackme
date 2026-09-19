package poolfuzz

import (
	"path/filepath"
	"testing"
)

func TestCorpusObjectStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	SetCorpusObjectDir(filepath.Join(dir, "corp"))
	t.Cleanup(func() { SetCorpusObjectDir("") })
	payload := make([]byte, 2048)
	for i := range payload {
		payload[i] = byte(i)
	}
	marker, err := writeCorpusObject("seed", "", payload)
	if err != nil {
		t.Fatal(err)
	}
	if !isCorpusObjMarker(marker) {
		t.Fatalf("want marker got %q", marker)
	}
	got, err := readCorpusObject(marker)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(payload) || got[0] != payload[0] || got[len(got)-1] != payload[len(payload)-1] {
		t.Fatal("hydrate mismatch")
	}
	small := []byte("tiny")
	m2, err := writeCorpusObject("seed", "", small)
	if err != nil || string(m2) != "tiny" {
		t.Fatalf("small should stay inline: %v %q", err, m2)
	}
}
