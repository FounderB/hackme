package workersettle

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateApplyPendingRoundTrip(t *testing.T) {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "worker_settlement_state.json")
	raw := []byte(`{
  "workers": {
    "w1": {"settled_hmc": 1.5, "payout_address": "HMC-abc", "pending_settle": {"delta_hmc": 0.25, "payout_address": "HMC-abc", "nonce": 9}}
  },
  "meta": {"last_force_unix": 42}
}`)
	if err := os.WriteFile(jsonPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "worker_settlement.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	ok, err := s.MigrateFromJSON(ctx, jsonPath)
	if err != nil || !ok {
		t.Fatalf("migrate: ok=%v err=%v", ok, err)
	}
	ok2, err := s.MigrateFromJSON(ctx, jsonPath)
	if err != nil || ok2 {
		t.Fatalf("second migrate must be no-op: ok=%v err=%v", ok2, err)
	}
	w, err := s.GetWorker(ctx, "w1")
	if err != nil {
		t.Fatal(err)
	}
	if w.SettledHMC != 1.5 || w.PayoutAddress != "HMC-abc" || w.PendingJSON == "" {
		t.Fatalf("row=%+v", w)
	}
	meta, err := s.GetMeta(ctx)
	if err != nil || meta.LastForceUnix != 42 {
		t.Fatalf("meta=%+v err=%v", meta, err)
	}

	id, err := s.SetPending(ctx, "w2", "HMC-xyz", 0.01, 3)
	if err != nil || id <= 0 {
		t.Fatalf("pending id=%d err=%v", id, err)
	}
	if err := s.AttachTxHash(ctx, "w2", "deadbeef"); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplySettle(ctx, "w2", "HMC-xyz", "deadbeef", 0.01, 0, true); err != nil {
		t.Fatal(err)
	}
	w2, err := s.GetWorker(ctx, "w2")
	if err != nil {
		t.Fatal(err)
	}
	if w2.SettledHMC != 0.01 || w2.PendingJSON != "" || w2.LastTxHash != "deadbeef" {
		t.Fatalf("after apply: %+v", w2)
	}
	n, err := s.CountPendingOutbox(ctx)
	if err != nil || n != 0 {
		// w1 migrated pending is only on worker row, not outbox; w2 applied
		t.Fatalf("pending outbox=%d err=%v", n, err)
	}
	exp, err := s.ExportJSON(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(exp, &doc); err != nil {
		t.Fatal(err)
	}
	workers, _ := doc["workers"].(map[string]any)
	if workers["w1"] == nil || workers["w2"] == nil {
		t.Fatalf("export missing workers: %s", exp)
	}
}

func TestClearPending(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if _, err := s.SetPending(ctx, "w", "HMC-a", 1.0, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearPending(ctx, "w"); err != nil {
		t.Fatal(err)
	}
	w, _ := s.GetWorker(ctx, "w")
	if w.PendingJSON != "" {
		t.Fatalf("pending still set: %q", w.PendingJSON)
	}
}
