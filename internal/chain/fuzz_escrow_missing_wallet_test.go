package chain

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"hackme/internal/store"
)

func TestOpenFuzzEscrowMissingWalletNoSQLLeak(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "nowallet.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := New(db)
	if _, _, err := svc.InitGenesis(ctx, "HMC-aaaaaaaaaaaaaaaa"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM wallet WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	_, err = svc.OpenFuzzEscrow(ctx, "camp-nowallet", 1.0, 10)
	if !errors.Is(err, ErrFuzzWalletMissing) {
		t.Fatalf("want ErrFuzzWalletMissing, got %v", err)
	}
	if strings.Contains(err.Error(), "sql:") || strings.Contains(err.Error(), "no rows") {
		t.Fatalf("must not leak driver wording: %v", err)
	}
}
