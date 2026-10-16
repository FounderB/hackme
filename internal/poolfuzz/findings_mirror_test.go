package poolfuzz

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"hackme/internal/store"
)

func TestListCampaignFindingsMirrorNewestFirst(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "mirror.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := &Service{DB: db}
	ctx := context.Background()
	id := "mirror-camp-1"
	if err := svc.RegisterCampaign(ctx, Campaign{
		ID: id, CampaignType: "hunt", Title: "t", Status: "running",
		BudgetRuns: 8, Config: map[string]any{"pool_distributed": true, "work_kind": "hunt_shard"},
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	_, err = db.ExecContext(ctx, `INSERT INTO fuzz_findings
		(id, campaign_id, finding_type, severity, title, input_sha256, artifact_path, repro_cmd, detail_json, created_at)
		VALUES ('f-old', ?, 'native_crash', 'high', 'old', 'aa', '', 'repro', '{}', ?)`, id, now-10)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO fuzz_findings
		(id, campaign_id, finding_type, severity, title, input_sha256, artifact_path, repro_cmd, detail_json, created_at)
		VALUES ('f-new', ?, 'native_crash', 'critical', 'new', 'bb', '', 'repro2', '{"input_hex":"41"}', ?)`, id, now)
	if err != nil {
		t.Fatal(err)
	}

	rows, err := svc.ListCampaignFindingsMirror(ctx, id, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("len=%d", len(rows))
	}
	if rows[0].ID != "f-new" || rows[0].Severity != "critical" {
		t.Fatalf("want newest first, got %+v", rows[0])
	}
	if rows[0].DetailJSON == "" {
		t.Fatal("detail missing")
	}
}
