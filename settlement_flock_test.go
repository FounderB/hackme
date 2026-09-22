package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestWithSettlementStateLockEmptyPath(t *testing.T) {
	err := withSettlementStateLock("  ", func() error { return nil })
	if err == nil {
		t.Fatal("expected error on empty path")
	}
}

func TestAtomicWriteFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "worker_settlement_state.json")
	payload := []byte(`{"workers":{"w1":{"settled_hmc":1.25}}}`)
	if err := atomicWriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("got=%q", got)
	}
	// Second write must replace cleanly (Windows rename path).
	payload2 := []byte(`{"workers":{"w1":{"settled_hmc":9.5}}}`)
	if err := atomicWriteFile(path, payload2, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload2) {
		t.Fatalf("got=%q", got)
	}
}

func TestWithSettlementStateLockSerializes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	var holding atomic.Bool
	var overlap atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := withSettlementStateLock(path, func() error {
				if holding.Swap(true) {
					overlap.Add(1)
				}
				time.Sleep(20 * time.Millisecond)
				holding.Store(false)
				return nil
			})
			if err != nil {
				t.Errorf("lock: %v", err)
			}
		}()
	}
	wg.Wait()
	if overlap.Load() != 0 {
		t.Fatalf("overlapping critical sections: %d", overlap.Load())
	}
}

func TestPersistWorkerSettlementStateConcurrentValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "worker_settlement_state.json")
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			st := workerSettlementState{Workers: map[string]workerSettlementStateEntry{
				"w1": {SettledHMC: float64(n)},
			}}
			persistWorkerSettlementState(path, st)
		}(i)
	}
	wg.Wait()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var st workerSettlementState
	if err := json.Unmarshal(raw, &st); err != nil {
		t.Fatalf("torn/invalid JSON after concurrent persist: %v\n%s", err, raw)
	}
	if st.Workers == nil || st.Workers["w1"].SettledHMC < 0 {
		t.Fatalf("unexpected state: %+v", st)
	}
}
