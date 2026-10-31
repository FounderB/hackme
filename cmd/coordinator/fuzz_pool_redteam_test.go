package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"hackme/internal/poolfuzz"
	"hackme/internal/store"
)

func canonFuzzSign(p poolfuzz.SubmitSignPayload) []byte { return poolfuzz.CanonicalSubmitBytes(p) }

// Report #26 class fix: once worker_id is payout-locked, unsigned/mismatched
// fuzz submits must be refused (same gate as fuzz_pool.go submit handler).
func TestFuzzSubmitLockedWorkerRejectsUnsigned(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	lockedAddr := signerAddr(pub)
	wm := &workManager{
		hybridSignerEnabled: true,
		hybridSignerStrict:  false,
		worker: map[string]workerPayoutStat{
			"victim-w1": {PayoutAddress: lockedAddr},
		},
	}
	locked := wm.lockedPayoutAddress("victim-w1")
	if locked == "" {
		t.Fatal("expected durable lock")
	}
	reject := func(payoutAddr string) bool {
		return payoutAddr == "" || !strings.EqualFold(locked, payoutAddr)
	}
	if !reject("") {
		t.Fatal("unsigned must reject when locked")
	}
	pub2, _, _ := ed25519.GenerateKey(nil)
	if !reject(signerAddr(pub2)) {
		t.Fatal("wrong address must reject")
	}
	if reject(lockedAddr) {
		t.Fatal("matching locked address must be allowed")
	}
}

func TestFuzzSubmitTamperedMinerRejected(t *testing.T) {
	wm := &workManager{
		hybridSignerEnabled:  true,
		acceptedSubmitNonces: make(map[string]struct{}),
		signedSubmitNonceMax: make(map[string]uint64),
	}
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	payload := poolfuzz.SubmitSignPayload{
		WorkerID: "w-rt", CampaignID: "c1", ItemID: 1, InputN: 1, ActualInput: 2, CheckResult: 0, SubmitNonce: 1,
	}
	body := canonFuzzSign(payload)
	sig := ed25519.Sign(priv, body)
	reqBody, _ := json.Marshal(map[string]any{
		"worker_id": "w-rt", "campaign_id": "c1", "item_id": 1, "input_n": 1, "actual_input": 2,
		"check_result": 0, "submit_nonce": 1,
		"miner_pubkey":  hex.EncodeToString(pub),
		"miner_sig":     hex.EncodeToString(sig),
		"miner_address": signerAddr(pub),
	})
	var decoded map[string]any
	_ = json.Unmarshal(reqBody, &decoded)
	decoded["miner_address"] = "HMC-ffffffffffffffff"
	reqBody, _ = json.Marshal(decoded)

	signBody := canonFuzzSign(payload)
	ok, reason, _ := wm.validateFuzzHybridSignature(fuzzSubmitAuth{
		WorkerID: "w-rt", MinerAddress: "HMC-ffffffffffffffff",
		MinerPubKey: hex.EncodeToString(pub), MinerSig: hex.EncodeToString(sig), SubmitNonce: 1,
	}, signBody)
	if ok || reason != "pubkey_address_mismatch" {
		t.Fatalf("tamper: ok=%v reason=%q", ok, reason)
	}
}

func TestFuzzSubmitReplayNonceRejected(t *testing.T) {
	wm := &workManager{
		hybridSignerEnabled:  true,
		acceptedSubmitNonces: make(map[string]struct{}),
		signedSubmitNonceMax: make(map[string]uint64),
	}
	pub, priv, _ := ed25519.GenerateKey(nil)
	payload := poolfuzz.SubmitSignPayload{WorkerID: "w", CampaignID: "c", ItemID: 1, SubmitNonce: 7}
	body := canonFuzzSign(payload)
	sig := hex.EncodeToString(ed25519.Sign(priv, body))
	auth := fuzzSubmitAuth{
		WorkerID: "w", MinerPubKey: hex.EncodeToString(pub), MinerSig: sig, SubmitNonce: 7,
	}
	if ok, _, addr := wm.validateFuzzHybridSignature(auth, body); !ok {
		t.Fatal("first sig should pass")
	} else {
		wm.commitFuzzHybridNonce(addr, 7)
	}
	if ok, reason, _ := wm.validateFuzzHybridSignature(auth, body); ok || reason != "replay" {
		t.Fatalf("replay want reject after commit, ok=%v reason=%q", ok, reason)
	}
}

