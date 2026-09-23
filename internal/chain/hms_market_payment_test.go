package chain

import (
	"context"
	"path/filepath"
	"testing"

	"hackme/internal/hms"
	"hackme/internal/store"
)

func TestPayHMSStorageMarketDebitsWallet(t *testing.T) {
	t.Setenv("HMS_MARKET_PAYMENT_HMAC_SECRET", "test-hmac-secret")
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "chain.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := New(db)
	if _, _, err := svc.InitGenesis(ctx, DevFeeAddress); err != nil {
		t.Fatal(err)
	}
	q, err := hms.QuoteStorageOrder(1<<30, 30)
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.PayHMSStorageMarket(ctx, "test-backup", 1<<30, 30, q.QuoteHash, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.PaymentID == "" || res.PaymentProof == "" || res.TotalDebitHMC <= 0 {
		t.Fatalf("bad payment: %+v", res)
	}
	if res.QuoteHash != q.QuoteHash {
		t.Fatal("quote hash not echoed")
	}
}

func TestPayHMSStorageMarketIdempotent(t *testing.T) {
	t.Setenv("HMS_MARKET_PAYMENT_HMAC_SECRET", "test-hmac-secret")
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "chain.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := New(db)
	if _, _, err := svc.InitGenesis(ctx, DevFeeAddress); err != nil {
		t.Fatal(err)
	}
	q, err := hms.QuoteStorageOrder(1<<30, 30)
	if err != nil {
		t.Fatal(err)
	}
	first, err := svc.PayHMSStorageMarket(ctx, "idem", 1<<30, 30, q.QuoteHash, "key-1")
	if err != nil {
		t.Fatal(err)
	}
	bal1 := first.BalanceAfter
	second, err := svc.PayHMSStorageMarket(ctx, "idem", 1<<30, 30, q.QuoteHash, "key-1")
	if err != nil {
		t.Fatal(err)
	}
	if !second.IdempotentReplay {
		t.Fatal("expected idempotent_replay")
	}
	if second.PaymentID != first.PaymentID {
		t.Fatalf("payment_id mismatch %q vs %q", first.PaymentID, second.PaymentID)
	}
	if second.BalanceAfter != bal1 {
		t.Fatalf("second call debited again: %v -> %v", bal1, second.BalanceAfter)
	}
}

func TestPayHMSStorageMarketRefusesDebitWithoutProofSecret(t *testing.T) {
	t.Setenv("HMS_MARKET_PAYMENT_HMAC_SECRET", "")
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "chain.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := New(db)
	if _, _, err := svc.InitGenesis(ctx, DevFeeAddress); err != nil {
		t.Fatal(err)
	}
	var before uint64
	if err := db.QueryRowContext(ctx, `SELECT balance_units FROM wallet WHERE id=1`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	q, err := hms.QuoteStorageOrder(1<<30, 30)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PayHMSStorageMarket(ctx, "no-proof", 1<<30, 30, q.QuoteHash, "key-missing"); err == nil {
		t.Fatal("expected missing HMAC secret to reject the debit")
	}
	var after uint64
	if err := db.QueryRowContext(ctx, `SELECT balance_units FROM wallet WHERE id=1`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("balance changed without a proof: %d -> %d", before, after)
	}
}
