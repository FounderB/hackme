package hunt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectCompanionSourcesIncludesSrc(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "fuzz"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fuzz", "fuzz_parse.c"),
		[]byte("int LLVMFuzzerTestOneInput(const unsigned char*,unsigned long){return 0;}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "lib.c"),
		[]byte("int hw_parse(void){return 1;}"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := collectCompanionSources(dir, "fuzz/fuzz_parse.c")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, rel := range got {
		if rel == "src/lib.c" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected src/lib.c companion, got %v", got)
	}
}

func TestImportInventoryPackSeedsAndDict(t *testing.T) {
	repo := t.TempDir()
	pin := t.TempDir()
	if err := os.MkdirAll(filepath.Join(pin, "corpus"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(pin, "fuzz"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pin, "corpus", "seed_tlv_raw.bin"), []byte("HWIR\x10\x00\x01"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pin, "corpus", "seed_bug_heap.bin"), []byte("CRASH"), 0o644); err != nil {
		t.Fatal(err)
	}
	dictBody := "hw_magic=\"HWIR\"\n"
	if err := os.WriteFile(filepath.Join(pin, "fuzz", "heavywire.dict"), []byte(dictBody), 0o644); err != nil {
		t.Fatal(err)
	}
	target := "inv_fuzz_parse_c"
	n, err := ImportInventoryPackSeeds(repo, target, pin)
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("imported=%d", n)
	}
	loaded, err := LoadLibFuzzerSeedFiles(LibFuzzerSeedDir(repo, target), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || string(loaded[0]) != "HWIR\x10\x00\x01" {
		t.Fatalf("seeds=%v", loaded)
	}
	dict, rel, err := LoadInventoryLibFuzzerDict(pin, "fuzz/fuzz_parse.c")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dict), "HWIR") || rel == "" {
		t.Fatalf("dict=%q rel=%q", dict, rel)
	}
	cfg := map[string]any{}
	if err := ApplyInventoryPackAssets(cfg, repo, target, pin, "fuzz/fuzz_parse.c"); err != nil {
		t.Fatal(err)
	}
	if LibFuzzerDictFromConfig(cfg) == nil {
		t.Fatal("expected libfuzzer_dict in cfg")
	}
	raw, ok := cfg["seed_byte_corpus"].([]any)
	if !ok || len(raw) < 1 {
		t.Fatalf("seed_byte_corpus=%v", cfg["seed_byte_corpus"])
	}
}
