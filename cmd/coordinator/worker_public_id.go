package main

import (
	"encoding/hex"
	"strings"

	"github.com/zeebo/blake3"
)

// publicWorkerKey returns a stable 8-byte hex pseudonym for a worker id (public APIs).
func publicWorkerKey(workerID string) string {
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return ""
	}
	sum := blake3.Sum256([]byte("hackme-public-worker-v1|" + workerID))
	return hex.EncodeToString(sum[:8])
}

// redactPublicActiveRigs pseudonymizes active_rigs rows on unauthenticated stats. Raw
// worker_id/name there would also let anyone recompute and de-anonymize the workers map keys.
func redactPublicActiveRigs(out map[string]any) {
	rows, ok := out["active_rigs"].([]any)
	if !ok || len(rows) == 0 {
		return
	}
	red := make([]any, 0, len(rows))
	for _, r := range rows {
		row, ok := r.(map[string]any)
		if !ok {
			continue
		}
		key := publicWorkerKey(asStringAny(row["worker_id"]))
		if key == "" {
			continue
		}
		cp := make(map[string]any, len(row))
		for k, v := range row {
			switch k {
			case "ip", "client_ip", "last_client_ip":
				continue
			}
			cp[k] = v
		}
		cp["worker_id"] = key
		cp["name"] = key
		red = append(red, cp)
	}
	out["active_rigs"] = red
}

func asStringAny(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

// publicWorkersMap re-keys workers under publicWorkerKey and drops raw hostname-like ids.
func publicWorkersMap(workers map[string]workerPayoutStat) map[string]workerPayoutStat {
	out := make(map[string]workerPayoutStat, len(workers))
	for id, st := range workers {
		key := publicWorkerKey(id)
		if key == "" {
			continue
		}
		st = redactPublicWorkerStat(st)
		if prev, ok := out[key]; ok {
			merged, _ := mergeWorkerStat(prev, st)
			out[key] = merged
			continue
		}
		out[key] = st
	}
	return out
}
