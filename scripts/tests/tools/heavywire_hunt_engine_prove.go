// Blind heavywire Hunt engine prove (pack seeds + dict, no private heap seed).
//go:build ignore

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"hackme/internal/fuzzengine"
	"hackme/internal/hunt"
)

func main() {
	pin := flag.String("pin", "", "inventory pin path (public pack)")
	wall := flag.Int("wall", 180, "libFuzzer wall seconds")
	out := flag.String("out", "", "score json")
	flag.Parse()
	if strings.TrimSpace(*pin) == "" {
		fmt.Fprintln(os.Stderr, "need -pin")
		os.Exit(2)
	}
	repo := hunt.RepoRoot()
	target := "inv_heavywire_blind_easy"
	cfg := map[string]any{
		"upstream_target_id": target,
		"hunt_package":       "hunt_standard",
		"hunt_source":        "inventory",
		"hunt_pin_path":      *pin,
		"source_rel":         "fuzz/fuzz_parse.c",
	}
	hunt.ApplyPackageDepthDefaults(cfg, "hunt_standard", true)
	if err := hunt.ApplyInventoryPackAssets(cfg, repo, target, *pin, "fuzz/fuzz_parse.c"); err != nil {
		fmt.Fprintf(os.Stderr, "pack assets: %v\n", err)
		os.Exit(1)
	}
	seeds := fuzzengine.ParseByteCorpus(cfg)
	var dict []byte
	if s, ok := cfg["libfuzzer_dict"].(string); ok {
		dict = []byte(s)
	}
	fmt.Fprintf(os.Stderr, "seeds=%d dict_bytes=%d wall=%d repo=%s\n", len(seeds), len(dict), *wall, repo)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*wall+180)*time.Second)
	defer cancel()

	res := hunt.RunEngineShard(ctx, hunt.EngineShardOpts{
		RepoRoot:  repo,
		TargetID:  target,
		Engine:    hunt.EngineLibFuzzer,
		WallSec:   *wall,
		SeedFiles: seeds,
		DictBytes: dict,
		PinPath:   *pin,
		SourceRel: "fuzz/fuzz_parse.c",
	})

	corpusDir := hunt.PersistentLibFuzzerCorpusDir(repo, target)
	after, _ := countDir(corpusDir)
	crashDir := filepath.Join(filepath.Dir(corpusDir), "crashes")
	crashes := listCrashes(crashDir, 32)
	asan := false
	if b, rerr := os.ReadFile(filepath.Join(filepath.Dir(corpusDir), "session.stderr")); rerr == nil {
		s := string(b)
		if strings.Contains(s, "heap-buffer-overflow") || strings.Contains(s, "AddressSanitizer") {
			asan = true
		}
	}
	if len(crashes) > 0 || len(res.CrashFiles) > 0 {
		asan = true
	}
	if len(crashes) == 0 {
		crashes = res.CrashFiles
	}
	payload := map[string]any{
		"ok":            res.Ran && (asan || len(crashes) > 0 || res.Err == ""),
		"engine":        "libfuzzer",
		"seeds":         len(seeds),
		"dict_bytes":    len(dict),
		"wall_sec":      *wall,
		"new_corpus":    res.NewCorpus,
		"corpus_after":  after,
		"crash_files":   len(crashes),
		"asan_hit":      asan,
		"err":           errString(res.Err),
		"skipped":       res.SkippedReason,
		"ran":           res.Ran,
		"engine_version": res.EngineVersion,
	}
	if asan || len(crashes) > 0 {
		payload["ok"] = true
	}
	b, _ := json.MarshalIndent(payload, "", "  ")
	if *out != "" {
		_ = os.WriteFile(*out, b, 0o644)
	}
	fmt.Println(string(b))
	if !asan && len(crashes) == 0 {
		os.Exit(1)
	}
}

func errString(err string) string {
	return strings.TrimSpace(err)
}

func countDir(dir string) (int, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range ents {
		if !e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			n++
		}
	}
	return n, nil
}

func listCrashes(dir string, max int) []string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if len(out) >= max {
			break
		}
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	return out
}
