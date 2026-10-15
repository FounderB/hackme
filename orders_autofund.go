package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"hackme/internal/chain"
)

// ordersAutoFundError is a stable HTTP-mappable failure from Fund / auto-fund-on-Start.
type ordersAutoFundError struct {
	HTTPStatus int
	Code       string
	Msg        string
	Extra      map[string]any
}

func (e *ordersAutoFundError) Error() string {
	if e == nil {
		return "orders auto-fund error"
	}
	return e.Msg
}

func writeOrdersAutoFundFailed(w http.ResponseWriter, err error) {
	var af *ordersAutoFundError
	if errors.As(err, &af) && af != nil {
		st := af.HTTPStatus
		if st == 0 {
			st = http.StatusBadRequest
		}
		writeAPIError(w, st, af.Code, af.Msg, af.Extra)
		return
	}
	writeAPIError(w, http.StatusInternalServerError, "fund_failed", err.Error(), nil)
}

// ordersFundAmountForShortfall rounds a spendable shortfall up to fund bounds.
func ordersFundAmountForShortfall(shortfallHMC float64) (float64, error) {
	if shortfallHMC <= 1e-12 {
		return 0, nil
	}
	amt := shortfallHMC
	if amt < chain.OrdersFundMinHMC {
		amt = chain.OrdersFundMinHMC
	}
	if amt > chain.OrdersFundMaxHMC+1e-12 {
		return 0, &ordersAutoFundError{
			HTTPStatus: http.StatusPaymentRequired,
			Code:       "fund_amount_too_large",
			Msg: fmt.Sprintf("need %.2f HMC more spendable but max single fund is %.0f HMC — Fund Orders in chunks or lower budget",
				shortfallHMC, chain.OrdersFundMaxHMC),
			Extra: map[string]any{"shortfall_hmc": shortfallHMC, "max_fund_hmc": chain.OrdersFundMaxHMC},
		}
	}
	return amt, nil
}

// ensureOrdersSpendableForBudget tops up local spendable from on-chain Wallet when short.
// No-op when local balance already covers needHMC. On non-follower nodes (no canonical fund path),
// returns a clear Wallet insufficient error instead of attempting Fund.
func (a *app) ensureOrdersSpendableForBudget(ctx context.Context, needHMC float64) (*chain.OrdersFundResult, error) {
	if a == nil || a.chain == nil {
		return nil, &ordersAutoFundError{
			HTTPStatus: http.StatusServiceUnavailable,
			Code:       "chain_unavailable",
			Msg:        "chain unavailable",
		}
	}
	if needHMC <= 1e-12 {
		return nil, nil
	}
	_, bal, err := a.chain.Wallet(ctx)
	if err != nil {
		return nil, err
	}
	if bal+1e-9 >= needHMC {
		return nil, nil
	}
	shortfall := needHMC - bal

	if _, cerr := a.ordersFundCanonicalBase(); cerr != nil {
		return nil, &ordersAutoFundError{
			HTTPStatus: http.StatusPaymentRequired,
			Code:       "escrow_failed",
			Msg: fmt.Sprintf("insufficient Wallet HMC for escrow (have %.4f, need %.4f). Top up on-chain balance on #wallet",
				bal, needHMC),
			Extra: map[string]any{
				"spendable_hmc": bal,
				"need_hmc":      needHMC,
				"shortfall_hmc": shortfall,
			},
		}
	}

	fundAmt, ferr := ordersFundAmountForShortfall(shortfall)
	if ferr != nil {
		return nil, ferr
	}
	if fundAmt <= 1e-12 {
		return nil, nil
	}

	out, err := a.fundOrdersSpendable(ctx, fundAmt)
	if err != nil {
		return nil, err
	}
	if out != nil && out.Pending {
		return nil, &ordersAutoFundError{
			HTTPStatus: http.StatusConflict,
			Code:       "fund_pending_confirm",
			Msg:        "on-chain fund submitted but not yet included — retry Start in a few seconds, or POST /api/orders/fund/confirm",
			Extra: map[string]any{
				"tx_hash":      out.TxHash,
				"amount_hmc":   out.AmountHMC,
				"amount_units": out.AmountUnits,
			},
		}
	}
	_, balAfter, _ := a.chain.Wallet(ctx)
	if balAfter+1e-9 < needHMC {
		return nil, &ordersAutoFundError{
			HTTPStatus: http.StatusPaymentRequired,
			Code:       "escrow_failed",
			Msg: fmt.Sprintf("after auto-fund spendable %.4f HMC still below need %.4f — check Wallet (#wallet) on-chain balance",
				balAfter, needHMC),
			Extra: map[string]any{
				"spendable_hmc": balAfter,
				"need_hmc":      needHMC,
				"funded":        out.Funded,
			},
		}
	}
	if out != nil {
		return out.Funded, nil
	}
	return nil, nil
}
