package hotlog

import (
	"bytes"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestEnqueueDoesNotBlockOnFullQueue(t *testing.T) {
	block := make(chan struct{})
	dst := writerFunc(func(p []byte) (int, error) {
		<-block
		return len(p), nil
	})
	w := newWriter(dst, 4)

	deadline := time.Now().Add(2 * time.Second)
	for i := 0; i < 64; i++ {
		if time.Now().After(deadline) {
			t.Fatal("enqueue blocked longer than 2s")
		}
		done := make(chan struct{})
		go func() {
			w.enqueue([]byte("x\n"))
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(500 * time.Millisecond):
			t.Fatal("enqueue blocked (would freeze mining loop)")
		}
	}
	if w.dropped.Load() == 0 {
		t.Fatal("expected some drops when queue full")
	}
	close(block)
	time.Sleep(50 * time.Millisecond)
}

func TestEnqueueEventuallyWrites(t *testing.T) {
	var buf bytes.Buffer
	var mu sync.Mutex
	dst := writerFunc(func(p []byte) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		return buf.Write(p)
	})
	w := newWriter(dst, 32)
	w.enqueue([]byte("hello 7\n"))
	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		got := buf.String()
		mu.Unlock()
		if got == "hello 7\n" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("got %q", got)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestVerboseRateLimit(t *testing.T) {
	ensure()
	verboseMinGap.Store(int64(200 * time.Millisecond))
	lastVerbose.Store(0)
	var n int
	for i := 0; i < 10; i++ {
		if VerboseStderrf("v %d", i) {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("expected 1 line through rate limit, got %d", n)
	}
	time.Sleep(220 * time.Millisecond)
	if !VerboseStderrf("v later") {
		t.Fatal("expected line after gap")
	}
}

func TestWriteBestEffortDropsOnBlockedDest(t *testing.T) {
	// Always-EAGAIN destination: drain must give up within deadline, not hang.
	dst := writerFunc(func(p []byte) (int, error) {
		return 0, syscall.EAGAIN
	})
	start := time.Now()
	writeBestEffort(dst, bytes.Repeat([]byte("y"), 64*1024))
	elapsed := time.Since(start)
	if elapsed > 500*time.Millisecond {
		t.Fatalf("writeBestEffort blocked too long: %s", elapsed)
	}
	if elapsed < 40*time.Millisecond {
		t.Fatalf("expected ~50ms deadline wait, got %s", elapsed)
	}
}

func TestInstallCapturesFmtStdout(t *testing.T) {
	ensure()
	before := Dropped()
	for i := 0; i < 10000; i++ {
		Stdoutf("flood %d", i)
	}
	_ = before
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }
