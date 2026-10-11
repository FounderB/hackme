package poolfuzz

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"hackme/internal/fuzzengine"
	"hackme/internal/hunt"
)

// Hunt engine pay kinds (coordinator-proven only — miner edges_touched ignored).
const (
	HuntEnginePayNone  = "none"  // reject / no settle
	HuntEnginePayFloor = "floor" // CLEAN without proven delta — pennies / no full PayRun
	HuntEnginePayEdges = "edges" // proven new coverage edges
	HuntEnginePayCrash = "crash" // ASAN-verified crash
)

// leaseProvenTrack accumulates coordinator-proven novelty while a Hunt lease is held.
type leaseProvenTrack struct {
	SeedsAccepted int
	ProvenEdges   int
	CrashesOK     int
	Engine        string
	EngineVersion string
}

var (
	leaseProven   sync.Map // key campaignID:itemID → *leaseProvenTrack
	leaseProvenMu sync.Mutex
)

func leaseProvenKey(campaignID string, itemID int64) string {
	return fmt.Sprintf("%s:%d", strings.TrimSpace(campaignID), itemID)
}

func noteLeaseProven(campaignID string, itemID int64, seeds, edges, crashes int, engine, version string) {
	key := leaseProvenKey(campaignID, itemID)
	leaseProvenMu.Lock()
	defer leaseProvenMu.Unlock()
	var tr *leaseProvenTrack
	if v, ok := leaseProven.Load(key); ok {
		tr = v.(*leaseProvenTrack)
	} else {
		tr = &leaseProvenTrack{}
		leaseProven.Store(key, tr)
	}
	tr.SeedsAccepted += seeds
	tr.ProvenEdges += edges
	tr.CrashesOK += crashes
	if engine != "" {
		tr.Engine = engine
	}
	if version != "" {
		tr.EngineVersion = version
	}
}

func takeLeaseProven(campaignID string, itemID int64) leaseProvenTrack {
	key := leaseProvenKey(campaignID, itemID)
	v, ok := leaseProven.LoadAndDelete(key)
	if !ok {
		return leaseProvenTrack{}
	}
	return *v.(*leaseProvenTrack)
}

func peekLeaseProven(campaignID string, itemID int64) leaseProvenTrack {
	key := leaseProvenKey(campaignID, itemID)
	v, ok := leaseProven.Load(key)
	if !ok {
		return leaseProvenTrack{}
	}
	tr := v.(*leaseProvenTrack)
	return *tr
}

func clearLeaseProven(campaignID string, itemID int64) {
	leaseProven.Delete(leaseProvenKey(campaignID, itemID))
}

// DecideHuntEnginePay chooses settle posture from coordinator-proven work only.
// Miner-reported edges_touched must not influence this decision.
func DecideHuntEnginePay(provenEdges, seedsAccepted, crashesOK int, minerEdges int, minerEdgesOK bool) (kind string, fullPay bool) {
	_ = minerEdges
	_ = minerEdgesOK // explicitly untrusted
	if crashesOK > 0 {
		return HuntEnginePayCrash, true
	}
	if provenEdges > 0 {
		return HuntEnginePayEdges, true
	}
	if seedsAccepted > 0 && provenEdges == 0 {
		// Seeds accepted but no new edges after coordinator replay — floor.
		return HuntEnginePayFloor, false
	}
	// CLEAN without corpus delta — pennies / floor (no full shard PayRun).
	return HuntEnginePayFloor, false
}

// HuntEngineEnabled is true unless HACKME_HUNT_COVERAGE_ENGINE=0.
func HuntEngineEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("HACKME_HUNT_COVERAGE_ENGINE")))
	if v == "" {
		return true
	}
	switch v {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// HuntEngineAllowByteFallback is true when coverage engine soft-skip may fall back to byte Hunt.
// Default ON so fleets without clang still mine Hunt; set 0 to require coverage runners.
func HuntEngineAllowByteFallback() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("HACKME_HUNT_ENGINE_BYTE_FALLBACK")))
	if v == "" {
		return true
	}
	switch v {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// WorkerHuntEngines parses claim capability list (comma-separated).
func WorkerHuntEngines(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, p := range parts {
		e := hunt.ParseHuntEngine(p)
		if seen[e] {
			continue
		}
		seen[e] = true
		out = append(out, e)
	}
	return out
}

// WorkerSupportsHuntEngine reports whether advertised engines include want.
func WorkerSupportsHuntEngine(advertised []string, want string) bool {
	want = hunt.ParseHuntEngine(want)
	if len(advertised) == 0 {
		// Legacy workers: libfuzzer_oneshot harness implies byte + oneshot ASAN,
		// not necessarily coverage LF shard. Only byte is assumed.
		return want == hunt.EngineByte
	}
	for _, e := range advertised {
		if hunt.ParseHuntEngine(e) == want {
			return true
		}
	}
	return false
}

// ProveHuntCorpusSeedEdges replays seed bytes once under the Hunt harness and
// records coverage. Returns how many seeds produced a new edge (coordinator truth).
// Miner edges_touched is never consulted.
func (s *Service) ProveHuntCorpusSeedEdges(ctx context.Context, campaignID string, cfg map[string]any, seeds []fuzzengine.PoolCorpusSeed, now int64) (provenEdges int, err error) {
	if s == nil || !IsHuntCampaign(cfg) {
		return 0, nil
	}
	maxProve := envIntClamped("HACKME_HUNT_ENGINE_PROVE_MAX", 16, 1, 64)
	n := 0
	for _, seed := range seeds {
		if n >= maxProve {
			break
		}
		b := seed.InputBytes
		if len(b) == 0 {
			continue
		}
		n++
		newEdge, _, cerr := s.recordCoverage(ctx, campaignID, cfg, seed.Input, b, nil, now)
		if cerr != nil {
			return provenEdges, cerr
		}
		if newEdge {
			provenEdges++
		}
	}
	return provenEdges, nil
}

// huntEnginePayStats exposed for pool stats / soak.
var (
	huntEnginePayFloorN atomic.Uint64
	huntEnginePayFullN  atomic.Uint64
)

func noteHuntEnginePay(kind string, full bool) {
	if full {
		huntEnginePayFullN.Add(1)
	} else if kind == HuntEnginePayFloor {
		huntEnginePayFloorN.Add(1)
	}
}

// HuntEnginePayStats returns counters for ops.
func HuntEnginePayStats() map[string]any {
	return map[string]any{
		"coverage_engine_enabled": HuntEngineEnabled(),
		"byte_fallback":           HuntEngineAllowByteFallback(),
		"pay_floor_n":             huntEnginePayFloorN.Load(),
		"pay_full_n":              huntEnginePayFullN.Load(),
	}
}
