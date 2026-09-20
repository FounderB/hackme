package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"hackme/internal/poolfuzz"
)

type coordinatorPoolCampaign struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	RunsDone   int    `json:"runs_done"`
	BudgetRuns int    `json:"budget_runs"`
	Findings   int    `json:"findings"`
}

func (a *app) fetchCoordinatorPoolCampaigns(ctx context.Context) (map[string]coordinatorPoolCampaign, error) {
	base := strings.TrimRight(strings.TrimSpace(a.coordinatorBaseURL()), "/")
	if base == "" {
		return nil, nil
	}
	reqCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, base+"/api/fuzz/pool/campaigns/list?limit=200", nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return nil, fmt.Errorf("coordinator pool list HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	var payload struct {
		Campaigns []coordinatorPoolCampaign `json:"campaigns"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, err
	}
	out := make(map[string]coordinatorPoolCampaign, len(payload.Campaigns))
	for _, c := range payload.Campaigns {
		id := strings.TrimSpace(c.ID)
		if id == "" {
			continue
		}
		out[id] = c
	}
	return out, nil
}

// fetchCoordinatorMarketplaceItems loads full public campaign rows from the coordinator.
// Used when the local SQLite marketplace query fails or returns nothing.
func (a *app) fetchCoordinatorMarketplaceItems(ctx context.Context) ([]map[string]any, error) {
	base := strings.TrimRight(strings.TrimSpace(a.coordinatorBaseURL()), "/")
	if base == "" {
		return nil, nil
	}
	reqCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, base+"/api/fuzz/pool/campaigns/list?limit=200", nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return nil, fmt.Errorf("coordinator pool list HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	var payload struct {
		Campaigns []map[string]any `json:"campaigns"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(payload.Campaigns))
	for _, c := range payload.Campaigns {
		if c == nil {
			continue
		}
		id := strings.TrimSpace(fmt.Sprint(c["id"]))
		if id == "" || id == "<nil>" {
			continue
		}
		c["id"] = id
		c["pool"] = true
		out = append(out, c)
	}
	return out, nil
}

func (a *app) fetchCoordinatorPoolCampaignProgress(ctx context.Context, campaignID string) (coordinatorPoolCampaign, bool) {
	base := strings.TrimRight(strings.TrimSpace(a.coordinatorBaseURL()), "/")
	if base == "" || campaignID == "" {
		return coordinatorPoolCampaign{}, false
	}
	reqCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, base+"/api/fuzz/pool/campaigns/progress?id="+url.QueryEscape(campaignID), nil)
	if err != nil {
		return coordinatorPoolCampaign{}, false
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return coordinatorPoolCampaign{}, false
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return coordinatorPoolCampaign{}, false
	}
	var payload coordinatorPoolCampaign
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return coordinatorPoolCampaign{}, false
	}
	payload.ID = campaignID
	return payload, true
}

