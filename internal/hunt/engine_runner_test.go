package hunt

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestIsLibFuzzerPreflightErr(t *testing.T) {
	if !isLibFuzzerPreflightErr(errors.New("hunt: libfuzzer import: clang required")) {
		t.Fatal("clang preflight")
	}
	if !isLibFuzzerPreflightErr(errors.New("clang subprocess libfuzzer: exit 1")) {
		t.Fatal("build preflight")
	}
	if isLibFuzzerPreflightErr(errors.New("hunt: libfuzzer import: no corpus files in /tmp/x")) {
		t.Fatal("empty import after session must NOT soft-skip")
	}
	if isLibFuzzerPreflightErr(errors.New("hunt: libfuzzer persist: corpus still empty after 60s")) {
		t.Fatal("empty persist after wall must NOT soft-skip")
	}
	if isLibFuzzerPreflightErr(nil) {
		t.Fatal("nil")
	}
}

func TestSeedPersistentCorpusBootstrapsEmpty(t *testing.T) {
	dir := t.TempDir()
	corpus := filepath.Join(dir, "corpus")
	if err := os.MkdirAll(corpus, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := seedPersistentCorpus(dir, "jsmn", corpus); err != nil {
		t.Fatal(err)
	}
	n, err := countCorpusFiles(corpus)
	if err != nil || n < 3 {
		t.Fatalf("want boot seeds, got n=%d err=%v", n, err)
	}
}

func TestRunLibFuzzerEngineShardEmptyImportStillRan(t *testing.T) {
	// Without a real target/manifest this hits preflight — ensure helper classification
	// matches the soft-skip contract used by runLibFuzzerEngineShard.
	err := errors.New("hunt: libfuzzer persist: no corpus files in /x (INFO: Running with entropic)")
	if isLibFuzzerPreflightErr(err) {
		t.Fatal("session empty corpus must remain Ran=true (CLEAN floor), not soft_skip")
	}
}
