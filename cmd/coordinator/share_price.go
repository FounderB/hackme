package main

import (
	"database/sql"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"hackme/internal/chain"
)

// Windowed EMA of share price (HMC units per verified share).
// Instantaneous price = base_reward * share_mod / target_mod (same as rewardPerShareAt).
// Pay uses EMA when HACKME_COORDINATOR_SHARE_PRICE_EMA=1 (default on with SHARE_STRICT).

const (
	defaultSharePriceEMAAlpha = 0.25
	sharePriceJournalMinGapSec  = int64(5)
)

type sharePriceState struct {
	emaUnits           uint64
	lastInstantUnits   uint64
	samples            uint64
	sharesPaid         uint64
	lastJournalUnix    int64
	windowTargetMod    uint64
	windowShareMod     uint64
}

func envSharePriceEMA() bool {
	v := strings.TrimSpace(os.Getenv("HACKME_COORDINATOR_SHARE_PRICE_EMA"))
	if v == "" {
		// Default ON with share-PPS; set =0 to pay raw instant price only.
		return true
	}
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}

func envSharePriceEMAAlpha() float64 {
	v := strings.TrimSpace(os.Getenv("HACKME_COORDINATOR_SHARE_PRICE_EMA_ALPHA"))
	if v == "" {
		return defaultSharePriceEMAAlpha
	}
	x, err := strconv.ParseFloat(v, 64)
	if err != nil || x <= 0 || x > 1 {
		return defaultSharePriceEMAAlpha
	}
	return x
}

func rewardPerShareUnits(baseRewardHMC float64, shareMod, targetMod uint64) uint64 {
	return chain.HMCToUnits(rewardPerShareAt(baseRewardHMC, shareMod, targetMod))
}

// observeSharePrice updates EMA and optionally journals a sample into coordinator SQLite.
func (m *workManager) observeSharePrice(instantUnits, nShares, targetMod, shareMod uint64) uint64 {
	if m == nil || instantUnits == 0 {
		return instantUnits
	}
	if m.sharePrice == nil {
		m.sharePrice = &sharePriceState{}
	}
	st := m.sharePrice
	st.lastInstantUnits = instantUnits
	st.windowTargetMod = targetMod
	st.windowShareMod = shareMod
	st.sharesPaid += nShares
	st.samples++
	if st.emaUnits == 0 {
		st.emaUnits = instantUnits
	} else {
		alpha := envSharePriceEMAAlpha()
		// EMA in float space then back to units (avoid uint truncation bias).
		prev := float64(st.emaUnits)
		inst := float64(instantUnits)
		st.emaUnits = uint64(alpha*inst + (1-alpha)*prev + 0.5)
		if st.emaUnits == 0 {
			st.emaUnits = instantUnits
		}
	}
	pay := instantUnits
	if envSharePriceEMA() && st.emaUnits > 0 {
		pay = st.emaUnits
	}
	m.maybeJournalSharePrice(nShares, instantUnits, pay, targetMod, shareMod)
	return pay
}

func (m *workManager) maybeJournalSharePrice(nShares, instant, ema, targetMod, shareMod uint64) {
	if m == nil || m.dedupDB == nil {
		return
	}
	now := time.Now().Unix()
	if m.sharePrice != nil && now-m.sharePrice.lastJournalUnix < sharePriceJournalMinGapSec {
		return
	}
	if m.sharePrice != nil {
		m.sharePrice.lastJournalUnix = now
	}
	ensureSharePriceJournal(m.dedupDB)
	_, err := m.dedupDB.Exec(`
INSERT INTO share_price_journal (created_at, share_count, instant_units, ema_units, target_mod, share_mod)
VALUES (?, ?, ?, ?, ?, ?)`, now, nShares, instant, ema, targetMod, shareMod)
	if err != nil {
		log.Printf("share_price_journal: %v", err)
	}
}

const sharePriceJournalTable = `
CREATE TABLE IF NOT EXISTS share_price_journal (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at INTEGER NOT NULL,
  share_count INTEGER NOT NULL DEFAULT 0,
  instant_units INTEGER NOT NULL DEFAULT 0,
  ema_units INTEGER NOT NULL DEFAULT 0,
  target_mod INTEGER NOT NULL DEFAULT 0,
  share_mod INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_share_price_journal_ts ON share_price_journal(created_at);
`

func ensureSharePriceJournal(db *sql.DB) {
	if db == nil {
		return
	}
	_, _ = db.Exec(sharePriceJournalTable)
}
