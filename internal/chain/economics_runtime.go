package chain

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"hackme/internal/block"
)

type RewardWindowBreakdown struct {
	WindowSec   int64   `json:"window_sec"`
	Blocks      int     `json:"blocks"`
	BaseBlocks  int     `json:"base_blocks"`
	OrderBlocks int     `json:"order_blocks"`
	BaseHMC     float64 `json:"base_hmc"`
	OrderHMC    float64 `json:"order_hmc"`
	TotalHMC    float64 `json:"total_hmc"`
}

// BaseRewardForNextBlock returns scheduled base reward for the next PoH block.
func (s *Service) BaseRewardForNextBlock(ctx context.Context) (float64, error) {
	h, _, err := s.Tip(ctx)
	if err != nil {
		return 0, err
	}
	return BaseRewardForBlockIndex(h + 1), nil
}

// RewardWindowBreakdownSince estimates base-vs-order mining rewards from PoH blocks
// in the given time window. For order-linked blocks it uses current task reward row.
// rewardWindowMaxBlocks bounds the reward-window scan (see RewardWindowBreakdownSince).
const rewardWindowMaxBlocks = 4000

func (s *Service) RewardWindowBreakdownSince(ctx context.Context, sinceUnix int64) (RewardWindowBreakdown, error) {
	out := RewardWindowBreakdown{WindowSec: 0}
	// Range-scan only the newest blocks via the block_index index: filtering on json_extract
	// alone scanned every block (~500k rows, 800 MB JSON), never finished inside the /api/metrics
	// budget, and published mining_poh_blocks_last_1h=0 while the chain was producing.
	// rewardWindowMaxBlocks is far above one hour of blocks (ASERT ≈ 120/h).
	rows, err := s.db.QueryContext(ctx,
		`SELECT block_index, json FROM blocks
		 WHERE block_index > MAX(0, (SELECT COALESCE(MAX(block_index), 0) FROM blocks) - ?)
		 AND CAST(json_extract(json, '$.timestamp_unix') AS INTEGER) >= ?
		 AND json_extract(json, '$.task.kind') = ?
		 ORDER BY block_index ASC`,
		rewardWindowMaxBlocks, sinceUnix, block.PoHBlockKind)
	if err != nil {
		return out, err
	}
	defer rows.Close()

	taskReward := map[string]float64{}
	for rows.Next() {
		var idx uint64
		var raw string
		if err := rows.Scan(&idx, &raw); err != nil {
			return out, err
		}
		var b block.Block
		if err := json.Unmarshal([]byte(raw), &b); err != nil {
			continue
		}
		out.Blocks++
		orderID := orderTaskIDFromPayload(b.Task.Payload)
		if orderID == "" {
			out.BaseBlocks++
			out.BaseHMC += BaseRewardForBlockIndex(b.Index)
			continue
		}
		out.OrderBlocks++
		rw, ok := taskReward[orderID]
		if !ok {
			rw = 0
			_ = s.db.QueryRowContext(ctx, `SELECT reward FROM tasks WHERE id = ?`, orderID).Scan(&rw)
			taskReward[orderID] = rw
		}
		out.OrderHMC += rw
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	out.TotalHMC = out.BaseHMC + out.OrderHMC
	return out, nil
}

func orderTaskIDFromPayload(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		return ""
	}
	v, _ := m["order_task_id"].(string)
	return strings.TrimSpace(v)
}

// BurnImpactPct returns burned/minted ratio in percent for high-level economics view.
func BurnImpactPct(ec EconomicsSnapshot) float64 {
	if ec.TotalMinted <= 1e-12 {
		return 0
	}
	return (ec.TotalBurned / ec.TotalMinted) * 100.0
}

// Ensure sql import is kept when build tags vary.
var _ = sql.ErrNoRows
