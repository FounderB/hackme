package main

import (
	"os"
	"strconv"
	"strings"
)

// Pseudo-share target: shareMod = targetMod / shareDivisor (min 1).
// A share is a nonce in the lease where PoH work % shareMod == 0.
// Expected shares per block ≈ shareDivisor; pay ≈ baseReward / shareDivisor per share.

const (
	defaultShareDivisor = uint64(1024)
	maxSharesPerSubmit  = 64
)

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
