package chain

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const metaOrdersStarterFunded = "orders_starter_funded_v1"

// DefaultOrdersStarterHMC is credited once to the primary wallet for B2B Orders desktops
// so Dig/Hunt escrow works without understanding on-chain vs local ledgers.
const DefaultOrdersStarterHMC = 100.0

// OrdersStarterHMCFromEnv returns starter budget (HMC). 0 disables bootstrap credit.
func OrdersStarterHMCFromEnv() float64 {
	v := strings.TrimSpace(os.Getenv("HACKME_ORDERS_STARTER_HMC"))
	if v == "" {
		return DefaultOrdersStarterHMC
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil || n < 0 {
		return DefaultOrdersStarterHMC
	}
	return n
}

// DesktopOrdersProfileEnabled is true when this process is the Orders (B2B) desktop profile.
func DesktopOrdersProfileEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("DESKTOP_PROFILE")), "orders")
}

// BootstrapOrdersSpendable ensures local genesis exists and the primary wallet has a one-time
// starter HMC balance for order escrow (moved from local genesis treasury). Idempotent.
func (s *Service) BootstrapOrdersSpendable(ctx context.Context, nodeAddr string, starterHMC float64) (credited float64, err error) {
	nodeAddr = strings.TrimSpace(nodeAddr)
	if nodeAddr == "" {
		return 0, fmt.Errorf("orders bootstrap: empty node address")
	}
	if starterHMC < 0 {
		starterHMC = 0
	}

	has, err := s.HasGenesis(ctx)
	if err != nil {
		return 0, err
	}
	if !has {
		if _, _, err := s.InitGenesis(ctx, nodeAddr); err != nil && err != ErrGenesisExists {
			return 0, err
		}
	}

	if starterHMC <= 0 {
		return 0, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var funded string
	_ = s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, metaOrdersStarterFunded).Scan(&funded)
	if strings.TrimSpace(funded) == "1" {
		return 0, nil
	}

	var walletAddr string
	var balUnits uint64
	if err := s.db.QueryRowContext(ctx, `SELECT address, COALESCE(balance_units,0) FROM wallet WHERE id=1`).Scan(&walletAddr, &balUnits); err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("orders bootstrap: wallet row missing after genesis")
		}
		return 0, err
	}
	walletAddr = strings.TrimSpace(walletAddr)
	if walletAddr == "" {
		walletAddr = nodeAddr
	}

	needUnits := HMCToUnits(starterHMC)
	if balUnits >= needUnits {
		_, _ = s.db.ExecContext(ctx, `INSERT INTO meta (key, value) VALUES (?, '1') ON CONFLICT(key) DO UPDATE SET value='1'`, metaOrdersStarterFunded)
		return 0, nil
	}
	credit := needUnits - balUnits
	treasury := strings.TrimSpace(DevFeeAddress)
	if treasury == "" {
		return 0, fmt.Errorf("orders bootstrap: empty DevFeeAddress")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var treasUnits uint64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(balance_units,0) FROM accounts WHERE address = ?`, treasury).Scan(&treasUnits); err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("orders bootstrap: treasury account missing")
		}
		return 0, err
	}
	if treasUnits < credit {
		return 0, fmt.Errorf("orders bootstrap: treasury has %d units, need %d", treasUnits, credit)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE accounts SET balance_units = balance_units - ?, updated_at = strftime('%s','now') WHERE address = ? AND balance_units >= ?`,
		credit, treasury, credit); err != nil {
		return 0, err
	}
	if err := s.creditUnits(ctx, tx, walletAddr, credit); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO meta (key, value) VALUES (?, '1') ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		metaOrdersStarterFunded); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return float64(credit) / float64(UnitsPerHMC), nil
}