func (a *app) fetchCoordinatorFleetCapacity(ctx context.Context) (map[string]any, error) {
	base := strings.TrimRight(strings.TrimSpace(a.coordinatorBaseURL()), "/")
	if base == "" {
		return nil, nil
	}
	reqCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, base+"/api/fuzz/pool/stats", nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return nil, fmt.Errorf("coordinator pool stats HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	var payload map[string]any
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if raw, ok := payload["fleet_capacity"].(map[string]any); ok && raw != nil {
		return raw, nil
	}
	// Fallback: top-level fields from /api/fuzz/pool/stats
	out := map[string]any{}
	for _, k := range []string{
		"fleet_hashrate_gh_s", "est_shards_per_hour", "est_dig_runs_per_hour",
		"hybrid_workers_online", "dig_only_workers_online", "workers_online",
		"ghs_priority_enabled", "note",
	} {
		if v, ok := payload[k]; ok {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func cloneMarketplaceCampaigns(in []map[string]any) []map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(in))
	for _, src := range in {
		if src == nil {
			continue
		}
		dst := make(map[string]any, len(src)+2)
		for k, v := range src {
			dst[k] = v
		}
		out = append(out, dst)
	}
	return out
}

func (a *app) mergeCoordinatorPoolMarketplace(ctx context.Context, items []map[string]any) []map[string]any {
	remote, err := a.fetchCoordinatorPoolCampaigns(ctx)
	if err != nil {
		remote = map[string]coordinatorPoolCampaign{}
	}
	out := a.mergeCoordinatorPoolMarketplaceWithRemote(ctx, items, remote)
	if len(out) > 0 {
		return out
	}
	// Local DB empty/broken: show coordinator marketplace rows directly.
	full, ferr := a.fetchCoordinatorMarketplaceItems(ctx)
	if ferr != nil || len(full) == 0 {
		return out
	}
	filtered := make([]map[string]any, 0, len(full))
	for _, item := range full {
		st, _ := item["status"].(string)
		if strings.EqualFold(strings.TrimSpace(st), "completed") {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(st), "cancelled") {
			continue
		}
		id := strings.TrimSpace(fmt.Sprint(item["id"]))
		runsDone := intFromAny(item["runs_done"])
		budgetRuns := intFromAny(item["budget_runs"])
		escrow, _ := item["escrow_status"].(string)
		// Prefer live progress so budget-exhausted coordinator rows with stale
		// summary runs_done=0 do not resurrect as diggable zombies.
		if id != "" {
			if rc, ok := a.fetchCoordinatorPoolCampaignProgress(ctx, id); ok {
				if rc.RunsDone > runsDone {
					runsDone = rc.RunsDone
					item["runs_done"] = rc.RunsDone
				}
				if st2 := strings.TrimSpace(rc.Status); st2 != "" {
					item["status"] = st2
					st = st2
				}
				if rc.BudgetRuns > 0 {
					budgetRuns = rc.BudgetRuns
					item["budget_runs"] = rc.BudgetRuns
				}
			}
		}
		if strings.EqualFold(strings.TrimSpace(st), "completed") || strings.EqualFold(strings.TrimSpace(st), "cancelled") {
			continue
		}
		if !poolfuzz.IsActivelyDiggable(st, escrow, runsDone, budgetRuns) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func (a *app) mergeCoordinatorPoolMarketplaceWithRemote(ctx context.Context, items []map[string]any, remote map[string]coordinatorPoolCampaign) []map[string]any {
	if remote == nil {
		remote = map[string]coordinatorPoolCampaign{}
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		id, _ := item["id"].(string)
		id = strings.TrimSpace(id)
		rc, ok := remote[id]
		if id != "" && len(items) <= 12 {
			progCtx, progCancel := context.WithTimeout(ctx, 3*time.Second)
			if rc2, ok2 := a.fetchCoordinatorPoolCampaignProgress(progCtx, id); ok2 {
				if !ok || rc2.RunsDone > rc.RunsDone || (rc.RunsDone == 0 && rc2.RunsDone > 0) {
					rc, ok = rc2, true
				}
			}
			progCancel()
		}
		if ok {
			item["runs_done"] = rc.RunsDone
			if rc.Findings > 0 {
				item["findings"] = rc.Findings
			}
			if st := strings.TrimSpace(rc.Status); st != "" {
				item["status"] = st
			}
			if rc.BudgetRuns > 0 {
				item["budget_runs"] = rc.BudgetRuns
			}
			if rd := rc.RunsDone; rc.BudgetRuns > 0 && rd >= rc.BudgetRuns {
				item["status"] = "completed"
			}
			// Persist coordinator terminal status / progress onto the node DB so
			// marketplace does not keep serving local "running" zombies.
			if id != "" && (rc.RunsDone > 0 ||
				strings.EqualFold(strings.TrimSpace(rc.Status), "completed") ||
				strings.EqualFold(strings.TrimSpace(rc.Status), "cancelled")) {
				go func(cid string) {
					syncCtx, syncCancel := context.WithTimeout(context.Background(), 8*time.Second)
					_ = a.syncPoolCampaignProgressFromCoordinator(syncCtx, cid)
					syncCancel()
				}(id)
			}
		}
		st, _ := item["status"].(string)
		if strings.EqualFold(strings.TrimSpace(st), "completed") {
			continue
		}
		out = append(out, item)
	}
	return out
}

func (a *app) syncPoolCampaignProgressFromCoordinator(ctx context.Context, campaignID string) error {
	rc, ok := a.fetchCoordinatorPoolCampaignProgress(ctx, campaignID)
	if !ok {
		return nil
	}
	remoteStatus := strings.TrimSpace(strings.ToLower(rc.Status))
	// Always honor terminal coordinator status — even when runs_done is 0 (cancelled /
	// never-started zombies). Previously RunsDone<=0 returned early and left the node
	// marketplace showing "running / ETA warming up" forever with closed escrow.
	if rc.RunsDone <= 0 && remoteStatus != "completed" && remoteStatus != "cancelled" {
		return nil
	}
	var summaryJSON, status string
	var budgetRuns int
	err := a.db.QueryRowContext(ctx,
		`SELECT status, budget_runs, summary_json FROM fuzz_campaigns WHERE id=?`,
		campaignID).Scan(&status, &budgetRuns, &summaryJSON)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	summary := parseMapJSON(summaryJSON)
	cur := intFromAny(summary["runs_done"])
	changed := false
	if rc.RunsDone > cur {
		summary["runs_done"] = rc.RunsDone
		changed = true
	}
	if rc.Findings > 0 {
		summary["unique_crashes"] = rc.Findings
		changed = true
	}
	nextStatus := strings.TrimSpace(strings.ToLower(status))
	if budgetRuns > 0 && rc.RunsDone >= budgetRuns {
		nextStatus = "completed"
	} else if remoteStatus == "completed" || remoteStatus == "cancelled" {
		nextStatus = remoteStatus
	}
	if nextStatus != strings.TrimSpace(strings.ToLower(status)) {
		changed = true
	}
	if !changed {
		return nil
	}
	summary["pool_workers"] = true
	summary["heartbeat_at"] = time.Now().Unix()
	completedAt := int64(0)
	if nextStatus == "completed" || nextStatus == "cancelled" {
		completedAt = time.Now().Unix()
	}
	_, err = a.db.ExecContext(ctx,
		`UPDATE fuzz_campaigns
		 SET status=?, summary_json=?, completed_at=CASE WHEN ? IN ('completed','cancelled') AND completed_at=0 THEN ? ELSE completed_at END
		 WHERE id=?`,
		nextStatus, marshalMapJSON(summary), nextStatus, completedAt, campaignID)
	if err != nil {
		return err
	}
	if nextStatus == "completed" || nextStatus == "cancelled" {
		a.tryCloseFuzzEscrowForStatus(ctx, campaignID, nextStatus)
	}
	return nil
}
