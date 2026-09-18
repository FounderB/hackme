// Package poolauth holds small shared pool auth/policy helpers for node + coordinator.
package poolauth

import (
	"os"
	"strings"
)

// EnvTruthy reads a boolean-ish env var (1|true|yes|on).
func EnvTruthy(key string, def bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if v == "" {
		return def
	}
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// HybridSignerEnabled matches coordinator workManager policy:
// default ON when HACKME_COORDINATOR_WORKER_TOKEN is set and ALLOW_INSECURE is off;
// HACKME_POOL_HYBRID_SIGNER_ENABLED overrides when explicitly set.
func HybridSignerEnabled() bool {
	workerToken := strings.TrimSpace(os.Getenv("HACKME_COORDINATOR_WORKER_TOKEN"))
	allowInsecure := EnvTruthy("HACKME_COORDINATOR_ALLOW_INSECURE", false)
	enabled := workerToken != "" && !allowInsecure
	if v := strings.TrimSpace(strings.ToLower(os.Getenv("HACKME_POOL_HYBRID_SIGNER_ENABLED"))); v != "" {
		enabled = v == "1" || v == "true" || v == "yes" || v == "on"
	}
	return enabled
}

// WorkerPayoutMapRaw returns the operator payout map CSV from env.
// Accepts HACKME_WORKER_PAYOUT_MAP first, then WORKER_PAYOUT_MAP (same as settlement_api).
func WorkerPayoutMapRaw() string {
	raw := strings.TrimSpace(os.Getenv("HACKME_WORKER_PAYOUT_MAP"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("WORKER_PAYOUT_MAP"))
	}
	return raw
}
