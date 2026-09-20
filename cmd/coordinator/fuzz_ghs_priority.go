package main

import (
	"hash/fnv"
	"os"
	"strconv"
	"strings"
	"time"
)

// Fuzz GHS ↔ Dig/Hunt coupling (soft priority + fleet capacity).
// PoH GH/s still pays mining; escrow still pays fuzz. This only steers claim admit
// and publishes honest ETA capacity from live hybrid GHS.

const (
	fuzzGHSHybridMinGHS     = 1.0 // worker counts as hybrid capacity above this
	fuzzGHSPriorityStaleSec = 90  // recent PoH/fuzz heartbeat window
	fuzzGHSDigOnlyAdmitPct  = 25  // dig-only admit share when hybrids are online
)

func fuzzClaimGHSPriorityEnabled() bool {
	v := strings.TrimSpace(os.Getenv("HACKME_FUZZ_CLAIM_GHS_PRIORITY"))
	if v == "" {
		return true // default ON: real GPU GHS prefers Dig/Hunt leases
	}
	switch strings.ToLower(v) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

func fuzzGHSDigOnlyAdmitPercent() int {
	v := strings.TrimSpace(os.Getenv("HACKME_FUZZ_CLAIM_GHS_DIG_ONLY_PCT"))
	if v == "" {
		return fuzzGHSDigOnlyAdmitPct
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fuzzGHSDigOnlyAdmitPct
	}
	if n < 0 {
		return 0
	}
	if n > 100 {
		return 100
	}
	return n
}

// allowFuzzClaimByGHS soft-defers dig-only (≈0 GH/s) workers when live hybrid GHS is on the pool.
// When no hybrid capacity is online, dig-only fleets keep full access (bootstrap / dedicated dig).
func (m *workManager) allowFuzzClaimByGHS(workerID string, now int64) (bool, string) {
	if m == nil || !fuzzClaimGHSPriorityEnabled() {
		return true, ""
	}
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return true, ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	cap := m.fuzzFleetCapacityUnlocked(now)
	my := 0.0
	if st, ok := m.worker[workerID]; ok {
		my = effectiveWorkerHashrateGHS(st)
	}
	if cap.HybridWorkersOnline == 0 || cap.FleetHashrateGHS < fuzzGHSHybridMinGHS {
		return true, ""
	}
	if my >= fuzzGHSHybridMinGHS {
		return true, ""
	}
	admit := fuzzGHSDigOnlyAdmitPercent()
	if admit >= 100 {
		return true, ""
	}
	if admit <= 0 {
		return false, "ghs_priority_defer"
	}
	// Deterministic slot per worker+10s window so diggers still get a fair minority share.
	slot := int(hashWorkerWindow(workerID, now/10) % 100)
	if slot < admit {
		return true, ""
	}
	return false, "ghs_priority_defer"
}

type fuzzFleetCapacity struct {
	FleetHashrateGHS     float64 `json:"fleet_hashrate_gh_s"`
	HybridWorkersOnline  int     `json:"hybrid_workers_online"`
	DigOnlyWorkersOnline int     `json:"dig_only_workers_online"`
	WorkersOnline        int     `json:"workers_online"`
	EstShardsPerHour     float64 `json:"est_shards_per_hour"`
	EstDigRunsPerHour    float64 `json:"est_dig_runs_per_hour"`
	GHSPriorityEnabled   bool    `json:"ghs_priority_enabled"`
	Note                 string  `json:"note,omitempty"`
}

func (m *workManager) fuzzFleetCapacity(now int64) fuzzFleetCapacity {
	if m == nil {
		return fuzzFleetCapacity{Note: "no work manager"}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.fuzzFleetCapacityUnlocked(now)
}

func (m *workManager) fuzzFleetCapacityUnlocked(now int64) fuzzFleetCapacity {
	out := fuzzFleetCapacity{
		GHSPriorityEnabled: fuzzClaimGHSPriorityEnabled(),
		Note:               "hybrid GPU GHS feeds Dig/Hunt capacity; ASAN/WASM still CPU",
	}
	if now <= 0 {
		now = time.Now().Unix()
	}
	for _, st := range m.worker {
		if st.LastSeenUnix <= 0 || (now-st.LastSeenUnix) > fuzzGHSPriorityStaleSec {
			continue
		}
		out.WorkersOnline++
		gh := effectiveWorkerHashrateGHS(st)
		if gh >= fuzzGHSHybridMinGHS {
			out.HybridWorkersOnline++
			out.FleetHashrateGHS += gh
		} else {
			out.DigOnlyWorkersOnline++
		}
	}
	// Heuristic throughput (honest order ETA input, not payout):
	// hybrid digs ~90 shards/h while also mining; dedicated dig ~180 shards/h.
	out.EstShardsPerHour = float64(out.HybridWorkersOnline)*90 + float64(out.DigOnlyWorkersOnline)*180
	out.EstDigRunsPerHour = out.EstShardsPerHour // Dig runs ≈ same claim cadence on pool
	return out
}

// estimateETASeconds maps remaining work units to wall seconds via fleet capacity.
func estimateETASeconds(remaining int, shardsPerHour float64) int64 {
	if remaining <= 0 {
		return 0
	}
	if shardsPerHour < 1 {
		return -1 // unknown / warming
	}
	sec := float64(remaining) / shardsPerHour * 3600.0
	if sec < 1 {
		return 1
	}
	return int64(sec + 0.5)
}

func intFromProgress(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	default:
		return 0
	}
}

func hashWorkerWindow(workerID string, window int64) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(workerID))
	var b [8]byte
	for i := 0; i < 8; i++ {
		b[i] = byte(window >> (8 * i))
	}
	_, _ = h.Write(b[:])
	return h.Sum32()
}
