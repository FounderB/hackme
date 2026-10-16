package poolfuzz

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// FindingMirrorRow is a coordinator→Orders findings export row (admin pull).
// Artifact paths are coordinator-local; Orders keeps repro_cmd + detail (input_hex).
type FindingMirrorRow struct {
	ID          string `json:"id"`
	CampaignID  string `json:"campaign_id"`
	FindingType string `json:"finding_type"`
	Severity    string `json:"severity"`
	Title       string `json:"title"`
	InputSHA256 string `json:"input_sha256"`
	ReproCmd    string `json:"repro_cmd"`
	DetailJSON  string `json:"detail_json"`
	CreatedAt   int64  `json:"created_at"`
}

const findingsMirrorMaxLimit = 200

// ListCampaignFindingsMirror returns newest findings for admin Orders ingest.
// Admin-only at the HTTP layer — do not expose to worker tokens.
func (s *Service) ListCampaignFindingsMirror(ctx context.Context, campaignID string, limit int) ([]FindingMirrorRow, error) {
	campaignID = strings.TrimSpace(campaignID)
	if campaignID == "" {
		return nil, fmt.Errorf("poolfuzz: campaign id required")
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > findingsMirrorMaxLimit {
		limit = findingsMirrorMaxLimit
	}
	var exists int
	if err := s.DB.QueryRowContext(ctx, `SELECT 1 FROM fuzz_campaigns WHERE id=? LIMIT 1`, campaignID).Scan(&exists); err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, campaign_id, finding_type, severity, title,
		       COALESCE(input_sha256,''), COALESCE(repro_cmd,''), COALESCE(detail_json,''), created_at
		FROM fuzz_findings
		WHERE campaign_id=?
		ORDER BY created_at DESC
		LIMIT ?`, campaignID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]FindingMirrorRow, 0, limit)
	for rows.Next() {
		var r FindingMirrorRow
		if err := rows.Scan(&r.ID, &r.CampaignID, &r.FindingType, &r.Severity, &r.Title,
			&r.InputSHA256, &r.ReproCmd, &r.DetailJSON, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
