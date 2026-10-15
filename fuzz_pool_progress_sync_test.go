package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"hackme/internal/chain"
	"hackme/internal/store"
)

// tryClose must refresh coordinator progress before finalize without sync→close recursion.
func TestTryCloseRefreshesPoolProgressBeforeFinalize(t *testing.T) {
	coord := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/fuzz/pool/campaigns/progress":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true, "id": "pool-sync-camp", "status": "completed",
				"runs_done": 8, "budget_runs": 8,
			})
		case "/api/fuzz/pool/settle/outbox":
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
		case "/api/fuzz/pool/settle/outbox/ack":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer coord.Close()

	t.Setenv("HACKME_POOL_COORDINATOR_URL", coord.URL)
	t.Setenv("HACKME_COORDINATOR_ADMIN_TOKEN", "coord-admin")

	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "sync.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := chain.New(db)
	payer := "HMC-1111111111111111"
	if _, _, err := svc.InitGenesis(ctx, payer); err != nil {
		t.Fatal(err)
	}
	preFundMainEscrow(t, ctx, db, payer, 50)
	const camp = "pool-sync-camp"
	if _, err := db.ExecContext(ctx,
		`INSERT INTO fuzz_campaigns
		 (id, campaign_type, status, title, description, owner_ref, task_id, target_ref, budget_runs, budget_seconds, config_json, summary_json, report_token_hash, report_token_issued_at, created_at, started_at, completed_at)
		 VALUES (?, 'hunt', 'running', 't', '', '', '', '', 8, 3600, '{"pool_distributed":true}', '{}', '', 0, 1, 0, 0)`,
		camp); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenHuntEscrow(ctx, camp, 20.0, 8); err != nil {
		t.Fatal(err)
	}
	a := &app{db: db, chain: svc}

	a.tryCloseFuzzEscrowForStatus(ctx, camp, "completed")

	var summaryJSON string
	if err := db.QueryRowContext(ctx, `SELECT summary_json FROM fuzz_campaigns WHERE id=?`, camp).Scan(&summaryJSON); err != nil {
		t.Fatal(err)
	}
	sum := parseMapJSON(summaryJSON)
	if intFromAny(sum["runs_done"]) != 8 {
		t.Fatalf("summary runs_done=%v want 8 from coordinator refresh", sum["runs_done"])
	}
}
