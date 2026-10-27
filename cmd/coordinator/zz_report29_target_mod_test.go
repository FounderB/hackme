package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"hackme/internal/chain"
)

func report29WM(t *testing.T, mod uint64) *workManager {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HACKME_COORDINATOR_TARGET_MOD_FILE", filepath.Join(dir, "coordinator_target_mod.txt"))
	return withTestTip(&workManager{
		hybridSignerEnabled:    true,
		hybridSignerStrict:     true,
		hybridRequireFoundSig:  true,
		claimRequirePubKey:     true,
		claimPerMin:            200,
		submitPerMin:           600,
		maxActiveLeases:        500000,
		defaultBatch:           1 << 22,
		maxClaimBatch:          1 << 24,
		leaseSec:               90,
		maxWorkers:             200000,
		maxDedupEntries:        500000,
		targetMod:              mod,
		targetModMin:           2_000_000,
		targetModMax:           1_000_000_000,
		poolRetarget:           true,
		poolRetargetMinSec:     3,
		rewardAuto:             true,
		baseRewardHMC:          0.01,
		foundBonus:             0.01,
		rewardPerM:             1e-05,
		active:                 map[workKey]leaseRecord{},
		worker:                 map[string]workerPayoutStat{},
		acceptedResultHashes:   map[string]struct{}{},
		acceptedFoundNonces:    map[uint64]struct{}{},
		acceptedSubmitNonces:   map[string]struct{}{},
		acceptedSignedPayloads: map[string]struct{}{},
		signedSubmitNonceMax:   map[string]uint64{},
		abuse:                  map[string]workerAbuseState{},
		ipAbuse:                map[string]workerAbuseState{},
		dropReasonCount:        map[string]uint64{},
		supMeta:                map[string]workerSupMeta{},
		pohFormulaVersion:      1,
	})
}

func TestReport29PoolClampKeepsBoundsOnly(t *testing.T) {
	const bad = 2_100_000 // historically unsolvable for 7n+13; Blake3 accepts any M
	if bad%7 != 0 {
		t.Fatal("precondition")
	}
	m := report29WM(t, bad)
	got := m.clampTargetMod(bad)
	if got != bad {
		t.Fatalf("pool clampTargetMod should keep in-bounds M (incl. %%7==0), got %d want %d", got, bad)
	}
	if chain.ClampPoHTargetMod(bad)%7 == 0 {
		t.Fatal("chain sanitizer must still nudge off mod 7 for pre-v3 algebraic PoH")
	}
	if sanitizePoolTargetMod7n13(bad, 2_000_000, 1_000_000_000) != bad {
		t.Fatal("pool sanitize is bounds-only after Blake3 v3")
	}
}

func TestReport29LoadRetargetStaysInBounds(t *testing.T) {
	t.Setenv("HACKME_COORDINATOR_POOL_BLOCK_SEC", "30")
	m := report29WM(t, 2_000_000)
	// report29WM caps max at 1e9 — raise for solvency-scale load M.
	m.targetModMax = poolTargetModMax
	now := int64(1_700_000_000)
	for i := 0; i < 80; i++ {
		now += m.poolRetargetMinSec + 1
		m.maybeRetargetPoolLoadLocked(now, 1.05, 1)
	}
	// 1.05 GH/s × 1e9 × 30s ≈ 3.15e10 (solvency-aligned load).
	if m.targetMod < 10_000_000_000 || m.targetMod > poolTargetModMax {
		t.Fatalf("unexpected converged M=%d", m.targetMod)
	}
}

func TestReport29FoundPayOnAnyInBoundsMod(t *testing.T) {
	m := report29WM(t, 2_000_003) // not multiple of 7 — v1 still works
	m.targetMod = m.clampTargetMod(m.targetMod)
	pub, priv, _ := ed25519.GenerateKey(nil)
	base, size, _, _, _, ok, reason := m.claim("rig-ok", 0)
	if !ok {
		t.Fatalf("claim: %s", reason)
	}
	var nonce uint64
	found := false
	for n := base; n < base+size; n++ {
		if validFoundNonceV1(n, m.targetMod) {
			nonce = n
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no valid nonce in lease for M=%d", m.targetMod)
	}
	req := submitWorkRequest{
		WorkerID: "rig-ok", BaseNonce: base, BatchSize: size,
		Found: true, HashrateGHS: 1.0, FoundNonce: nonce, ResultHash: "r29aa",
		Attempts: 1, SubmitNonce: 1,
		MinerPubKey: hex.EncodeToString(pub), MinerAddress: signerAddr(pub), MinerSigAlg: "ed25519",
	}
	req.MinerSig = hex.EncodeToString(ed25519.Sign(priv, canonicalSubmitBytes(req)))
	accepted, reason2, payout, _, _ := m.submit(req)
	if !accepted || reason2 != "" || payout < m.foundBonus {
		t.Fatalf("must accept found: ok=%v reason=%s payout=%f", accepted, reason2, payout)
	}
}

func TestReport29PersistLoadBoundsOnly(t *testing.T) {
	report29WM(t, 2_100_000)
	path := poolTargetModPersistPath()
	if err := os.WriteFile(path, []byte("2100000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := loadPersistedPoolTargetMod()
	if got != 2_100_000 {
		t.Fatalf("load must keep persisted in-bounds M, got %d", got)
	}
	persistPoolTargetMod(2_100_000)
	got2 := loadPersistedPoolTargetMod()
	if got2 != 2_100_000 {
		t.Fatalf("persist+load must keep M, got %d", got2)
	}
}

func TestReport29StallEasingWithoutFound(t *testing.T) {
	m := report29WM(t, 5_000_003) // high M
	now := int64(1_700_000_000)
	m.targetModUpdatedUnix = now
	m.lastPoolRetargetUnix = now
	m.maybeEasePoolModOnStallLocked(now + chain.PoHRetargetTargetSec*3)
	if m.targetMod != 5_000_003 {
		t.Fatalf("must not ease before stall window, got %d", m.targetMod)
	}
	m.maybeEasePoolModOnStallLocked(now + chain.PoHRetargetTargetSec*6 + 1)
	if m.targetMod >= 5_000_003 {
		t.Fatalf("stall easing without found must lower M, got %d", m.targetMod)
	}
}
