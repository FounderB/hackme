package main

import (
	"os"
	"strconv"
	"strings"
)

// Pool daily pay budget keeps SHARE_STRICT accrual solvent vs chain emission.
// Default: base_reward × (86400 / block_sec) ≈ 0.01 × 2880 = 28.8 HMC/day.
// Override: HACKME_COORDINATOR_POOL_DAILY_BUDGET_HMC (0 disables the cap).

const (
	defaultPoolBlockSec = 30.0
)

func poolBlockSec() float64 {
	v := strings.TrimSpace(os.Getenv("HACKME_COORDINATOR_POOL_BLOCK_SEC"))
	if v == "" {
		return defaultPoolBlockSec
	}
	x, err := strconv.ParseFloat(v, 64)
	if err != nil || x < 1 || x > 600 {
		return defaultPoolBlockSec
	}
	return x
}

func (m *workManager) poolDailyBudgetHMC() float64 {
	if m == nil {
		return 0
	}
	if v := strings.TrimSpace(os.Getenv("HACKME_COORDINATOR_POOL_DAILY_BUDGET_HMC")); v != "" {
		x, err := strconv.ParseFloat(v, 64)
		if err == nil && x >= 0 {
			return x
		}
	}
	base := m.baseRewardHMC
	if base <= 0 {
		base = 0.01
	}
	sec := poolBlockSec()
	if sec <= 0 {
		sec = defaultPoolBlockSec
	}
	return base * (86400.0 / sec)
}

func (m *workManager) resetPoolPayDayIfNeeded(now int64) {
	day := utcDayID(now)
	if m.poolPayDayID != day {
		m.poolPayDayID = day
		m.poolPayAccruedDay = 0
	}
}

// applyPoolDailyBudget caps a submit payout so rolling UTC-day accrual stays ≤ budget.
// Caller holds m.mu. Budget 0 disables the cap.
func (m *workManager) applyPoolDailyBudget(payout float64, now int64) float64 {
	if m == nil || payout <= 0 {
		return payout
	}
	budget := m.poolDailyBudgetHMC()
	if budget <= 0 {
		return payout
	}
	m.resetPoolPayDayIfNeeded(now)
	remain := budget - m.poolPayAccruedDay
	if remain <= 0 {
		m.poolBudgetClipped++
		return 0
	}
	if payout > remain {
		payout = remain
		m.poolBudgetClipped++
	}
	m.poolPayAccruedDay += payout
	return payout
}

func (m *workManager) poolBudgetStats() map[string]any {
	if m == nil {
		return map[string]any{"enabled": false}
	}
	budget := m.poolDailyBudgetHMC()
	remain := budget - m.poolPayAccruedDay
	if remain < 0 {
		remain = 0
	}
	return map[string]any{
		"enabled":             budget > 0,
		"daily_budget_hmc":    budget,
		"accrued_today_hmc":   m.poolPayAccruedDay,
		"remaining_today_hmc": remain,
		"budget_clip_events":  m.poolBudgetClipped,
		"block_sec":           poolBlockSec(),
		"note":                "accrual capped to chain emission schedule; settle still requires treasury float",
	}
}
