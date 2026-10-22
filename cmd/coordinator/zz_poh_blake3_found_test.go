package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"path/filepath"
	"testing"

	"hackme/internal/chain"
)

func TestBlake3FoundRejectsAlgebraicAndWrongAddr(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HACKME_COORDINATOR_TARGET_MOD_FILE", filepath.Join(dir, "mod.txt"))
	prev := "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	mod := uint64(1021) // small M for fast Blake3 hits in tests
	pub, priv, _ := ed25519.GenerateKey(nil)
	addr := signerAddr(pub)
	m := &workManager{
		hybridSignerEnabled: true, hybridSignerStrict: true, hybridRequireFoundSig: true,
		claimRequirePubKey: true, claimPerMin: 200, submitPerMin: 600,
		maxActiveLeases: 1000, defaultBatch: 1 << 20, maxClaimBatch: 1 << 22,
		leaseSec: 90, maxWorkers: 1000, maxDedupEntries: 10000,
		targetMod: mod, targetModMin: 251, targetModMax: 1_000_000_000,
		foundBonus: 0.01, rewardPerM: 1e-05, baseRewardHMC: 0.01,
		active: map[workKey]leaseRecord{}, worker: map[string]workerPayoutStat{},
		acceptedResultHashes: map[string]struct{}{}, acceptedFoundNonces: map[uint64]struct{}{},
		acceptedSubmitNonces: map[string]struct{}{}, acceptedSignedPayloads: map[string]struct{}{},
		signedSubmitNonceMax: map[string]uint64{}, abuse: map[string]workerAbuseState{},
		ipAbuse: map[string]workerAbuseState{}, dropReasonCount: map[string]uint64{},
		supMeta: map[string]workerSupMeta{},
		tipHash: prev, tipHeight: 500_000, pohFormulaVersion: 3,
		pohFormula: chain.PoHFormulaLabelForKind(chain.TaskKindSyntheticPoHV3),
		pohActivationHeight: 500_000,
	}
	m.worker["rig-b3"] = workerPayoutStat{PayoutAddress: addr}

	base, size, _, _, _, ok, reason := m.claim("rig-b3", 4096)
	if !ok {
		t.Fatalf("claim: %s", reason)
	}
	rec := m.active[workKey{base: base, batch: size}]
	if rec.FormulaVersion != 3 || rec.PrevHash != prev {
		t.Fatalf("lease bind: %+v", rec)
	}

	var nonce uint64
	hit := false
	for n := uint64(0); n < 200_000; n++ {
		if chain.PohWorkBlake3(prev, addr, n)%mod == 0 {
			nonce = n
			hit = true
			break
		}
	}
	if !hit {
		t.Fatal("no blake3 hit")
	}
	m.active[workKey{base: nonce, batch: 1}] = leaseRecord{
		WorkerID: "rig-b3", BaseNonce: nonce, BatchSize: 1, IssuedAt: 1, ExpiresAt: 1 << 40,
		TargetMod: mod, PrevHash: prev, MinerAddress: addr, FormulaVersion: 3, TipHeight: 500_000,
	}
	base, size = nonce, 1
	_ = base
	_ = size

	mk := func(foundNonce uint64, result string) submitWorkRequest {
		req := submitWorkRequest{
			WorkerID: "rig-b3", BaseNonce: base, BatchSize: size,
			Found: true, FoundNonce: foundNonce, ResultHash: result, HashrateGHS: 1,
			Attempts: 1, SubmitNonce: uint64(len(result)),
			MinerPubKey: hex.EncodeToString(pub), MinerAddress: addr, MinerSigAlg: "ed25519",
		}
		req.MinerSig = hex.EncodeToString(ed25519.Sign(priv, canonicalSubmitBytes(req)))
		return req
	}

	okSub, reason, payout, _, _ := m.submit(mk(nonce, "b3ok"))
	if !okSub || payout < m.foundBonus {
		t.Fatalf("valid blake3 found rejected: %s payout=%f", reason, payout)
	}

	// Algebraic shortcut must fail under v3 lease.
	var algNonce uint64
	for n := base; n < base+size; n++ {
		if chain.PohEval(n)%mod == 0 && n != nonce {
			algNonce = n
			break
		}
	}
	if algNonce == 0 {
		for n := uint64(0); n < 100000; n++ {
			if chain.PohEval(n)%mod == 0 {
				algNonce = n
				break
			}
		}
		m.active[workKey{base: algNonce, batch: 1}] = leaseRecord{
			WorkerID: "rig-b3", BaseNonce: algNonce, BatchSize: 1, IssuedAt: 1, ExpiresAt: 1 << 40,
			TargetMod: mod, PrevHash: prev, MinerAddress: addr, FormulaVersion: 3,
		}
		base, size = algNonce, 1
	}
	okSub, reason, _, _, _ = m.submit(mk(algNonce, "b3alg"))
	if okSub || reason != "found_nonce_invalid_for_target_mod" {
		t.Fatalf("algebraic must fail under v3: ok=%v reason=%s", okSub, reason)
	}

	// Wrong miner address on lease.
	m.active[workKey{base: nonce, batch: 1}] = leaseRecord{
		WorkerID: "rig-b3", BaseNonce: nonce, BatchSize: 1, IssuedAt: 1, ExpiresAt: 1 << 40,
		TargetMod: mod, PrevHash: prev, MinerAddress: "HMC-other", FormulaVersion: 3,
	}
	okSub, reason, _, _, _ = m.submit(submitWorkRequest{
		WorkerID: "rig-b3", BaseNonce: nonce, BatchSize: 1,
		Found: true, FoundNonce: nonce, ResultHash: "b3wrong", HashrateGHS: 1,
		Attempts: 1, SubmitNonce: 99,
		MinerPubKey: hex.EncodeToString(pub), MinerAddress: addr, MinerSigAlg: "ed25519",
		MinerSig: hex.EncodeToString(ed25519.Sign(priv, canonicalSubmitBytes(submitWorkRequest{
			WorkerID: "rig-b3", BaseNonce: nonce, BatchSize: 1, Found: true, FoundNonce: nonce,
			ResultHash: "b3wrong", Attempts: 1, SubmitNonce: 99,
		}))),
	})
	// Signature may fail first; either reject is fine for security — prefer invalid PoH if sig ok.
	if okSub {
		t.Fatal("wrong lease miner_address must not pay")
	}
	_ = reason
}

