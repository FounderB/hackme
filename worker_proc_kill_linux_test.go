//go:build linux

package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func startProc(t *testing.T, name string, args ...string) *exec.Cmd {
	t.Helper()
	c := exec.Command(name, args...)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Process.Kill(); _, _ = c.Process.Wait() })
	return c
}

func exited(c *exec.Cmd, within time.Duration) bool {
	done := make(chan struct{})
	go func() { _, _ = c.Process.Wait(); close(done) }()
	select {
	case <-done:
		return true
	case <-time.After(within):
		return false
	}
}

// The old `pkill -f "workerpoh "` / `"workerfuzz "` killed anything mentioning the words (it
// killed `go test` itself). Exact matching kills only real worker binaries and helper scripts.
func TestPoolWorkerKillIsExact(t *testing.T) {
	dir := t.TempDir()
	// Real worker binary: an executable whose file name is workerpoh.
	fake := filepath.Join(dir, "workerpoh")
	src, err := os.Open("/bin/sleep")
	if err != nil {
		t.Skip("no /bin/sleep")
	}
	dst, err := os.OpenFile(fake, os.O_CREATE|os.O_WRONLY, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(dst, src)
	_ = src.Close()
	_ = dst.Close()
	script := filepath.Join(dir, "worker_loop.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	bystander := startProc(t, "sh", "-c", "sleep 30", "x", "workerpoh ", "hackme/cmd/workerfuzz ", "worker_loop.sh.bak")
	worker := startProc(t, fake, "30")
	loop := startProc(t, "sh", script)
	time.Sleep(200 * time.Millisecond)

	allowed := map[int]bool{bystander.Process.Pid: true, worker.Process.Pid: true, loop.Process.Pid: true}
	killPoolWorkerProcs(func(pid int) bool { return allowed[pid] })
	killWorkerfuzzProcs(func(pid int) bool { return allowed[pid] })

	if !exited(worker, 3*time.Second) {
		t.Fatal("real workerpoh binary was not stopped")
	}
	if !exited(loop, 3*time.Second) {
		t.Fatal("worker_loop.sh was not stopped")
	}
	if exited(bystander, 500*time.Millisecond) {
		t.Fatal("unrelated process mentioning worker names was killed")
	}
	if err := bystander.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("bystander gone: %v", err)
	}
}
