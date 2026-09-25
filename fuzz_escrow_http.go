package main

import (
	"errors"
	"net/http"
	"strings"

	"hackme/internal/chain"
)

// writeFuzzEscrowFailed maps escrow open errors to stable HTTP codes/messages.
// Never surface driver strings (e.g. sql.ErrNoRows) to clients.
func writeFuzzEscrowFailed(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, chain.ErrFuzzWalletMissing):
		writeAPIError(w, http.StatusPaymentRequired, "escrow_unavailable",
			"local wallet not initialized (genesis required)", nil)
	case errors.Is(err, chain.ErrFuzzInsufficientBalance):
		writeAPIError(w, http.StatusPaymentRequired, "escrow_failed",
			"insufficient wallet balance for escrow", nil)
	default:
		// Surface stable validation messages (shards/budget floors) without sql driver leaks.
		msg := "escrow open failed"
		if err != nil {
			s := err.Error()
			switch {
			case strings.Contains(s, "budget_shards below minimum"),
				strings.Contains(s, "per-shard payout below minimum"),
				strings.Contains(s, "budget below minimum"),
				strings.Contains(s, "budget above maximum"),
				strings.Contains(s, "already exists"):
				msg = s
			}
		}
		writeAPIError(w, http.StatusPaymentRequired, "escrow_failed", msg, nil)
	}
}
