package chain

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"hackme/internal/store"
)

func TestFindBlockSummaryByTaskHashPrefixAndIndex(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "find.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := New(db)
	addr := "HMC-find"
	if _, _, err := svc.InitGenesis(ctx, addr); err != nil {
		t.Fatal(err)
	}
	m0, err := svc.PoHTargetMod(ctx)
	if err != nil {
		t.Fatal(err)
	}
	nonce, eval := firstPoHHit(m0)
	b, err := svc.AppendPoHBlock(ctx, addr, nonce, eval, 0.01, m0, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(b.Task.ID, "task-") {
		t.Fatalf("unexpected task id %q", b.Task.ID)
	}

	cases := map[string]string{
		"task id":     b.Task.ID,
		"full hash":   b.Hash,
		"upper hash":  strings.ToUpper(b.Hash),
		"prefix 8":    b.Hash[:8],
		"ellipsis":    b.Hash[:16] + "…",
		"index":       "1",
		"padded task": "  " + b.Task.ID + " ",
	}
	for name, q := range cases {
		row, ok, err := svc.FindBlockSummary(ctx, q)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !ok || row.Index != b.Index || row.TaskID != b.Task.ID {
			t.Fatalf("%s: q=%q ok=%v row=%+v want index %d", name, q, ok, row, b.Index)
		}
	}

	misses := []string{"", "task-poh-0000000000000000", b.Hash[:7], "zzzzzzzz", "999999", strings.Repeat("a", 129)}
	for _, q := range misses {
		if _, ok, err := svc.FindBlockSummary(ctx, q); err != nil || ok {
			t.Fatalf("miss q=%q ok=%v err=%v", q, ok, err)
		}
	}
}

func TestFindBlockSummaryTaskLookupUsesIndex(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "plan.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx,
		`EXPLAIN QUERY PLAN SELECT block_index FROM blocks WHERE json_extract(json, '$.task.id') = ? LIMIT 1`, "task-x")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(detail)
	}
	if !strings.Contains(plan.String(), "idx_blocks_task_id") {
		t.Fatalf("task lookup must use idx_blocks_task_id, plan=%q", plan.String())
	}
}
