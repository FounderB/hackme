package workerlock

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAcquireExclusive(t *testing.T) {
	dir := t.TempDir()
	g1, err := Acquire("workerpoh", "rig-a", dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g1.Release()
	if g1.Path() != filepath.Join(dir, "workerlock-workerpoh-rig-a.pid") {
		t.Fatalf("path=%s", g1.Path())
	}
	_, err = Acquire("workerpoh", "rig-a", dir)
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("want ErrAlreadyRunning, got %v", err)
	}
	// Different kind or id can run in parallel.
	g2, err := Acquire("workerfuzz", "rig-a", dir)
	if err != nil {
		t.Fatal(err)
	}
	g2.Release()
	g1.Release()
	g3, err := Acquire("workerpoh", "rig-a", dir)
	if err != nil {
		t.Fatal(err)
	}
	g3.Release()
}

func TestHeld(t *testing.T) {
	dir := t.TempDir()
	if Held("workerfuzz", "rig-b", dir) {
		t.Fatal("empty lock should not be held")
	}
	g, err := Acquire("workerfuzz", "rig-b", dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Release()
	if !Held("workerfuzz", "rig-b", dir) {
		t.Fatal("expected held while Acquire live")
	}
	g.Release()
	if Held("workerfuzz", "rig-b", dir) {
		t.Fatal("expected free after Release")
	}
}

func TestReadPID(t *testing.T) {
	dir := t.TempDir()
	g, err := Acquire("workerpoh", "poc-01", dir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Release()
	pid, err := ReadPID("workerpoh", "poc-01", dir)
	if err != nil {
		t.Fatal(err)
	}
	if pid != os.Getpid() {
		t.Fatalf("pid=%d want %d", pid, os.Getpid())
	}
}

func TestParseLockFileName(t *testing.T) {
	kind, wid, ok := parseLockFileName("workerlock-workerpoh-worker-poc-01.pid")
	if !ok || kind != "workerpoh" || wid != "worker-poc-01" {
		t.Fatalf("got kind=%q wid=%q ok=%v", kind, wid, ok)
	}
	kind, wid, ok = parseLockFileName("workerlock-workerfuzz-rig-b.pid")
	if !ok || kind != "workerfuzz" || wid != "rig-b" {
		t.Fatalf("got kind=%q wid=%q ok=%v", kind, wid, ok)
	}
}

func TestReapKillsHolder(t *testing.T) {
	if os.Getenv("WORKERLOCK_HOLDER") == "1" {
		dir := os.Getenv("WORKERLOCK_DIR")
		g, err := Acquire("workerpoh", "orphan-poc", dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "acquire: %v\n", err)
			os.Exit(1)
		}
		defer g.Release()
		fmt.Println("READY")
		time.Sleep(45 * time.Second)
		return
	}

	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestReapKillsHolder$", "-test.v")
	cmd.Env = append(os.Environ(), "WORKERLOCK_HOLDER=1", "WORKERLOCK_DIR="+dir)
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
	case <-time.After(15 * time.Second):
		t.Fatal("holder child did not become READY")
	}
	if !Held("workerpoh", "orphan-poc", dir) {
		t.Fatal("expected lock held by child")
	}
	childPID := cmd.Process.Pid
	res, err := Reap("workerpoh", "orphan-poc", dir)
	if err != nil {
		t.Fatalf("reap: %v", err)
	}
	if !res.Killed {
		t.Fatalf("expected killed, got %+v", res)
	}
	if res.PID != childPID {
		t.Fatalf("reaped pid=%d want child=%d", res.PID, childPID)
	}
	if Held("workerpoh", "orphan-poc", dir) {
		t.Fatal("lock still held after reap")
	}
	// New acquire must succeed (report #2 PoC inverted).
	g, err := Acquire("workerpoh", "orphan-poc", dir)
	if err != nil {
		t.Fatalf("acquire after reap: %v", err)
	}
	g.Release()
}
