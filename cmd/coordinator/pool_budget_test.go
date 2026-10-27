package main

import (
	"testing"
	"time"
)

func TestApplyPoolDailyBudgetCapsToEmission(t *testing.T) {
	t.Setenv("HACKME_COORDINATOR_POOL_DAILY_BUDGET_HMC", "1.0")
	t.Setenv("HACKME_COORDINATOR_POOL_BLOCK_SEC", "30")
	wm := &workManager{baseRewardHMC: 0.01}
	now := time.Now().Unix()
	got := wm.applyPoolDailyBudget(0.4, now)
	if got != 0.4 {
		t.Fatalf("first credit want 0.4 got %v", got)
	}
	got = wm.applyPoolDailyBudget(0.5, now)
	if got != 0.5 {
		t.Fatalf("second credit want 0.5 got %v", got)
	}
	got = wm.applyPoolDailyBudget(0.2, now)
	if got < 0.099 || got > 0.101 {
		t.Fatalf("third credit want ~0.1 remain, got %v", got)
	}
	got = wm.applyPoolDailyBudget(0.5, now)
	if got != 0 {
		t.Fatalf("over budget want 0 got %v", got)
	}
	st := wm.poolBudgetStats()
	if st["accrued_today_hmc"].(float64) < 0.99 || st["accrued_today_hmc"].(float64) > 1.01 {
		t.Fatalf("accrued=%v", st["accrued_today_hmc"])
	}
}

func TestPoolDailyBudgetDefaultFromEmission(t *testing.T) {
	t.Setenv("HACKME_COORDINATOR_POOL_DAILY_BUDGET_HMC", "")
	t.Setenv("HACKME_COORDINATOR_POOL_BLOCK_SEC", "30")
	wm := &workManager{baseRewardHMC: 0.01}
	got := wm.poolDailyBudgetHMC()
	want := 0.01 * (86400.0 / 30.0) // 28.8
	if got < want-0.01 || got > want+0.01 {
		t.Fatalf("budget=%v want ~%v", got, want)
	}
}

func TestLoadRetargetUsesHashTimesBlockSec(t *testing.T) {
	t.Setenv("HACKME_COORDINATOR_POOL_BLOCK_SEC", "30")
	wm := &workManager{
		poolRetarget:       true,
		targetMod:          poolTargetModMin,
		targetModMin:       poolTargetModMin,
		targetModMax:       poolTargetModMax,
		poolRetargetMinSec: 0,
		baseRewardHMC:      0.01,
		rewardAuto:         true,
	}
	// Ramp catches up over ticks (1.35× when far below load).
	for i := 0; i < 80; i++ {
		wm.lastPoolRetargetUnix = 0
		wm.maybeRetargetPoolLoadLocked(time.Now().Unix()+int64(i), 15.0, 3)
	}
	// 15 GH/s × 1e9 × 30s ≈ 4.5e11 (boost for miner count may raise slightly).
	if wm.targetMod < 200_000_000_000 {
		t.Fatalf("after load retarget M too small: %d (want ~4.5e11 scale)", wm.targetMod)
	}
	if wm.targetMod > poolTargetModMax {
		t.Fatalf("M %d exceeds max", wm.targetMod)
	}
}
