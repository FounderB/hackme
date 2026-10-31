package main

// Pins the M14 payout-lock reject behavior on /api/fuzz/work/submit:
//  1. a foreign-key submit declaring a locked worker_id must NOT release the
//     victim's lease (it previously sniped any lease via the forged worker_id),
//  2. the 403 body must not echo the locked (victim) payout address,
//  3. neither the declared worker id nor the submit IP gets an abuse strike.

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"hackme/internal/hunt"
	"hackme/internal/poolfuzz"
	"hackme/internal/store"
)

func TestFuzzSubmitM14RejectKeepsVictimLeaseAndHidesLock(t *testing.T) {
	t.Setenv("HACKME_POOL_HUNT_REPLAY", "1")
	t.Setenv("HACKME_POOL_HUNT_REPLAY_ASYNC", "1")

	ctx := context.Background()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "m14.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	pf := &poolfuzz.Service{DB: db}

	cfg := map[string]any{
		"pool_distributed":     true,
		"work_kind":            "hunt_shard",
		"campaign_type":        "hunt",
		"upstream_target_id":   "jsmn",
		"harness_hash":         "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		"iterations_per_shard": 2,
		"max_input_bytes":      256,
		"depth_tier":           "oss_cve",
		"check_semantics":      "native_crash",
	}
	if err := pf.RegisterCampaign(ctx, poolfuzz.Campaign{ID: "m14-camp", CampaignType: "hunt", Status: "running", BudgetRuns: 1, Config: cfg}); err != nil {
		t.Fatal(err)
	}
	hunt.SetHarnessObjectDir("")
	t.Cleanup(func() { hunt.SetHarnessObjectDir("") })
	if err := hunt.PutHarnessArtifact(ctx, db, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef", []byte("m14-harness-blob"), "jsmn"); err != nil {
		t.Fatal(err)
	}
	_ = pf.EnsureWorkItems(ctx, "m14-camp", time.Now().Unix())
	w, ok, err := pf.Claim(ctx, "victim-w1", time.Now().Unix())
	if err != nil || !ok {
		t.Fatalf("claim ok=%v err=%v", ok, err)
	}

	// The victim's claim-time payout lock, as checkClaimMinerIdentity binds it.
	victimPub, _, _ := ed25519.GenerateKey(nil)
	wm := &workManager{
		hybridSignerEnabled:  true,
		claimPerMin:          60,
		submitPerMin:         60,
		acceptedSubmitNonces: make(map[string]struct{}),
		signedSubmitNonceMax: make(map[string]uint64),
		abuse:                make(map[string]workerAbuseState),
		ipAbuse:              make(map[string]workerAbuseState),
		worker:               map[string]workerPayoutStat{"victim-w1": {PayoutAddress: signerAddr(victimPub)}},
		dropReasonCount:      make(map[string]uint64),
	}
	// Bind the claim-time payout lock exactly as the coordinator claim lane does
	// (checkClaimMinerIdentity -> notePayoutLock): durable row via the same store
	// db, on top of the in-memory worker entry seeded in the literal above.
	wm.attachDedupDB(db)
	wm.notePayoutLock("victim-w1", signerAddr(victimPub))

	mux := http.NewServeMux()
	addFuzzPoolRoutes(mux, "admin-tok", "worker-tok", "", false, wm, pf)

	// Attacker holds a valid pool token and signs with their own key while
	// declaring the victim's worker_id — the forged-worker_id submit position.
	atkPub, atkPriv, _ := ed25519.GenerateKey(nil)
	payload := poolfuzz.SubmitSignPayload{
		WorkerID: "victim-w1", CampaignID: w.CampaignID, ItemID: w.ItemID,
		InputN: w.InputN, ActualInput: w.ActualInput, CheckResult: 0, SubmitNonce: 1,
		SegmentExecDone: 2,
	}
	sig := ed25519.Sign(atkPriv, poolfuzz.CanonicalSubmitBytes(payload))
	reqBody, _ := json.Marshal(map[string]any{
		"worker_id": "victim-w1", "campaign_id": w.CampaignID, "item_id": w.ItemID,
		"input_n": w.InputN, "actual_input": w.ActualInput, "check_result": 0,
		"submit_nonce": 1, "segment_exec_done": 2,
		"miner_pubkey":  hex.EncodeToString(atkPub),
		"miner_sig":     hex.EncodeToString(sig),
		"miner_address": signerAddr(atkPub),
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/fuzz/work/submit", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hackme-Admin-Token", "worker-tok")
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 payout_address_locked, got %d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if reason, _ := out["reason"].(string); reason != "payout_address_locked" {
		t.Fatalf("reason must be the short form, body=%s", rec.Body.String())
	}
	if _, has := out["locked_payout_address"]; has {
		t.Fatal("403 body must not echo the locked (victim) payout address")
	}
	if _, has := out["submitted_payout_address"]; has {
		t.Fatal("403 body must not echo payout addresses")
	}

	// The victim's lease must survive the forged submit.
	var st, owner string
	if err := db.QueryRowContext(ctx,
		`SELECT status, COALESCE(lease_owner,'') FROM fuzz_work_items WHERE campaign_id=? AND id=?`,
		w.CampaignID, w.ItemID).Scan(&st, &owner); err != nil {
		t.Fatal(err)
	}
	if st != "leased" || owner != "victim-w1" {
		t.Fatalf("forged submit must not snipe the victim lease: status=%q lease_owner=%q", st, owner)
	}

	// No abuse strike for the declared (victim) id or the submit IP.
	wm.mu.Lock()
	victim := wm.abuse["victim-w1"]
	ipStrikes := 0
	for _, s := range wm.ipAbuse {
		ipStrikes += int(s.BadStrikes)
	}
	wm.mu.Unlock()
	if victim.BadStrikes != 0 || victim.BannedUntil != 0 || ipStrikes != 0 {
		t.Fatalf("M14 reject must charge nobody: victim={strikes:%d banned:%d} ipStrikes=%d",
			victim.BadStrikes, victim.BannedUntil, ipStrikes)
	}
}

func TestMarkSubmitOutcomePayoutAddressLockedChargesNobody(t *testing.T) {
	wm := &workManager{
		abuse:   make(map[string]workerAbuseState),
		ipAbuse: make(map[string]workerAbuseState),
	}
	now := time.Now().Unix()
	for i := 0; i < 30; i++ {
		wm.markSubmitOutcome("victim-w1", "203.0.113.9", "payout_address_locked", now)
	}
	wm.mu.Lock()
	v := wm.abuse["victim-w1"]
	ip := wm.ipAbuse["203.0.113.9"]
	wm.mu.Unlock()
	if v.BadStrikes != 0 || v.BannedUntil != 0 || ip.BadStrikes != 0 || ip.BannedUntil != 0 {
		t.Fatalf("payout_address_locked must strike nobody: worker=%+v ip=%+v", v, ip)
	}
}
