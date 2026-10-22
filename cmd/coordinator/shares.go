package main

import (
	"os"
	"strconv"
	"strings"
)

// Pseudo-share target: shareMod = targetMod / shareDivisor (min 1).
// A share is a nonce in the lease where PoH work % shareMod == 0.
// Expected shares per full lease ≈ batch * divisor / M ≈ divisor * batch / targetMod.
//
// Soft mode (default, !SHARE_STRICT): attempt-pay only. Shares are verified for
// stats / rate checks; they must NOT change the payout formula (rc17.4 audit).
// Strict mode: pay ≈ baseReward / divisor per accepted share; require shares when !found.

const (
	defaultShareDivisor = uint64(1024)
	maxSharesPerSubmit  = 64
	// shareHarvestScanCap must match workerpoh collectShareNonces maxScan until
	// GPU full-batch harvest ships. Soft share-rate expected uses this cap so
	// honest CPU miners are not flagged share_rate_low (~0.06 vs threshold 0.30).
	shareHarvestScanCap = uint64(256 * 1024)
	// defaultShareRateMin: after enough expected shares, accepted/expected below this
	// flags share_rate_low. With soft scan-cap, honest rate ≈ 1.0; cheaters who omit
	// share_nonces stay near 0. Keep SHARE_RATE_REJECT off until GPU harvest if unsure.
	defaultShareRateMin         = 0.30
	defaultShareRateMinExpected = 8.0
)

// softShareScanAttempts caps soft-mode expected shares to the worker harvest window.
func softShareScanAttempts(attempts uint64, shareStrict bool) uint64 {
	if shareStrict || attempts <= shareHarvestScanCap {
		return attempts
	}
	return shareHarvestScanCap
}

func envShareDivisor() uint64 {
	v := strings.TrimSpace(os.Getenv("HACKME_COORDINATOR_SHARE_DIVISOR"))
	if v == "" {
		return defaultShareDivisor // rc17.4+: pseudo-shares on by default
	}
	if v == "0" || strings.EqualFold(v, "off") || strings.EqualFold(v, "false") {
		return 0
	}
	x, err := strconv.ParseUint(v, 10, 64)
	if err != nil || x < 2 {
		return defaultShareDivisor
	}
	if x > 1_000_000 {
		return 1_000_000
	}
	return x
}

func envShareStrict() bool {
	v := strings.TrimSpace(os.Getenv("HACKME_COORDINATOR_SHARE_STRICT"))
	if v == "" {
		return false
	}
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}

func envShareRateMin() float64 {
	v := strings.TrimSpace(os.Getenv("HACKME_COORDINATOR_SHARE_RATE_MIN"))
	if v == "" {
		return defaultShareRateMin
	}
	x, err := strconv.ParseFloat(v, 64)
	if err != nil || x < 0 {
		return defaultShareRateMin
	}
	if x > 1 {
		return 1
	}
	return x
}

func envShareRateMinExpected() float64 {
	v := strings.TrimSpace(os.Getenv("HACKME_COORDINATOR_SHARE_RATE_MIN_EXPECTED"))
	if v == "" {
		return defaultShareRateMinExpected
	}
	x, err := strconv.ParseFloat(v, 64)
	if err != nil || x < 1 {
		return defaultShareRateMinExpected
	}
	return x
}

func envShareRateReject() bool {
	v := strings.TrimSpace(os.Getenv("HACKME_COORDINATOR_SHARE_RATE_REJECT"))
	if v == "" {
		return false
	}
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}

func shareModFor(targetMod, divisor uint64) uint64 {
	if divisor < 2 || targetMod == 0 {
		return 0
	}
	sm := targetMod / divisor
	if sm < 1 {
		sm = 1
	}
	return sm
}

// expectedShares returns attempts * divisor / M (Poisson mean for full-batch harvest).
func expectedShares(attempts, targetMod, divisor uint64) float64 {
	if attempts == 0 || targetMod == 0 || divisor < 2 {
		return 0
	}
	return float64(attempts) * float64(divisor) / float64(targetMod)
}

// validateShareNonces checks each nonce is in [base, base+batch) and satisfies
// PoH work % shareMod == 0. Dedupes, caps at maxSharesPerSubmit. Returns accepted count.
func validateShareNonces(shares []uint64, base, batch, shareMod uint64, formulaVersion int, prevHash, minerAddress string) (accepted []uint64, rejected int) {
	if shareMod == 0 || batch == 0 || len(shares) == 0 {
		return nil, 0
	}
	seen := make(map[uint64]struct{}, len(shares))
	end := base + batch
	if end < base {
		end = ^uint64(0)
	}
	for _, n := range shares {
		if len(accepted) >= maxSharesPerSubmit {
			rejected += len(shares) - len(accepted) - rejected
			break
		}
		if _, ok := seen[n]; ok {
			rejected++
			continue
		}
		seen[n] = struct{}{}
		if n < base || n >= end {
			rejected++
			continue
		}
		if !validFoundNoncePoH(n, shareMod, formulaVersion, prevHash, minerAddress) {
			rejected++
			continue
		}
		accepted = append(accepted, n)
	}
	return accepted, rejected
}

func rewardPerShare(baseRewardHMC float64, divisor uint64) float64 {
	if divisor < 2 || baseRewardHMC <= 0 {
		return 0
	}
	return baseRewardHMC / float64(divisor)
}