func TestResolvedPoHFormulaUsesTipHeight(t *testing.T) {
	m := report29WM(t, 2_000_000)
	m.pohFormulaVersion = 0
	m.tipHeight = 500_000
	m.pohActivationHeight = 500_000
	if got := m.resolvedPoHFormulaVersionLocked(); got != 3 {
		t.Fatalf("tip past activation must resolve v3, got %d", got)
	}
	m.tipHeight = 100
	if got := m.resolvedPoHFormulaVersionLocked(); got != 1 {
		t.Fatalf("pre-activation tip must resolve v1, got %d", got)
	}
}

func TestFoundRejectsWithoutTipHash(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HACKME_COORDINATOR_TARGET_MOD_FILE", filepath.Join(dir, "mod-notip.txt"))
	mod := uint64(1_000_000)
	pub, priv, _ := ed25519.GenerateKey(nil)
	addr := signerAddr(pub)
	m := report29WM(t, mod)
	m.tipHash = ""
	m.tipHeight = 0
	m.pohFormulaVersion = 0 // would historically fail-open to algebraic v1
	m.worker["rig-notip"] = workerPayoutStat{PayoutAddress: addr}
	nonce := uint64(142857) // classic 7n+13 hit for large M
	m.active[workKey{base: nonce, batch: 1}] = leaseRecord{
		WorkerID: "rig-notip", BaseNonce: nonce, BatchSize: 1, IssuedAt: 1, ExpiresAt: 1 << 40,
		TargetMod: mod, MinerAddress: addr, FormulaVersion: 1,
	}
	req := submitWorkRequest{
		WorkerID: "rig-notip", BaseNonce: nonce, BatchSize: 1, Found: true, FoundNonce: nonce,
		ResultHash: "deadbeef", Attempts: 1, SubmitNonce: 1, HashrateGHS: 1,
		MinerPubKey: hex.EncodeToString(pub), MinerAddress: addr, MinerSigAlg: "ed25519",
	}
	req.MinerSig = hex.EncodeToString(ed25519.Sign(priv, canonicalSubmitBytes(req)))
	ok, reason, _, _, _ := m.submit(req)
	if ok || reason != "tip_hash_unavailable" {
		t.Fatalf("found without tip must fail tip_hash_unavailable, got ok=%v reason=%s", ok, reason)
	}
}

func TestBlake3StalePrevHashRejected(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HACKME_COORDINATOR_TARGET_MOD_FILE", filepath.Join(dir, "mod2.txt"))
	prev := "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	mod := uint64(1021)
	pub, priv, _ := ed25519.GenerateKey(nil)
	addr := signerAddr(pub)
	m := report29WM(t, mod)
	m.targetModMin = 251
	m.targetMod = mod
	m.tipHash = prev
	m.tipHeight = 500_000
	m.pohFormulaVersion = 3
	m.worker["rig-stale"] = workerPayoutStat{PayoutAddress: addr}
	var nonce uint64
	for n := uint64(0); n < 200_000; n++ {
		if chain.PohWorkBlake3(prev, addr, n)%mod == 0 {
			nonce = n
			break
		}
	}
	m.active[workKey{base: nonce, batch: 1}] = leaseRecord{
		WorkerID: "rig-stale", BaseNonce: nonce, BatchSize: 1, IssuedAt: 1, ExpiresAt: 1 << 40,
		TargetMod: mod, PrevHash: prev, MinerAddress: addr, FormulaVersion: 3,
	}
	m.tipHash = "0000000000000000000000000000000000000000000000000000000000000001" // tip moved
	req := submitWorkRequest{
		WorkerID: "rig-stale", BaseNonce: nonce, BatchSize: 1,
		Found: true, FoundNonce: nonce, ResultHash: "stale1", HashrateGHS: 1,
		Attempts: 1, SubmitNonce: 1,
		MinerPubKey: hex.EncodeToString(pub), MinerAddress: addr, MinerSigAlg: "ed25519",
	}
	req.MinerSig = hex.EncodeToString(ed25519.Sign(priv, canonicalSubmitBytes(req)))
	ok, reason, _, _, _ := m.submit(req)
	if ok || reason != "stale_prev_hash" {
		t.Fatalf("want stale_prev_hash, got ok=%v reason=%s", ok, reason)
	}
}
