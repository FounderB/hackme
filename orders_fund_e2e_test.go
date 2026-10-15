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

// TestOrdersFundE2E_NoStarterHuntFailsThenConfirmFundsEscrow exercises the honest path:
// starter default 0 → escrow fails → confirm credited fund → Hunt escrow opens.
func TestOrdersFundE2E_NoStarterHuntFailsThenConfirmFundsEscrow(t *testing.T) {
	t.Setenv("HACKME_ORDERS_STARTER_HMC", "")
	if chain.OrdersStarterHMCFromEnv() != 0 {
		t.Fatalf("starter default want 0 got %v", chain.OrdersStarterHMCFromEnv())
	}

	a, _ := newWalletTestApp(t)
	from := a.signer.Address()
	tok := "orders-fund-e2e-admin-token-32ok!"
	t.Setenv("HACKME_ADMIN_TOKEN", tok)
	t.Setenv("HACKME_DESKTOP_MODE", "1")
	t.Setenv("DESKTOP_PROFILE", "orders")

	ctx := context.Background()
	if _, err := a.chain.OpenHuntEscrow(ctx, "hunt-before-fund", 20, 100); err == nil {
		t.Fatal("expected hunt escrow to fail with zero spendable")
	}

	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/tx/") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tx_hash":      "e2e01234e2e05678aaaa0000bbbb7777",
				"status":       "included",
				"from":         from,
				"to":           chain.DevFeeAddress,
				"amount_units": chain.HMCToUnits(20),
				"memo":         chain.OrdersFundMemo(from),
			})
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/address/") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"address":       from,
				"balance_units": chain.HMCToUnits(100),
				"next_nonce":    1,
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer peer.Close()
	t.Setenv("HACKME_CANONICAL_CHAIN_URL", peer.URL)

	body, _ := json.Marshal(map[string]any{
		"tx_hash":    "e2e01234e2e05678aaaa0000bbbb7777",
		"amount_hmc": 20,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/orders/fund/confirm", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hackme-Admin-Token", tok)
	rec := httptest.NewRecorder()
	a.handleOrdersFundAPI(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm HTTP %d body=%s", rec.Code, rec.Body.String())
	}

	_, bal, err := a.chain.Wallet(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if bal < 19.9 {
		t.Fatalf("spendable=%v", bal)
	}
	row, err := a.chain.OpenHuntEscrow(ctx, "hunt-after-fund-e2e", 20, 100)
	if err != nil {
		t.Fatal(err)
	}
	if row.BudgetHMC < 19.9 {
		t.Fatalf("escrow=%+v", row)
	}
}
