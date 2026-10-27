package main

import (
	"testing"

	"hackme/internal/chain"
)

func TestObserveSharePriceEMA(t *testing.T) {
	t.Setenv("HACKME_COORDINATOR_SHARE_PRICE_EMA", "1")
	t.Setenv("HACKME_COORDINATOR_SHARE_PRICE_EMA_ALPHA", "0.5")
	m := &workManager{}
	u1 := uint64(1000)
	pay1 := m.observeSharePrice(u1, 1, 1<<20, 1<<10)
	if pay1 != 1000 || m.sharePrice.emaUnits != 1000 {
		t.Fatalf("seed pay=%d ema=%d", pay1, m.sharePrice.emaUnits)
	}
	pay2 := m.observeSharePrice(2000, 1, 1<<20, 1<<10)
	// alpha=0.5 → 0.5*2000 + 0.5*1000 = 1500
	if pay2 != 1500 {
		t.Fatalf("ema pay=%d want 1500", pay2)
	}
	t.Setenv("HACKME_COORDINATOR_SHARE_PRICE_EMA", "0")
	pay3 := m.observeSharePrice(2000, 1, 1<<20, 1<<10)
	if pay3 != 2000 {
		t.Fatalf("instant-only pay=%d", pay3)
	}
}

func TestRewardPerShareUnitsMatchesFloat(t *testing.T) {
	base := 0.01
	sm, tm := uint64(1024), uint64(1<<20)
	u := rewardPerShareUnits(base, sm, tm)
	want := chain.HMCToUnits(rewardPerShareAt(base, sm, tm))
	if u != want || u == 0 {
		t.Fatalf("units=%d want=%d", u, want)
	}
}
