//go:build !windows

package workerlock

import (
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

// A stale pid file (owner exited, lock released) whose PID now belongs to an unrelated live
// process must never be signalled: PIDs are reused. Before the fix Reap SIGTERMed it — in a
// multi-package `go test` run that was `go` itself; on a miner desktop it could be any app.
func TestReapNeverKillsUnrelatedProcessFromStalePIDFile(t *testing.T) {
	dir := t.TempDir()
	bystander := exec.Command("sleep", "30")
	if err := bystander.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bystander.Process.Kill(); _, _ = bystander.Process.Wait() }()
	path := LockPath("workerpoh", "worker-stale-01", dir)
	if err := os.WriteFile(path, []byte(strconv.Itoa(bystander.Process.Pid)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Reap("workerpoh", "worker-stale-01", dir)
	if err != nil {
		t.Fatalf("reap: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if res.Killed || !processAlive(bystander.Process.Pid) {
		t.Fatalf("Reap killed unrelated pid %d from a stale pid file (res=%+v)", bystander.Process.Pid, res)
	}
	if _, err := ReapAll(dir); err != nil {
		t.Fatalf("reap all: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if !processAlive(bystander.Process.Pid) {
		t.Fatal("ReapAll killed unrelated pid from a stale pid file")
	}
}
