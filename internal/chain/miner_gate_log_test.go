package chain

import (
	"errors"
	"testing"
)

func TestLogGateErrRateLimitedCountsAndThrottles(t *testing.T) {
	m := &Miner{}
	err := errors.New("sandbox: HACKME_WASM_VALIDATE_BIN not found")
	for i := 0; i < 1000; i++ {
		m.logGateErrRateLimited(err)
	}
	if got := m.gateErrCount.Load(); got != 1000 {
		t.Fatalf("gateErrCount=%d want 1000", got)
	}
	first := m.gateErrLogNano.Load()
	if first == 0 {
		t.Fatal("first gate error must be journaled")
	}
	m.logGateErrRateLimited(err)
	if m.gateErrLogNano.Load() != first {
		t.Fatal("second journal line inside interval must be suppressed")
	}
}
