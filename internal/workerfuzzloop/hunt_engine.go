package workerfuzzloop

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"hackme/internal/fuzzengine"
	"hackme/internal/hunt"
	"hackme/internal/poolfuzz"
)

// DetectHuntEngines returns locally available Hunt engines for claim advertising.
// Always includes byte (ASAN L1). Adds libfuzzer when clang is present; aflpp when afl-fuzz is bundled.
func DetectHuntEngines() []string {
	out := []string{hunt.EngineByte}
	if ok, _ := hunt.RunnerAvailable(hunt.EngineLibFuzzer); ok {
		out = append(out, hunt.EngineLibFuzzer)
	}
	if ok, _ := hunt.RunnerAvailable(hunt.EngineAFLPP); ok {
		out = append(out, hunt.EngineAFLPP)
	}
	return out
}

// HuntEnginesClaimCSV is the claim capability string.
func HuntEnginesClaimCSV() string {
	return strings.Join(DetectHuntEngines(), ",")
}

// HuntEngineResult is one coverage-guided Hunt shard outcome for submit.
type HuntEngineResult struct {
	CheckResult   int32
	DurationMS    int
	Trap          string
	ExecDone      int
	Engine        string
	EngineVersion string
	CorpusDeltas  int
	Crashes       int
	SoftSkip      bool
	SkipReason    string
}

// RunHuntEngineShard runs libFuzzer/AFL++ for the claim lease window, uploads corpus_delta,
// and returns a CLEAN (or crash-trap) submit posture. SoftSkip=true → release lease (no submit).
func RunHuntEngineShard(ctx context.Context, run ResearchSlotRun, cr ClaimResp) HuntEngineResult {
	engine := hunt.ParseHuntEngine(cr.HuntEngine)
	out := HuntEngineResult{Engine: engine, ExecDone: 1}
	if !hunt.IsCoverageEngine(engine) {
		out.SoftSkip = true
		out.SkipReason = "not_coverage_engine"
		return out
	}
	if ok, reason := hunt.RunnerAvailable(engine); !ok {
		out.SoftSkip = true
		out.SkipReason = reason
		return out
	}
	wall := cr.EngineLeaseSec
	if wall <= 0 {
		wall = hunt.EngineLeaseSecFromConfig(nil, engine)
	}
	start := time.Now()
	var seedFiles [][]byte
	if h := strings.TrimSpace(cr.InputBytesHex); h != "" {
		if b, err := decodeHex(h); err == nil && len(b) > 0 {
			seedFiles = append(seedFiles, b)
		}
	}
	for _, m := range cr.CorpusSeeds {
		if b, ok := corpusSeedBytes(m); ok {
			seedFiles = append(seedFiles, b)
		}
	}
	repoRoot := strings.TrimSpace(run.RepoRoot)
	if repoRoot == "" {
		repoRoot = hunt.RepoRoot()
	}
	res := hunt.RunEngineShard(ctx, hunt.EngineShardOpts{
		RepoRoot:  repoRoot,
		TargetID:  strings.TrimSpace(cr.UpstreamTargetID),
		Engine:    engine,
		WallSec:   wall,
		SeedFiles: seedFiles,
	})
	out.EngineVersion = res.EngineVersion
	out.DurationMS = int(time.Since(start).Milliseconds())
	if !res.Ran {
		out.SoftSkip = true
		out.SkipReason = res.SkippedReason
		if res.Err != "" {
			out.SkipReason = res.SkippedReason + ":" + res.Err
		}
		return out
	}

	// Upload corpus delta while lease is held (same API as research slot).
	deltas, crashes := collectEngineDelta(res, 32)
	out.CorpusDeltas = len(deltas)
	out.Crashes = len(crashes)
	if len(deltas) > 0 || len(crashes) > 0 {
		ns := poolfuzz.ResearchNamespaceForTarget(cr.UpstreamTargetID)
		cl := run.HTTPClient
		if cl == nil {
			cl = http.DefaultClient
		}
		_, findings, upErr := uploadResearchCorpusDelta(ctx, cl, run.CoordURL, run.Token, run.WorkerID, run.MinerAddr,
			cr.CampaignID, cr.ItemID, ns, deltas, crashes)
		if upErr != nil {
			fmt.Fprintf(os.Stderr, "hunt_engine: corpus_delta: %v\n", upErr)
		} else if findings > 0 {
			out.CheckResult = 1
			out.Trap = "hunt_crash:engine_corpus_delta"
		}
	}
	return out
}

func collectEngineDelta(res hunt.EngineShardResult, maxFiles int) (seeds []fuzzengine.PoolCorpusSeed, crashes [][]byte) {
	if maxFiles < 1 {
		maxFiles = 32
	}
	dir := res.CorpusDir
	if dir == "" {
		return nil, nil
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil
	}
	// Prefer newest files up to maxFiles as delta candidates (session-local growth).
	n := 0
	for i := len(ents) - 1; i >= 0 && n < maxFiles; i-- {
		e := ents[i]
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil || len(b) == 0 || len(b) > 65536 {
			continue
		}
		seeds = append(seeds, fuzzengine.PoolCorpusSeed{
			Input:      fuzzengine.PackInputBytesToU64(b),
			InputBytes: append([]byte(nil), b...),
			Energy:     4,
		})
		n++
	}
	for _, p := range res.CrashFiles {
		if len(crashes) >= 8 {
			break
		}
		b, err := os.ReadFile(p)
		if err != nil || len(b) == 0 || len(b) > 65536 {
			continue
		}
		crashes = append(crashes, b)
	}
	return seeds, crashes
}

func corpusSeedBytes(m map[string]any) ([]byte, bool) {
	if m == nil {
		return nil, false
	}
	if h, ok := m["input_bytes_hex"].(string); ok {
		b, err := decodeHex(h)
		if err == nil && len(b) > 0 {
			return b, true
		}
	}
	return nil, false
}

func decodeHex(h string) ([]byte, error) {
	h = strings.TrimSpace(h)
	if h == "" {
		return nil, fmt.Errorf("empty")
	}
	return hex.DecodeString(h)
}
