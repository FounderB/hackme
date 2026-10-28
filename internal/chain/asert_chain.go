package chain

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// blockTimestampQ reads timestamp_unix of the block at index.
func blockTimestampQ(ctx context.Context, q queryRowContext, index uint64) (int64, error) {
	var ts sql.NullInt64
	err := q.QueryRowContext(ctx,
		`SELECT json_extract(json, '$.timestamp_unix') FROM blocks WHERE block_index = ?`, index).Scan(&ts)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("chain: block %d missing (asert)", index)
	}
	if err != nil {
		return 0, err
	}
	if !ts.Valid {
		return 0, fmt.Errorf("chain: block %d has no timestamp (asert)", index)
	}
	return ts.Int64, nil
}

// asertModAfterQ returns the ASERT M for block parentIndex+1, given the parent's timestamp
// (the parent may not be committed yet). The anchor is block activation−1.
func asertModAfterQ(ctx context.Context, q queryRowContext, parentIndex uint64, parentTime int64) (uint64, error) {
	activation := PoHASERTActivationHeight()
	anchorIndex := activation - 1
	if parentIndex < anchorIndex {
		return 0, fmt.Errorf("chain: block %d precedes asert activation %d", parentIndex+1, activation)
	}
	anchorTime := parentTime
	if parentIndex != anchorIndex {
		t, err := blockTimestampQ(ctx, q, anchorIndex)
		if err != nil {
			return 0, err
		}
		anchorTime = t
	}
	return ASERTTargetMod(PoHASERTAnchorMod, anchorIndex, anchorTime, parentIndex, parentTime), nil
}

// asertModForIndexQ returns the ASERT M required for block index (both anchor and parent committed).
func asertModForIndexQ(ctx context.Context, q queryRowContext, index uint64) (uint64, error) {
	if index == 0 {
		return 0, errors.New("chain: genesis has no asert target")
	}
	parentTime, err := blockTimestampQ(ctx, q, index-1)
	if err != nil {
		return 0, err
	}
	return asertModAfterQ(ctx, q, index-1, parentTime)
}
