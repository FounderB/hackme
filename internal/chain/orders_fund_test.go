package chain

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"hackme/internal/nodecrypto"
	"hackme/internal/store"
)

func TestOrdersStarterDefaultIsZero(t *testing.T) {
	t.Setenv("HACKME_ORDERS_STARTER_HMC", "")
	if OrdersStarterHMCFromEnv() != 0 {
		t.Fatalf("default starter=%v want 0", OrdersStarterHMCFromEnv())
	}
	t.Setenv("HACKME_ORDERS_STARTER_HMC", "100")
	if OrdersStarterHMCFromEnv() != 100 {
		t.Fatalf("explicit starter=%v want 100", OrdersStarterHMCFromEnv())
	}
}

func TestCreditOrdersFundIdempotent(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "fund.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := New(db)
	ctx := context.Background()
	node := "HMC-ordersfundtest0001"
	if _, _, err := svc.InitGenesis(ctx, node); err != nil {
		t.Fatal(err)
	}
	units := HMCToUnits(20)
	r1, err := svc.CreditOrdersFund(ctx, node, "abcd1234abcd5678ffff0000eeee1111", units)
	if err != nil {
		t.Fatal(err)
	}
	if r1.IdempotentReplay || r1.AmountUnits != units {
		t.Fatalf("first: %+v", r1)
	}
	_, bal, err := svc.Wallet(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if bal < 19.9 || bal > 20.1 {
		t.Fatalf("balance after credit=%v", bal)
	}
	r2, err := svc.CreditOrdersFund(ctx, node, "abcd1234abcd5678ffff0000eeee1111", units)
	if err != nil {
		t.Fatal(err)
	}
	if !r2.IdempotentReplay {
		t.Fatal("expected idempotent replay")
	}
	_, bal2, _ := svc.Wallet(ctx)
	if bal2 > bal+0.0001 {
		t.Fatalf("double credit: %v -> %v", bal, bal2)
	}
}

func TestCreditOrdersFundRejectsWrongFromAndAmountMismatch(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "fund2.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := New(db)
	ctx := context.Background()
	node := "HMC-ordersfundtest0002"
	if _, _, err := svc.InitGenesis(ctx, node); err != nil {
		t.Fatal(err)
	}
	units := HMCToUnits(20)
	if _, err := svc.CreditOrdersFund(ctx, "HMC-notprimary000001", "bbbb1234bbbb5678ffff0000eeee2222", units); err == nil {
		t.Fatal("expected wrong from reject")
	}
	if _, err := svc.CreditOrdersFund(ctx, node, "cccc1234cccc5678ffff0000eeee3333", units); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreditOrdersFund(ctx, node, "cccc1234cccc5678ffff0000eeee3333", HMCToUnits(25)); err == nil {
		t.Fatal("expected amount mismatch reject")
	}
}

func TestCreditOrdersFundConcurrentSameHash(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "fund3.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := New(db)
	ctx := context.Background()
	node := "HMC-ordersfundtest0003"
	if _, _, err := svc.InitGenesis(ctx, node); err != nil {
		t.Fatal(err)
	}
	units := HMCToUnits(20)
	hash := "dddd1234dddd5678ffff0000eeee4444"
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.CreditOrdersFund(ctx, node, hash, units)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	_, bal, err := svc.Wallet(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if bal < 19.9 || bal > 20.1 {
		t.Fatalf("concurrent credit inflated balance=%v", bal)
	}
}

func TestCreditOrdersFundThenHuntEscrow(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "fund4.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := New(db)
	ctx := context.Background()
	node := "HMC-ordersfundtest0004"
	if _, _, err := svc.InitGenesis(ctx, node); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenHuntEscrow(ctx, "hunt-nofund", 20, 100); err == nil {
		t.Fatal("expected escrow fail without fund")
	}
	if _, err := svc.CreditOrdersFund(ctx, node, "eeee1234eeee5678ffff0000eeee5555", HMCToUnits(20)); err != nil {
		t.Fatal(err)
	}
	row, err := svc.OpenHuntEscrow(ctx, "hunt-after-fund", 20, 100)
	if err != nil {
		t.Fatal(err)
	}
	if row == nil || row.BudgetHMC < 19.9 {
		t.Fatalf("bad escrow row: %+v", row)
	}
}

func TestMemoIsOrdersFund(t *testing.T) {
	if !MemoIsOrdersFund(OrdersFundMemoPrefix) {
		t.Fatal("prefix")
	}
	if !MemoIsOrdersFund(OrdersFundMemo("HMC-abc")) {
		t.Fatal("with addr")
	}
	if MemoIsOrdersFund("other") {
		t.Fatal("other")
	}
}

func TestCreditOrdersFundCreatesMissingWalletRow(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "fund-miss.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	signer, err := nodecrypto.LoadOrCreate(dir)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewWithSigner(db, signer)
	ctx := context.Background()
	node := signer.Address()
	if _, _, err := svc.InitGenesis(ctx, node); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM wallet WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	units := HMCToUnits(20)
	r, err := svc.CreditOrdersFund(ctx, node, "dddd1234dddd5678ffff0000eeee4444", units)
	if err != nil {
		t.Fatal(err)
	}
	if r.AmountUnits != units {
		t.Fatalf("%+v", r)
	}
	_, bal, err := svc.Wallet(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if bal < 19.9 {
		t.Fatalf("bal=%v", bal)
	}
}
