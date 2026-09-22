package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hackme/internal/workerlock"
)

func TestRotateWorkerParticipantLog(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "worker_participant.log")
	if err := os.WriteFile(logPath, []byte("old orphan line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rotateWorkerParticipantLog(logPath)
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatalf("expected log renamed away, err=%v", err)
	}
	prev := logPath + ".prev"
	b, err := os.ReadFile(prev)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "old orphan line\n" {
		t.Fatalf("prev=%q", b)
	}
}

func TestReapOrphanPoolWorkersClearsLock(t *testing.T) {
	if os.Getenv("WORKER_ORPHAN_HOLDER") == "1" {
		dir := os.Getenv("WORKER_ORPHAN_DIR")
		g, err := workerlock.Acquire("workerpoh", "worker-poc-01", dir)
		if err != nil {
			os.Exit(1)
		}
		defer g.Release()
		_, _ = os.Stdout.Write([]byte("READY\n"))
		time.Sleep(45 * time.Second)
		return
	}

	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestReapOrphanPoolWorkersClearsLock$", "-test.v")
	cmd.Env = append(os.Environ(), "WORKER_ORPHAN_HOLDER=1", "WORKER_ORPHAN_DIR="+dir)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	readyCh := make(chan bool, 1)
	go func() {
		buf := make([]byte, 256)
		var acc string
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				acc += string(buf[:n])
				if strings.Contains(acc, "READY") {
					readyCh <- true
					return
				}
			}
			if err != nil {
				readyCh <- false
				return
			}
		}
	}()
	select {
	case ok := <-readyCh:
		if !ok {
			t.Fatal("holder child exited before READY")
		}
	case <-time.After(20 * time.Second):
		t.Fatal("holder child did not become READY")
	}
	if !workerlock.Held("workerpoh", "worker-poc-01", dir) {
		t.Fatal("expected orphan lock held")
	}

	t.Setenv("HACKME_WORKER_LOCK_DIR", dir)
	reapOrphanPoolWorkers(dir, "worker-poc-01", dir)
	if workerlock.Held("workerpoh", "worker-poc-01", dir) {
		t.Fatal("lock still held after reapOrphanPoolWorkers")
	}
	g, err := workerlock.Acquire("workerpoh", "worker-poc-01", dir)
	if err != nil {
		t.Fatalf("acquire after orphan reap (report #2 fix): %v", err)
	}
	g.Release()
}

func TestAbsoluteWorkerLockDirStable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HACKME_WORKER_LOCK_DIR", "")
	abs := absoluteWorkerLockDir(dir)
	if !filepath.IsAbs(abs) {
		t.Fatalf("want abs path, got %q", abs)
	}
	if abs != filepath.Clean(abs) {
		t.Fatalf("unclean: %q", abs)
	}
}

func TestReapOrphanPoolWorkersClearsAltRepoLogs(t *testing.T) {
	if os.Getenv("WORKER_ORPHAN_ALT_HOLDER") == "1" {
		dir := os.Getenv("WORKER_ORPHAN_ALT_DIR")
		g, err := workerlock.Acquire("workerpoh", "worker-poc-01", dir)
		if err != nil {
			os.Exit(1)
		}
		defer g.Release()
		_, _ = os.Stdout.Write([]byte("READY\n"))
		time.Sleep(45 * time.Second)
		return
	}

	nodeLogs := t.TempDir()
	repoRoot := t.TempDir()
	altLogs := filepath.Join(repoRoot, "logs")
	if err := os.MkdirAll(altLogs, 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestReapOrphanPoolWorkersClearsAltRepoLogs$", "-test.v")
	cmd.Env = append(os.Environ(), "WORKER_ORPHAN_ALT_HOLDER=1", "WORKER_ORPHAN_ALT_DIR="+altLogs)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	readyCh := make(chan bool, 1)
	go func() {
		buf := make([]byte, 256)
		var acc string
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				acc += string(buf[:n])
				if strings.Contains(acc, "READY") {
					readyCh <- true
					return
				}
			}
			if err != nil {
				readyCh <- false
				return
			}
		}
	}()
	select {
	case ok := <-readyCh:
		if !ok {
			t.Fatal("holder child exited before READY")
		}
	case <-time.After(20 * time.Second):
		t.Fatal("holder child did not become READY")
	}

	if !workerlock.Held("workerpoh", "worker-poc-01", altLogs) {
		t.Fatal("expected orphan lock in repoRoot/logs")
	}
	// Node log dir differs from repoRoot/logs (apt XDG vs /opt) — reap must clear both.
	t.Setenv("HACKME_WORKER_LOCK_DIR", nodeLogs)
	reapOrphanPoolWorkers(nodeLogs, "worker-poc-01", repoRoot)
	if workerlock.Held("workerpoh", "worker-poc-01", altLogs) {
		t.Fatal("alt repo lock still held after dual-dir reap")
	}
}
