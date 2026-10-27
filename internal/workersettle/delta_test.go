package workersettle

import (
	"os"
	"testing"
)

func TestComputeAccrualDeltaNormal(t *testing.T) {
	d := ComputeAccrualDelta(1_500_00000, 1_000_00000, 0)
	if d.Rebase || d.NeedAnchor || d.ClearAnchor {
		t.Fatalf("unexpected flags: %+v", d)
	}
	if d.DeltaUnits != 500_00000 || d.BaselineUnits != 1_000_00000 {
		t.Fatalf("delta=%+v", d)
	}
}

func TestComputeAccrualDeltaRebase(t *testing.T) {
	// Coordinator reset: accrued 0.01, settled 10.0
	d := ComputeAccrualDelta(1_000_000, 10_000_00000, 0)
	if !d.Rebase || !d.NeedAnchor || d.NewAnchorUnits != 1_000_000 {
		t.Fatalf("rebase: %+v", d)
	}
	if d.DeltaUnits != 0 {
		t.Fatalf("no growth yet: %+v", d)
	}
	// After growth to 0.05 with anchor 0.01
	d2 := ComputeAccrualDelta(5_000_000, 10_000_00000, 1_000_000)
	if !d2.Rebase || d2.NeedAnchor || d2.DeltaUnits != 4_000_000 {
		t.Fatalf("session growth: %+v", d2)
	}
}

func TestComputeAccrualDeltaCatchUpClearsAnchor(t *testing.T) {
	d := ComputeAccrualDelta(12_000_00000, 10_000_00000, 1_000_000)
	if d.Rebase || !d.ClearAnchor || d.DeltaUnits != 2_000_00000 {
		t.Fatalf("catch-up: %+v", d)
	}
}

func TestShouldSettleAndCapChunk(t *testing.T) {
	if !ShouldSettle(10_000, 1_000, false, 1) {
		t.Fatal("min met")
	}
	if ShouldSettle(100, 1_000, false, 1) {
		t.Fatal("below min")
	}
	if !ShouldSettle(100, 1_000, true, 50) {
		t.Fatal("force min")
	}
	if CapChunk(1000, 500, 100, 0) != 400 {
		t.Fatal("avail")
	}
	if CapChunk(1000, 5000, 100, 200) != 200 {
		t.Fatal("chunk")
	}
	if CapChunk(50, 5000, 100, 200) != 50 {
		t.Fatal("delta")
	}
	if CapChunk(1000, 50, 100, 200) != 0 {
		t.Fatal("reserve")
	}
}

func TestApplySettleUnitsLedger(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir + "/s.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := t.Context()
	// Seed migrate so export works later
	jsonPath := dir + "/state.json"
	if err := osWriteJSON(jsonPath); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MigrateFromJSON(ctx, jsonPath); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetPendingUnits(ctx, "w1", "HMC-to", 250_00000, 7); err != nil {
		t.Fatal(err)
	}
	if err := s.AttachTxHash(ctx, "w1", "abcd"); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplySettleUnits(ctx, "w1", "HMC-to", "abcd", 250_00000, 0, true); err != nil {
		t.Fatal(err)
	}
	w, err := s.GetWorker(ctx, "w1")
	if err != nil {
		t.Fatal(err)
	}
	if w.SettledUnits != 250_00000 || w.SettledHMC != 0.25 || w.PendingJSON != "" {
		t.Fatalf("row=%+v", w)
	}
}

func osWriteJSON(path string) error {
	return os.WriteFile(path, []byte(`{"workers":{},"meta":{"last_force_unix":0}}`), 0o600)
}
