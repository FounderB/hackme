package chain

import (
	"context"
	"path/filepath"
	"testing"

	"hackme/internal/store"
)

func TestReopenFuzzEscrowForSettleDrain(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "reopen.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := New(db)
	payer := "HMC-1111111111111111"
	miner := "HMC-2222222222222222"
	if _, _, err := svc.InitGenesis(ctx, payer); err != nil {
		t.Fatal(err)
	}
	preFundEscrow(t, ctx, db, payer, 50)
	const camp = "reopen-camp"
	if _, err := svc.OpenFuzzEscrow(ctx, camp, 10.0, 100); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO accounts(address, balance_units, next_nonce, updated_at) VALUES(?,0,0,0)`, miner); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.FinalizeFuzzEscrow(ctx, camp); err != nil {
		t.Fatal(err)
	}
	row, err := svc.GetFuzzEscrow(ctx, camp)
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != "closed" {
		t.Fatalf("status=%q", row.Status)
	}
	re, err := svc.ReopenFuzzEscrowForSettleDrain(ctx, camp)
	if err != nil {
		t.Fatal(err)
	}
	if re.Status != "open" {
		t.Fatalf("reopen status=%q", re.Status)
	}
	if _, err := svc.PayFuzzRun(ctx, camp, miner); err != nil {
		t.Fatal(err)
	}
	paid, err := svc.GetFuzzEscrow(ctx, camp)
	if err != nil {
		t.Fatal(err)
	}
	if paid.RunsPaidHMC <= 0 {
		t.Fatal("expected run paid after reopen")
	}
}