func TestFuzzSubmitConcurrentNonceMapsNoPanic(t *testing.T) {
	wm := &workManager{
		hybridSignerEnabled:  true,
		acceptedSubmitNonces: make(map[string]struct{}),
		signedSubmitNonceMax: make(map[string]uint64),
	}
	const n = 64
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			pub, priv, err := ed25519.GenerateKey(nil)
			if err != nil {
				t.Errorf("keygen: %v", err)
				return
			}
			nonce := uint64(1)
			payload := poolfuzz.SubmitSignPayload{WorkerID: "w-conc", CampaignID: "c", ItemID: int64(i + 1), SubmitNonce: nonce}
			body := canonFuzzSign(payload)
			sig := hex.EncodeToString(ed25519.Sign(priv, body))
			auth := fuzzSubmitAuth{
				WorkerID: "w-conc", MinerPubKey: hex.EncodeToString(pub), MinerSig: sig, SubmitNonce: nonce,
			}
			ok, reason, addr := wm.validateFuzzHybridSignature(auth, body)
			if !ok {
				t.Errorf("worker %d: want accept, reason=%q", i, reason)
				return
			}
			wm.commitFuzzHybridNonce(addr, nonce)
		}(i)
	}
	wg.Wait()
	if len(wm.acceptedSubmitNonces) != n {
		t.Fatalf("accepted nonces=%d want %d", len(wm.acceptedSubmitNonces), n)
	}
}

func TestHTTPFuzzSubmitUnauth(t *testing.T) {
	pf := &poolfuzz.Service{DB: nil}
	mux := http.NewServeMux()
	addFuzzPoolRoutes(mux, "admin-tok", "worker-tok", "", false, &workManager{}, pf)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/fuzz/work/submit", bytes.NewReader([]byte(`{"worker_id":"x"}`)))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestChargeClaimWorkerRateLimits(t *testing.T) {
	wm := &workManager{
		claimPerMin:     1,
		abuse:           make(map[string]workerAbuseState),
		ipAbuse:         make(map[string]workerAbuseState),
		worker:          make(map[string]workerPayoutStat),
		dropReasonCount: make(map[string]uint64),
	}
	now := time.Now().Unix()
	if ok, _ := wm.chargeClaimWorker("w-rate", now); !ok {
		t.Fatal("first chargeClaimWorker should pass")
	}
	if ok, reason := wm.chargeClaimWorker("w-rate", now); ok || reason != "claim_rate_limited" {
		t.Fatalf("want claim_rate_limited, got ok=%v reason=%q", ok, reason)
	}
}

func TestHTTPFuzzClaimPeerGateDoesNotPreChargeWorker(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "claim-peer.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	pf := &poolfuzz.Service{DB: db}
	wm := &workManager{
		claimPerMin:     1,
		abuse:           make(map[string]workerAbuseState),
		ipAbuse:         make(map[string]workerAbuseState),
		worker:          make(map[string]workerPayoutStat),
		dropReasonCount: make(map[string]uint64),
		maxWorkers:      100,
	}
	mux := http.NewServeMux()
	addFuzzPoolRoutes(mux, "admin-tok", "worker-tok", "", false, wm, pf)
	// Empty queue → no_fuzz_work; forgeable id must not consume claim bucket.
	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/fuzz/work/claim", bytes.NewReader([]byte(`{"worker_id":"w-rate"}`)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Hackme-Admin-Token", "worker-tok")
		mux.ServeHTTP(rec, req)
		if rec.Code == http.StatusOK {
			t.Fatalf("expected empty queue, got 200")
		}
	}
	wm.mu.Lock()
	st := wm.abuse["w-rate"]
	wm.mu.Unlock()
	if st.ClaimCount != 0 {
		t.Fatalf("failed claims must not charge worker claim slot, ClaimCount=%d", st.ClaimCount)
	}
}

func TestTouchWorkerSeenKeepsFuzzWorkerOnline(t *testing.T) {
	wm := &workManager{worker: make(map[string]workerPayoutStat)}
	wm.worker["fuzz-w1"] = workerPayoutStat{LastSeenUnix: 1}
	wm.touchWorkerSeen("fuzz-w1")
	wm.mu.Lock()
	st := wm.worker["fuzz-w1"]
	wm.mu.Unlock()
	if st.LastSeenUnix <= 1 {
		t.Fatalf("touchWorkerSeen must refresh last_seen_unix, got %d", st.LastSeenUnix)
	}
	now := time.Now().Unix()
	if st.LastSeenUnix > now+1 || now-st.LastSeenUnix > 2 {
		t.Fatalf("last_seen out of range: %d vs now %d", st.LastSeenUnix, now)
	}
}

func TestHTTPFuzzClaimTouchesLastSeenOnEmptyQueue(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "fuzz-claim.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	pf := &poolfuzz.Service{DB: db}
	wm := &workManager{
		claimPerMin:     60,
		abuse:           make(map[string]workerAbuseState),
		ipAbuse:         make(map[string]workerAbuseState),
		worker:          make(map[string]workerPayoutStat),
		dropReasonCount: make(map[string]uint64),
	}
	mux := http.NewServeMux()
	addFuzzPoolRoutes(mux, "admin-tok", "worker-tok", "", false, wm, pf)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/fuzz/work/claim", bytes.NewReader([]byte(`{"worker_id":"fuzz-online"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hackme-Admin-Token", "worker-tok")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("empty queue want 429 no_fuzz_work, got %d body=%s", rec.Code, rec.Body.String())
	}
	wm.mu.Lock()
	st := wm.worker["fuzz-online"]
	wm.mu.Unlock()
	if st.LastSeenUnix <= 0 {
		t.Fatal("claim with empty queue must still refresh last_seen (fuzz online heartbeat)")
	}
}
