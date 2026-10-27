package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedactPublicActiveRigsPseudonymizesIDs(t *testing.T) {
	out := map[string]any{
		"active_rigs": []any{
			map[string]any{"worker_id": "worker-alice-laptop", "name": "worker-alice-laptop", "hashrate_gh_s": 1.5, "ip": "203.0.113.7"},
			map[string]any{"worker_id": "", "name": "x"},
		},
	}
	redactPublicActiveRigs(out)
	raw, _ := json.Marshal(out)
	s := string(raw)
	if strings.Contains(s, "alice") || strings.Contains(s, "203.0.113.7") {
		t.Fatalf("raw id or ip leaked: %s", s)
	}
	rows := out["active_rigs"].([]any)
	if len(rows) != 1 {
		t.Fatalf("empty worker_id row must be dropped, got %d rows", len(rows))
	}
	row := rows[0].(map[string]any)
	want := publicWorkerKey("worker-alice-laptop")
	if row["worker_id"] != want || row["name"] != want {
		t.Fatalf("want pseudonym %s, got %v", want, row)
	}
	if row["hashrate_gh_s"] != 1.5 {
		t.Fatalf("non-identifying fields must stay: %v", row)
	}
}
