package chain

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"hackme/internal/fuzzescrow"
)

const (
	metaOrdersFundTxPrefix = "orders_fund_tx:"
	OrdersFundMemoPrefix   = "orders_fund_v1"
)

// Orders fund amount bounds (HMC).
const (
	OrdersFundMinHMC = fuzzescrow.HuntMinLiteBudgetHMC // 15
	OrdersFundMaxHMC = fuzzescrow.MaxCampaignBudgetHMC // 500
)

// OrdersFundResult is returned after crediting local spendable from a proven on-chain fund tx.
type OrdersFundResult struct {
	TxHash           string  `json:"tx_hash"`
	AmountHMC        float64 `json:"amount_hmc"`
	AmountUnits      uint64  `json:"amount_units"`
	SpendableAfter   float64 `json:"spendable_after_hmc"`
	IdempotentReplay bool    `json:"idempotent_replay,omitempty"`
	FromAddress      string  `json:"from_address"`
}

func ordersFundMetaKey(txHash string) string {
	return metaOrdersFundTxPrefix + strings.TrimSpace(strings.ToLower(txHash))
}

// ValidateOrdersFundAmountHMC checks fund amount against production bounds.
func ValidateOrdersFundAmountHMC(amountHMC float64) error {
	if amountHMC+1e-12 < OrdersFundMinHMC {
		return fmt.Errorf("orders fund: amount below minimum %.0f HMC", OrdersFundMinHMC)
	}
	if amountHMC > OrdersFundMaxHMC+1e-12 {
		return fmt.Errorf("orders fund: amount above maximum %.0f HMC", OrdersFundMaxHMC)
	}
	if HMCToUnits(amountHMC) == 0 {
		return errors.New("orders fund: amount rounds to zero units")
	}
	return nil
}

// OrdersFundMemo builds the on-chain transfer memo for a fund payment.
func OrdersFundMemo(nodeAddr string) string {
	nodeAddr = strings.TrimSpace(nodeAddr)
	if nodeAddr == "" {
		return OrdersFundMemoPrefix
	}
	return OrdersFundMemoPrefix + ":" + nodeAddr
}

// MemoIsOrdersFund reports whether a transfer memo is an Orders fund payment.
func MemoIsOrdersFund(memo string) bool {
	m := strings.TrimSpace(memo)
	return m == OrdersFundMemoPrefix || strings.HasPrefix(m, OrdersFundMemoPrefix+":")
}

// CreditOrdersFund credits local dual-ledger spendable once per canonical tx_hash.
// Does not debit local genesis treasury — on-chain payment to DevFeeAddress is the real debit.
func (s *Service) CreditOrdersFund(ctx context.Context, fromAddr, txHash string, amountUnits uint64) (*OrdersFundResult, error) {
	fromAddr = strings.TrimSpace(fromAddr)
	txHash = strings.TrimSpace(strings.ToLower(txHash))
	if fromAddr == "" {
		return nil, errors.New("orders fund: from address required")
	}
	if txHash == "" || len(txHash) < 16 {
		return nil, errors.New("orders fund: tx_hash required")
	}
	if amountUnits == 0 {
		return nil, errors.New("orders fund: amount_units required")
	}
	amountHMC := UnitsToHMC(amountUnits)
	if err := ValidateOrdersFundAmountHMC(amountHMC); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	metaKey := ordersFundMetaKey(txHash)
	var raw string
	err = tx.QueryRowContext(ctx, `SELECT value FROM meta WHERE key=?`, metaKey).Scan(&raw)
	if err == nil && strings.TrimSpace(raw) != "" {
		var prev OrdersFundResult
		if json.Unmarshal([]byte(raw), &prev) == nil && prev.TxHash != "" {
			if prev.AmountUnits != amountUnits {
				return nil, fmt.Errorf("orders fund: tx_hash already credited for %d units (got %d)", prev.AmountUnits, amountUnits)
			}
			prev.IdempotentReplay = true
			_ = tx.Rollback()
			return &prev, nil
		}
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	var walletAddr string
	var balUnits uint64
	if err := tx.QueryRowContext(ctx, `SELECT address, COALESCE(balance_units,0) FROM wallet WHERE id=1`).Scan(&walletAddr, &balUnits); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("orders fund: wallet row missing")
		}
		return nil, err
	}
	walletAddr = strings.TrimSpace(walletAddr)
	if !strings.EqualFold(walletAddr, fromAddr) {
		return nil, fmt.Errorf("orders fund: from %s is not primary wallet %s", fromAddr, walletAddr)
	}

	if err := s.creditUnits(ctx, tx, walletAddr, amountUnits); err != nil {
		return nil, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(balance_units,0) FROM wallet WHERE id=1`).Scan(&balUnits); err != nil {
		return nil, err
	}

	out := OrdersFundResult{
		TxHash:         txHash,
		AmountHMC:      amountHMC,
		AmountUnits:    amountUnits,
		SpendableAfter: UnitsToHMC(balUnits),
		FromAddress:    walletAddr,
	}
	blob, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		metaKey, string(blob)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &out, nil
}

// OrdersFundAlreadyApplied reports whether tx_hash was already credited.
func (s *Service) OrdersFundAlreadyApplied(ctx context.Context, txHash string) (OrdersFundResult, bool, error) {
	txHash = strings.TrimSpace(strings.ToLower(txHash))
	if txHash == "" {
		return OrdersFundResult{}, false, nil
	}
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key=?`, ordersFundMetaKey(txHash)).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return OrdersFundResult{}, false, nil
	}
	if err != nil {
		return OrdersFundResult{}, false, err
	}
	var prev OrdersFundResult
	if json.Unmarshal([]byte(raw), &prev) != nil || prev.TxHash == "" {
		return OrdersFundResult{}, false, nil
	}
	return prev, true, nil
}
