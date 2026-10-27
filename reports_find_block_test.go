package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"hackme/internal/chain"
	"hackme/internal/store"
)

func TestReportsBlocksQueryFindsTaskAcrossChain(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "q.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := chain.New(db)
	if _, _, err := svc.InitGenesis(ctx, "HMC-1111111111111111"); err != nil {
		t.Fatal(err)
	}
	genesis, ok, err := svc.GetBlockSummaryByIndex(ctx, 0)
	if err != nil || !ok {
		t.Fatalf("genesis summary ok=%v err=%v", ok, err)
	}
	a := &app{chain: svc}

	get := func(q string) map[string]any {
		t.Helper()
		rr := httptest.NewRecorder()
		a.handleReportsBlocks(rr, httptest.NewRequest(http.MethodGet, "/api/reports/blocks?q="+url.QueryEscape(q), nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("q=%q status %d body %s", q, rr.Code, rr.Body.String())
		}
		var out map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	hit := get(genesis.Hash[:12])
	if hit["lookup"] != true || hit["found"] != true {
		t.Fatalf("prefix lookup: %v", hit)
	}
	rows, _ := hit["blocks"].([]any)
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %v", hit["blocks"])
	}
	if row, _ := rows[0].(map[string]any); row["hash"] != genesis.Hash {
		t.Fatalf("wrong block %v", row)
	}

	miss := get("task-poh-ffffffffffffffff")
	if miss["lookup"] != true || miss["found"] != false {
		t.Fatalf("miss lookup: %v", miss)
	}
	if rows, _ := miss["blocks"].([]any); len(rows) != 0 {
		t.Fatalf("miss must return empty blocks, got %v", miss["blocks"])
	}
}
