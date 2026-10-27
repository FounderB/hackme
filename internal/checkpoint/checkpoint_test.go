package checkpoint

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"hackme/internal/chain"
	"hackme/internal/nodecrypto"
	"hackme/internal/store"
)

func testChain(t *testing.T) (*sql.DB, *chain.Service, string) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "cp.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := chain.New(db)
	addr := "HMC-1111111111111111"
	if _, _, err := svc.InitGenesis(ctx, addr); err != nil {
		t.Fatal(err)
	}
	return db, svc, addr
}

func testSigner(t *testing.T) *nodecrypto.Signer {
	t.Helper()
	s, err := nodecrypto.LoadOrCreate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func mustSnap(t *testing.T, db *sql.DB) Snapshot {
	t.Helper()
	s, err := ReadSnapshot(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestStateHashCommitsToEveryLedgerField(t *testing.T) {
	db, _, _ := testChain(t)
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO accounts (address, balance_units, next_nonce) VALUES ('HMC-aaaaaaaaaaaaaaaa', 500, 3)
	      ON CONFLICT(address) DO UPDATE SET balance_units=500, next_nonce=3`)
	base := mustSnap(t, db)
	if again := mustSnap(t, db); again.StateHash != base.StateHash {
		t.Fatal("state hash must be deterministic")
	}
	if base.Height != 0 || base.BlockHash == "" || base.Accounts == 0 {
		t.Fatalf("unexpected snapshot %+v", base)
	}

	mutations := []struct {
		name, do, undo string
	}{
		{"balance", `UPDATE accounts SET balance_units=501 WHERE address='HMC-aaaaaaaaaaaaaaaa'`, `UPDATE accounts SET balance_units=500 WHERE address='HMC-aaaaaaaaaaaaaaaa'`},
		{"nonce", `UPDATE accounts SET next_nonce=4 WHERE address='HMC-aaaaaaaaaaaaaaaa'`, `UPDATE accounts SET next_nonce=3 WHERE address='HMC-aaaaaaaaaaaaaaaa'`},
		{"sup", `UPDATE accounts SET balance_sup_units=1 WHERE address='HMC-aaaaaaaaaaaaaaaa'`, `UPDATE accounts SET balance_sup_units=0 WHERE address='HMC-aaaaaaaaaaaaaaaa'`},
		{"hms nonce", `UPDATE accounts SET hms_next_nonce=1 WHERE address='HMC-aaaaaaaaaaaaaaaa'`, `UPDATE accounts SET hms_next_nonce=0 WHERE address='HMC-aaaaaaaaaaaaaaaa'`},
		{"minted", `INSERT INTO meta(key,value) VALUES('econ_total_minted_units','999') ON CONFLICT(key) DO UPDATE SET value='999'`, ``},
	}
	prev := base.StateHash
	for _, m := range mutations {
		exec(m.do)
		got := mustSnap(t, db).StateHash
		if got == prev {
			t.Fatalf("%s change not committed to state_hash", m.name)
		}
		if m.undo != "" {
			exec(m.undo)
			if back := mustSnap(t, db).StateHash; back != prev {
				t.Fatalf("%s undo must restore the state hash", m.name)
			}
		} else {
			prev = got
		}
	}

	var escrowTable int
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE name='fuzz_campaign_escrow'`).Scan(&escrowTable)
	if escrowTable == 1 {
		before := mustSnap(t, db)
		exec(`INSERT INTO fuzz_campaign_escrow (campaign_id, budget_units, runs_pool_units, bounty_pool_units, budget_runs, per_run_units, created_at)
		      VALUES ('camp-1', 100, 20, 80, 10, 2, 1)`)
		after := mustSnap(t, db)
		if after.StateHash == before.StateHash || after.Escrows != before.Escrows+1 {
			t.Fatal("escrow rows must be committed to state_hash")
		}
	}
}

func TestSignVerifyAndTamperDetection(t *testing.T) {
	db, _, _ := testChain(t)
	s := testSigner(t)
	c, err := New(mustSnap(t, db), nil, 1_700_000_000, s)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.VerifySignature(); err != nil {
		t.Fatalf("fresh checkpoint must verify: %v", err)
	}
	if c.SignerAddress != s.Address() {
		t.Fatalf("signer address %s want %s", c.SignerAddress, s.Address())
	}
	// JSON round trip (what verifiers read from git) must keep the signature valid.
	raw, _ := json.MarshalIndent(c, "", "  ")
	var back Checkpoint
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if err := back.VerifySignature(); err != nil {
		t.Fatalf("round-tripped checkpoint must verify: %v", err)
	}

	tampers := map[string]func(*Checkpoint){
		"block hash": func(x *Checkpoint) { x.BlockHash = strings.Repeat("0", 64) },
		"state hash": func(x *Checkpoint) { x.StateHash = strings.Repeat("1", 64) },
		"height":     func(x *Checkpoint) { x.Height++ },
		"total":      func(x *Checkpoint) { x.Totals = map[string]uint64{"econ_total_minted_units": 1} },
		"address":    func(x *Checkpoint) { x.SignerAddress = "HMC-0000000000000000" },
		"other key": func(x *Checkpoint) {
			other := testSigner(t)
			x.SignerPubKey = other.PublicKeyHex()
			x.SignerAddress = other.Address()
		},
	}
	for name, mut := range tampers {
		x := back
		x.Totals = map[string]uint64{}
		for k, v := range back.Totals {
			x.Totals[k] = v
		}
		mut(&x)
		if err := x.VerifySignature(); err == nil {
			t.Fatalf("tampered %s must fail verification", name)
		}
	}
}

func TestVerifyChainLinksAndOrdering(t *testing.T) {
	db, svc, addr := testChain(t)
	ctx := context.Background()
	s := testSigner(t)
	c0, err := New(mustSnap(t, db), nil, 1, s)
	if err != nil {
		t.Fatal(err)
	}
	m, err := svc.PoHTargetMod(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var nonce, eval uint64
	tipHash := mustSnap(t, db).BlockHash
	for n := uint64(0); ; n++ {
		if v := chain.PohWorkForIndex(1, n, tipHash, addr); v%m == 0 {
			nonce, eval = n, v
			break
		}
	}
	if _, err := svc.AppendPoHBlock(ctx, addr, nonce, eval, 0.01, m, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := New(mustSnap(t, db), &Checkpoint{Payload: Payload{Height: 5, ChainID: c0.ChainID}}, 2, s); err == nil {
		t.Fatal("tip below previous checkpoint must be refused")
	}
	c1, err := New(mustSnap(t, db), &c0, 2, s)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyChain([]Checkpoint{c0, c1}); err != nil {
		t.Fatalf("valid chain: %v", err)
	}
	if err := VerifyChain([]Checkpoint{c1, c0}); err == nil {
		t.Fatal("out-of-order chain must fail")
	}
	// Replacing c0 with a re-signed variant breaks c1's prev link (history rewrite).
	c0b, err := New(mustSnap(t, db), nil, 1, s)
	if err != nil {
		t.Fatal(err)
	}
	c0b.Height = 0
	msg, _ := c0b.Payload.SignedBytes()
	c0b.Sig = s.SignHex(msg)
	if err := VerifyChain([]Checkpoint{c0b, c1}); err == nil {
		t.Fatal("rewritten earlier checkpoint must break the prev link")
	}
	other := testSigner(t)
	c1other, err := New(mustSnap(t, db), &c0, 2, other)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyChain([]Checkpoint{c0, c1other}); err == nil {
		t.Fatal("signer key change must fail")
	}
}

func TestLoadNeverCreatesKey(t *testing.T) {
	if _, err := nodecrypto.Load(t.TempDir()); err == nil {
		t.Fatal("Load must fail when no seed exists (never mint a new node identity)")
	}
}
