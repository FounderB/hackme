package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hackme/internal/chain"
)

func TestOrdersFundRequiresAuth(t *testing.T) {
	a, _ := newWalletTestApp(t)
	t.Setenv("HACKME_ADMIN_TOKEN", "orders-fund-admin-token-32chars!!")
	t.Setenv("HACKME_CANONICAL_CHAIN_URL", "https://hackme.tech")
	t.Setenv("HACKME_DESKTOP_MODE", "1")
	body, _ := json.Marshal(map[string]any{"amount_hmc": 20})
	req := httptest.NewRequest(http.MethodPost, "/api/orders/fund", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	a.handleOrdersFundAPI(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOrdersFundRejectsBelowMin(t *testing.T) {
	a, _ := newWalletTestApp(t)
	tok := "orders-fund-admin-token-32chars!!"
	t.Setenv("HACKME_ADMIN_TOKEN", tok)
	t.Setenv("HACKME_CANONICAL_CHAIN_URL", "https://hackme.tech")
	t.Setenv("HACKME_DESKTOP_MODE", "1")
	body, _ := json.Marshal(map[string]any{"amount_hmc": 1})
	req := httptest.NewRequest(http.MethodPost, "/api/orders/fund", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hackme-Admin-Token", tok)
	rec := httptest.NewRecorder()
	a.handleOrdersFundAPI(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid_amount") && !strings.Contains(rec.Body.String(), "minimum") {
		t.Fatalf("body=%s", rec.Body.String())
	}
}

func TestOrdersFundConfirmMismatchAndCredit(t *testing.T) {
	a, _ := newWalletTestApp(t)
	from := a.signer.Address()
	tok := "orders-fund-admin-token-32chars!!"
	t.Setenv("HACKME_ADMIN_TOKEN", tok)
	t.Setenv("HACKME_DESKTOP_MODE", "1")

	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/tx/") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tx_hash":      "ffff1234ffff5678aaaa0000bbbb9999",
				"status":       "included",
				"from":         from,
				"to":           chain.DevFeeAddress,
				"amount_units": chain.HMCToUnits(20),
				"memo":         chain.OrdersFundMemo(from),
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer peer.Close()
	t.Setenv("HACKME_CANONICAL_CHAIN_URL", peer.URL)

	badBody, _ := json.Marshal(map[string]any{
		"tx_hash":    "ffff1234ffff5678aaaa0000bbbb9999",
		"amount_hmc": 25,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/orders/fund/confirm", bytes.NewReader(badBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hackme-Admin-Token", tok)
	rec := httptest.NewRecorder()
	a.handleOrdersFundAPI(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatalf("expected mismatch reject, body=%s", rec.Body.String())
	}

	okBody, _ := json.Marshal(map[string]any{
		"tx_hash":    "ffff1234ffff5678aaaa0000bbbb9999",
		"amount_hmc": 20,
	})
	req2 := httptest.NewRequest(http.MethodPost, "/api/orders/fund/confirm", bytes.NewReader(okBody))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Hackme-Admin-Token", tok)
	rec2 := httptest.NewRecorder()
	a.handleOrdersFundAPI(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("confirm status=%d body=%s", rec2.Code, rec2.Body.String())
	}
	_, bal, err := a.chain.Wallet(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if bal < 19.9 {
		t.Fatalf("spendable after confirm=%v", bal)
	}
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodPost, "/api/orders/fund/confirm", bytes.NewReader(okBody))
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("X-Hackme-Admin-Token", tok)
	a.handleOrdersFundAPI(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("replay status=%d body=%s", rec3.Code, rec3.Body.String())
	}
	_, bal2, _ := a.chain.Wallet(context.Background())
	if bal2 > bal+0.0001 {
		t.Fatalf("replay inflated %v -> %v", bal, bal2)
	}
}

func TestCanonicalTransferMemoMissingKey(t *testing.T) {
	if got := canonicalTransferMemo(map[string]any{}); got != "" {
		t.Fatalf("missing memo key: got %q", got)
	}
	if got := canonicalTransferMemo(map[string]any{"memo": nil}); got != "" {
		t.Fatalf("nil memo: got %q", got)
	}
	if got := canonicalTransferMemo(map[string]any{"memo": chain.OrdersFundMemo("HMC-abc")}); got == "" {
		t.Fatal("expected orders fund memo")
	}
}

func TestOrdersFundConfirmCanonicalTxGetOmitsMemo(t *testing.T) {
	a, _ := newWalletTestApp(t)
	from := a.signer.Address()
	tok := "orders-fund-admin-token-32chars!!"
	t.Setenv("HACKME_ADMIN_TOKEN", tok)
	t.Setenv("HACKME_DESKTOP_MODE", "1")

	txHash := "aabbccddaabbccddaabbccddaabbccdd112233445566778899aabbccddeeff"
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/tx/") {
			// Simulates production GET /api/tx/{hash} before memo round-trip fix.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tx_hash":      txHash,
				"status":       "included",
				"from":         from,
				"to":           chain.DevFeeAddress,
				"amount_units": chain.HMCToUnits(20),
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer peer.Close()
	t.Setenv("HACKME_CANONICAL_CHAIN_URL", peer.URL)

	body, _ := json.Marshal(map[string]any{"tx_hash": txHash, "amount_hmc": 20})
	req := httptest.NewRequest(http.MethodPost, "/api/orders/fund/confirm", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hackme-Admin-Token", tok)
	rec := httptest.NewRecorder()
	a.handleOrdersFundAPI(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm status=%d body=%s", rec.Code, rec.Body.String())
	}
}
