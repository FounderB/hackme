package poolfuzz

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"hackme/internal/fuzzengine"
)

// ListPublicCampaigns returns redacted pool campaigns for the marketplace UI.
// Hot path: prefer summary_json / one escrow JOIN — avoid N+1 COUNT scans under claim load.
func (s *Service) ListPublicCampaigns(ctx context.Context, limit int) ([]map[string]any, error) {
	if s == nil || s.DB == nil {
		return nil, fmt.Errorf("poolfuzz: no database")
	}
	const maxPublicCampaigns = 200
	if limit <= 0 {
		limit = 50
	}
	if limit > maxPublicCampaigns {
		limit = maxPublicCampaigns
	}
	// Bound list work so marketplace never parks behind SQLite claim storms.
	listCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	rows, err := s.DB.QueryContext(listCtx,
		`SELECT c.id, c.campaign_type, c.status, c.title, c.owner_ref, c.budget_runs,
		        c.summary_json, c.config_json, c.created_at, c.completed_at,
		        COALESCE(e.status, '')
		 FROM fuzz_campaigns c
		 LEFT JOIN fuzz_campaign_escrow e ON e.campaign_id = c.id
		 WHERE json_extract(c.config_json, '$.pool_distributed') IN (1, 'true', '1')
		   AND c.status IN ('planned', 'running')
		 ORDER BY c.created_at DESC
		 LIMIT ?`, limit*4)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0, limit)
	for rows.Next() {
		var id, ctype, status, title, ownerRef, summaryJSON, cfgJSON, escrowStatus string
		var budgetRuns int
		var createdAt, completedAt int64
		if err := rows.Scan(&id, &ctype, &status, &title, &ownerRef, &budgetRuns, &summaryJSON, &cfgJSON, &createdAt, &completedAt, &escrowStatus); err != nil {
			return nil, err
		}
		cfg := parseConfigJSON(cfgJSON)
		if !IsMarketplaceCampaign(status, id, title, ownerRef, cfg) {
			continue
		}
		summary := parseConfigJSON(summaryJSON)
		// Marketplace list must stay cheap: trust summary / escrow, not live COUNT(*).
		runsDone := intFromJSON(summary["runs_done"])
		// Stale summary_json (runs_done stuck at 0 while work is done) is how
		// coordinator "running" zombies reappear as ETA warming up after node close.
		if runsDone == 0 && strings.EqualFold(strings.TrimSpace(status), "running") && budgetRuns > 0 {
			var live int
			_ = s.DB.QueryRowContext(listCtx,
				`SELECT COUNT(*) FROM fuzz_work_items WHERE campaign_id=? AND status='done'`, id).Scan(&live)
			if live > 0 {
				runsDone = live
			}
		}
		findings := intFromJSON(summary["unique_crashes"])
		if findings <= 0 {
			findings = intFromJSON(summary["findings"])
		}
		if !IsActivelyDiggable(status, escrowStatus, runsDone, budgetRuns) {
			continue
		}
		budgetHMC := 0.0
		if v, ok := cfg["budget_hmc"]; ok {
			budgetHMC = floatFromJSON(v)
		}
		item := map[string]any{
			"id":             id,
			"campaign_type":  ctype,
			"status":         status,
			"title":          title,
			"budget_runs":    budgetRuns,
			"budget_hmc":     budgetHMC,
			"runs_done":      runsDone,
			"unique_crashes": findings,
			"findings":       findings,
			"pool":           true,
			"created_at":     createdAt,
			"completed_at":   completedAt,
		}
		if escrowStatus != "" {
			item["escrow_status"] = escrowStatus
		}
		if fe, ok := summary["fuzz_engine"].(map[string]any); ok {
			item["check_semantics"] = fe["check_semantics"]
			item["depth_tier"] = fe["depth_tier"]
			item["input_mode"] = fe["input_mode"]
			if v, ok := fe["max_input_bytes"]; ok {
				item["max_input_bytes"] = v
			}
		} else if cs := strings.TrimSpace(jsonString(cfg["check_semantics"])); cs != "" {
			item["check_semantics"] = cs
		}
		if dt := strings.TrimSpace(jsonString(cfg["depth_tier"])); dt != "" {
			item["depth_tier"] = dt
		}
		if budgetHMC > 0 {
			shards := budgetRuns
			share := 0.20
			if strings.TrimSpace(jsonString(cfg["escrow_split"])) == "50_50" || strings.EqualFold(ctype, "hunt") {
				share = 0.50
				if v := intFromJSON(cfg["budget_shards"]); v >= 8 {
					shards = v
				}
			}
			if shards >= 8 {
				per := (budgetHMC * share) / float64(shards)
				item["per_run_hmc"] = per
				if IsHuntCampaign(cfg) {
					item["per_shard_hmc"] = per
				}
			}
		}
		if native, ok := summary["native"].(map[string]any); ok {
			item["native_status"] = native["status"]
		} else {
			item["native_status"] = "n/a"
		}
		out = append(out, item)
		if len(out) >= limit {
			break
		}
	}
	return out, rows.Err()
}

func floatFromJSON(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	default:
		return 0
	}
}

func escrowStatusForCampaign(ctx context.Context, db *sql.DB, id string) string {
	if db == nil {
		return ""
	}
	var st string
	err := db.QueryRowContext(ctx, `SELECT status FROM fuzz_campaign_escrow WHERE campaign_id=?`, id).Scan(&st)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(st)
}

func crashClassFindingsCount(ctx context.Context, db *sql.DB, id string) int {
	if db == nil {
		return 0
	}
	rows, err := db.QueryContext(ctx, `SELECT finding_type FROM fuzz_findings WHERE campaign_id=?`, id)
	if err != nil {
		return 0
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var ft string
		if err := rows.Scan(&ft); err != nil {
			continue
		}
		if fuzzengine.IsCrashClass(ft) {
			n++
		}
	}
	return n
}
