// Coverage-engine Hunt bench: libFuzzer / AFL++ lease windows vs raw stats.
//go:build ignore

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"hackme/internal/hunt"
)

func main() {
	target := flag.String("target", "cjson", "catalog target")
	engine := flag.String("engine", "libfuzzer", "libfuzzer|aflpp")
	wall := flag.Int("wall", 90, "wall seconds")
	out := flag.String("out", "", "output json")
	flag.Parse()

	repo := hunt.RepoRoot()
	if repo == "" {
		repo, _ = os.Getwd()
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*wall+60)*time.Second)
	defer cancel()

	start := time.Now()
	res := hunt.RunEngineShard(ctx, hunt.EngineShardOpts{
		RepoRoot: repo,
		TargetID: *target,
		Engine:   *engine,
		WallSec:  *wall,
	})
	elapsed := time.Since(start).Seconds()
	payload := map[string]any{
		"ok":              res.Err == "",
		"target":          *target,
		"engine":          res.Engine,
		"engine_version":  res.EngineVersion,
		"wall_sec":        *wall,
		"elapsed_sec":     elapsed,
		"ran":             res.Ran,
		"corpus_before":   res.CorpusBefore,
		"corpus_after":    res.CorpusAfter,
		"new_corpus":      res.NewCorpus,
		"crash_files":     len(res.CrashFiles),
		"err":             res.Err,
		"stamp_utc":       time.Now().UTC().Format(time.RFC3339),
		"note":            "coverage engine lease — exec/s reported by engine logs, not byte-shard iterations",
	}
	b, _ := json.MarshalIndent(payload, "", "  ")
	if *out != "" {
		_ = os.WriteFile(*out, b, 0o644)
	}
	fmt.Println(string(b))
	if res.Err != "" && !res.Ran {
		os.Exit(1)
	}
}
