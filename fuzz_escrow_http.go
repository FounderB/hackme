package main

import (
	"errors"
	"net/http"

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
		writeAPIError(w, http.StatusPaymentRequired, "escrow_failed",
			"escrow open failed", nil)
	}
}
