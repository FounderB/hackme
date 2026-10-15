package chain

import (
	"context"
	"path/filepath"
	"testing"

	"hackme/internal/store"
)

func TestBootstrapOrdersSpendableCreditsPrimaryWallet(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := New(db)
	ctx := context.Background()
	node := "HMC-ordersbootstraptst01"
	got, err := svc.BootstrapOrdersSpendable(ctx, node, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got < 99.9 {
		t.Fatalf("credited=%v want ~100", got)
	}
	addr, bal, err := svc.Wallet(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if addr != node {
		t.Fatalf("wallet addr=%q want %q", addr, node)
	}
	if bal < 99.9 {
		t.Fatalf("balance=%v want >=100", bal)
	}
	// Idempotent
	got2, err := svc.BootstrapOrdersSpendable(ctx, node, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got2 != 0 {
		t.Fatalf("second call credited=%v want 0", got2)
	}
	_, bal2, _ := svc.Wallet(ctx)
	if bal2 < 99.9 || bal2 > bal+0.001 {
		t.Fatalf("balance changed on second bootstrap: %v -> %v", bal, bal2)
	}
}
