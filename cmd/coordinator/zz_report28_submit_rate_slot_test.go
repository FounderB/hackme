package main

// Report #28: submit-lane rate slots must not charge the declared worker_id
// before signature / payout-lock verification. Forged unsigned floods under a
// victim id must not freeze that miner's earning lane.

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hackme/internal/lanpool"
	"hackme/internal/poolfuzz"
	"hackme/internal/store"
)

func report28Setup(t *testing.T, submitPerMin int) (*workManager, *http.ServeMux) {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "r28.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	pf := &poolfuzz.Service{DB: db}
	wm := &workManager{
		hybridSignerEnabled:  true,
		hybridSignerStrict:   true,
		claimRequirePubKey:   true,
		claimPerMin:          60,
		submitPerMin:         submitPerMin,
		acceptedSubmitNonces: make(map[string]struct{}),
		signedSubmitNonceMax: make(map[string]uint64),
		abuse:                make(map[string]workerAbuseState),
		ipAbuse:              make(map[string]workerAbuseState),
		worker:               map[string]workerPayoutStat{},
		dropReasonCount:      make(map[string]uint64),
	}
	wm.attachDedupDB(db)
	mux := http.NewServeMux()
	addFuzzPoolRoutes(mux, "admin-tok", "worker-tok", false, wm, pf)
	addWorkRoutes(mux, "admin-tok", "worker-tok", false, lanpool.NewRegistry(), wm, nil)
	return wm, mux
}

func report28Post(t *testing.T, mux *http.ServeMux, path string, body map[string]any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hackme-Admin-Token", "worker-tok")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestReport28FuzzSubmitUnsignedFloodDoesNotFreezeVictim(t *testing.T) {
	wm, mux := report28Setup(t, 6)
	vicPub, _, _ := ed25519.GenerateKey(nil)
	wm.notePayoutLock("victim-rig-01", signerAddr(vicPub))

	body := map[string]any{
		"worker_id": "victim-rig-01", "campaign_id": "r28-camp", "item_id": 1,
	}

	code, out := report28Post(t, mux, "/api/fuzz/work/submit", body)
	if code == http.StatusTooManyRequests {
		t.Fatalf("control must not be rate limited: %d %v", code, out)
	}
	if code != http.StatusForbidden || out["reason"] != "signature_required" {
		t.Fatalf("control want 403 signature_required, got %d %v", code, out)
	}

	// Flood well past the victim bucket ceiling — must not charge declared id.
	for i := 0; i < 20; i++ {
		code, out = report28Post(t, mux, "/api/fuzz/work/submit", body)
		if code == http.StatusTooManyRequests && out["reason"] == "submit_rate_limited" {
			t.Fatalf("unsigned flood must not freeze victim bucket at req %d: %v", i, out)
		}
	}

	code, out = report28Post(t, mux, "/api/fuzz/work/submit", body)
	if code == http.StatusTooManyRequests {
		t.Fatalf("victim must still reach signature gate: %d %v", code, out)
	}
	if out["reason"] != "signature_required" {
		t.Fatalf("want signature_required after flood, got %d %v", code, out)
	}

	wm.mu.Lock()
	st := wm.abuse["victim-rig-01"]
	wm.mu.Unlock()
	if st.SubmitCount != 0 {
		t.Fatalf("unsigned flood must not charge victim submit slot: %+v", st)
	}
}

func TestReport28PoHSubmitUnsignedFloodDoesNotFreezeVictim(t *testing.T) {
	wm, mux := report28Setup(t, 6)
	body := map[string]any{"worker_id": "victim-rig-02", "batch_size": 1}

	code, out := report28Post(t, mux, "/api/work/submit", body)
	if code == http.StatusTooManyRequests {
		t.Fatalf("control must not be rate limited: %d %v", code, out)
	}
	for i := 0; i < 20; i++ {
		code, out = report28Post(t, mux, "/api/work/submit", body)
		if code == http.StatusTooManyRequests && out["reason"] == "submit_rate_limited" {
			t.Fatalf("PoH unsigned flood must not freeze victim at req %d: %v", i, out)
		}
	}
	code, out = report28Post(t, mux, "/api/work/submit", body)
	if code == http.StatusTooManyRequests {
		t.Fatalf("victim PoH submit must not be frozen: %d %v", code, out)
	}
	wm.mu.Lock()
	st := wm.abuse["victim-rig-02"]
	wm.mu.Unlock()
	if st.SubmitCount != 0 {
		t.Fatalf("PoH flood must not charge victim submit slot: %+v", st)
	}
}

func TestReport28ChargeWorkerOnlyAfterIdentity(t *testing.T) {
	wm, _ := report28Setup(t, 600)
	wm.mu.Lock()
	wm.worker["victim-gpu-01"] = workerPayoutStat{LastHashrateGHS: 4.0}
	wm.mu.Unlock()
	now := time.Now().Unix()
	lim := wm.workerRateLimitPerMin("victim-gpu-01", 600)
	if lim != 120 {
		t.Fatalf("live GPU rig bucket = %d/min, want 120", lim)
	}
	// Peer gate alone must not fill the worker bucket.
	for i := 0; i < lim+5; i++ {
		if ok, reason := wm.allowSubmitPeer("victim-gpu-01", "1.2.3.4", now); !ok {
			t.Fatalf("peer gate failed early at %d: %v %q", i, ok, reason)
		}
	}
	if ok, reason := wm.chargeSubmitWorker("victim-gpu-01", now); !ok || reason != "" {
		t.Fatalf("first post-identity charge must pass: %v %q", ok, reason)
	}
	wm.mu.Lock()
	got := wm.abuse["victim-gpu-01"].SubmitCount
	wm.mu.Unlock()
	if got != 1 {
		t.Fatalf("worker submit count=%d want 1 after single charge", got)
	}
}

func TestReport28ClaimLaneStillIdentityBeforeRate(t *testing.T) {
	wm, mux := report28Setup(t, 6)
	vicPub, _, _ := ed25519.GenerateKey(nil)
	atkPub, _, _ := ed25519.GenerateKey(nil)
	wm.notePayoutLock("victim-rig-03", signerAddr(vicPub))

	for i := 0; i < 30; i++ {
		code, out := report28Post(t, mux, "/api/fuzz/work/claim", map[string]any{
			"worker_id":    "victim-rig-03",
			"miner_pubkey": hex.EncodeToString(atkPub), "miner_address": signerAddr(atkPub),
			"hunt_engines": "libfuzzer,aflpp,byte",
		})
		reason, _ := out["reason"].(string)
		if code != http.StatusForbidden || !strings.HasPrefix(reason, "payout_address_locked") {
			t.Fatalf("forged claim %d: %d %v", i, code, out)
		}
	}
	code, out := report28Post(t, mux, "/api/fuzz/work/claim", map[string]any{
		"worker_id":    "victim-rig-03",
		"miner_pubkey": hex.EncodeToString(vicPub), "miner_address": signerAddr(vicPub),
		"hunt_engines": "libfuzzer,aflpp,byte",
	})
	if out["reason"] == "claim_rate_limited" {
		t.Fatalf("claim lane must not charge before identity: %d %v", code, out)
	}
}
