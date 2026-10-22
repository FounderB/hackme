package chain

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"hackme/internal/block"
	"hackme/internal/sandbox"
)

// SubmitOrderPoHSolve validates a pool-reported PoH hit against an open order's WASM gate
// and appends the next chain block. Order escrow is credited to minerAddress.
func (s *Service) SubmitOrderPoHSolve(ctx context.Context, minerAddress string, nonce, targetMod uint64, orderTaskID string) (*block.Block, error) {
	minerAddress = strings.TrimSpace(minerAddress)
	orderTaskID = strings.TrimSpace(orderTaskID)
	if minerAddress == "" {
		return nil, errors.New("chain: miner_address required")
	}
	if orderTaskID == "" {
		return nil, errors.New("chain: order_task_id required")
	}
	if !strings.HasPrefix(minerAddress, "HMC-") {
		return nil, fmt.Errorf("chain: invalid miner_address %q", minerAddress)
	}

	// Always use canonical chain target_mod — ignore caller-supplied difficulty
	// (easy mods would drain order escrow without real PoH work).
	chainMod, err := s.PoHTargetMod(ctx)
	if err != nil {
		return nil, err
	}
	targetMod = ClampPoHTargetMod(chainMod)
	var tipHash string
	var maxIdx sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = 'tip_hash'`).Scan(&tipHash); err != nil {
		return nil, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(block_index) FROM blocks`).Scan(&maxIdx); err != nil {
		return nil, err
	}
	if !maxIdx.Valid {
		return nil, errors.New("chain: empty (genesis required)")
	}
	nextIdx := uint64(maxIdx.Int64) + 1
	eval := PohWorkForIndex(nextIdx, nonce, tipHash, minerAddress)
	if targetMod == 0 || eval%targetMod != 0 {
		return nil, errors.New("chain: invalid poh solution for submitted target_mod")
	}

	var manifestJSON string
	var reward float64
	now := time.Now().Unix()
	if err := s.db.QueryRowContext(ctx,
		`SELECT manifest_json, reward FROM tasks WHERE id = ? AND status = ? AND (expires_at = 0 OR expires_at > ?)`,
		orderTaskID, TaskStatusOpen, now,
	).Scan(&manifestJSON, &reward); err != nil {
		return nil, fmt.Errorf("chain: order task %q not open: %w", orderTaskID, err)
	}
	wasm, err := s.WasmCheckFromManifestJSON([]byte(manifestJSON))
	if err != nil {
		return nil, err
	}
	if len(wasm) == 0 {
		return nil, errors.New("chain: order has no wasm gate")
	}
	ok, err := sandbox.InvokeCheck(ctx, wasm, nonce)
	if err != nil {
		return nil, fmt.Errorf("chain: wasm gate: %w", err)
	}
	if !ok {
		return nil, errors.New("chain: wasm gate rejected nonce (check returned 0)")
	}

	return s.AppendPoHBlock(ctx, minerAddress, nonce, eval, reward, targetMod, orderTaskID)
}
