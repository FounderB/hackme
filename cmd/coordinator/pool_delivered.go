package main

// Delivered pool hashrate = verified share work (shares × share_mod expected hashes) over a
// sliding window. Device-reported/wall GH/s (sum of LastHashrateGHS) measures how fast rigs hash
// while busy, not how much work reaches the pool: GPU rigs with short leases idled ~50–85% on
// RTT, so M (sized from device GH/s) paid ~1/3–1/5 of emission and listings showed 2–3× the
// real pool. Shares are coordinator-verified, so this cannot be inflated by client reports.

const (
	deliveredGHWindowSec    int64 = 600
	deliveredGHMinSpanSec   int64 = 120
	deliveredSampleEverySec int64 = 10
)

type shareWorkSample struct {
	unix int64
	work float64 // cumulative expected hashes at unix
}

// noteShareWorkLocked records accepted share work (caller holds m.mu).
func (m *workManager) noteShareWorkLocked(nShares int, shareMod uint64, now int64) {
	if m == nil || nShares <= 0 || shareMod == 0 {
		return
	}
	m.shareWorkTotal += float64(nShares) * float64(shareMod)
	m.sampleShareWorkLocked(now)
}

func (m *workManager) sampleShareWorkLocked(now int64) {
	n := len(m.shareWorkSamples)
	if n == 0 || now-m.shareWorkSamples[n-1].unix >= deliveredSampleEverySec {
		m.shareWorkSamples = append(m.shareWorkSamples, shareWorkSample{unix: now, work: m.shareWorkTotal})
	}
	// Keep one anchor at/before the window start so the span always covers the window.
	cut := 0
	for cut+1 < len(m.shareWorkSamples) && m.shareWorkSamples[cut+1].unix <= now-deliveredGHWindowSec {
		cut++
	}
	if cut > 0 {
		m.shareWorkSamples = append(m.shareWorkSamples[:0], m.shareWorkSamples[cut:]...)
	}
}

// deliveredPoolGHLocked returns verified-share GH/s over the window; ok=false until
// deliveredGHMinSpanSec of history exists (fresh restart falls back to device GH/s).
func (m *workManager) deliveredPoolGHLocked(now int64) (gh float64, ok bool) {
	if m == nil || len(m.shareWorkSamples) == 0 {
		return 0, false
	}
	anchor := m.shareWorkSamples[0]
	for _, s := range m.shareWorkSamples {
		if s.unix >= now-deliveredGHWindowSec {
			break
		}
		anchor = s
	}
	span := now - anchor.unix
	if span < deliveredGHMinSpanSec {
		return 0, false
	}
	// An anchor older than the window (quiet pool) still gives an honest rate over its true span.
	return (m.shareWorkTotal - anchor.work) / float64(span) / 1e9, true
}

func (m *workManager) deliveredPoolGH(now int64) (float64, bool) {
	if m == nil {
		return 0, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.deliveredPoolGHLocked(now)
}
