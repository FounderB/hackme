// Package hotlog provides non-blocking stdout/stderr writers for the mining hot path.
//
// A blocked disk/AV write on the real fd must never stall Search/submit. Lines are
// queued to a background drain; when the queue is full, new lines are dropped.
//
// IMPORTANT: we do NOT redirect os.Stdout/os.Stderr through an os.Pipe. A full
// pipe buffer would block fmt.Print* again and reintroduce #1 freezes.
package hotlog

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const defaultDepth = 2048

type writer struct {
	dst     io.Writer
	ch      chan []byte
	dropped atomic.Uint64
}

func newWriter(dst io.Writer, depth int) *writer {
	if depth < 16 {
		depth = 16
	}
	w := &writer{dst: dst, ch: make(chan []byte, depth)}
	go w.loop()
	return w
}

func (w *writer) loop() {
	for b := range w.ch {
		writeBestEffort(w.dst, b)
	}
}

func writeBestEffort(dst io.Writer, b []byte) {
	deadline := time.Now().Add(50 * time.Millisecond)
	for len(b) > 0 {
		n, err := dst.Write(b)
		if n > 0 {
			b = b[n:]
		}
		if err == nil {
			continue
		}
		if isAgain(err) {
			if time.Now().After(deadline) {
				return // drop remainder; never block the drain for long
			}
			time.Sleep(time.Millisecond)
			continue
		}
		// EPIPE / closed pipe / permanent errors: drop.
		return
	}
}

func isAgain(err error) bool {
	return errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK)
}

func (w *writer) enqueue(b []byte) {
	if len(b) == 0 {
		return
	}
	cp := append([]byte(nil), b...)
	select {
	case w.ch <- cp:
	default:
		w.dropped.Add(1)
	}
}

var (
	startOnce sync.Once
	outW      *writer
	errW      *writer

	verboseMinGap atomic.Int64 // nanoseconds
	lastVerbose   atomic.Int64 // unix nano
)

func tryNonblock(f *os.File) {
	if f == nil {
		return
	}
	_ = syscall.SetNonblock(int(f.Fd()), true)
}

func configureGap() {
	gapMS := 500
	if v := strings.TrimSpace(os.Getenv("HACKME_HOTLOG_VERBOSE_MS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			gapMS = n
		}
	}
	verboseMinGap.Store(int64(gapMS) * int64(time.Millisecond))
}

// Install starts the background drains. Safe to call multiple times; first call wins.
// Call once at process start (before the mining loop).
func Install() {
	startOnce.Do(func() {
		tryNonblock(os.Stdout)
		tryNonblock(os.Stderr)
		outW = newWriter(os.Stdout, defaultDepth)
		errW = newWriter(os.Stderr, defaultDepth)
		configureGap()
	})
}

func ensure() {
	Install()
}

// Dropped returns how many lines were discarded because the queue was full.
func Dropped() uint64 {
	ensure()
	return outW.dropped.Load() + errW.dropped.Load()
}

// Stdoutf formats and enqueues a line to stdout without blocking the caller.
func Stdoutf(format string, args ...any) {
	ensure()
	msg := fmt.Sprintf(format, args...)
	if !strings.HasSuffix(msg, "\n") {
		msg += "\n"
	}
	outW.enqueue([]byte(msg))
}

// Stderrf formats and enqueues a line to stderr without blocking the caller.
func Stderrf(format string, args ...any) {
	ensure()
	msg := fmt.Sprintf(format, args...)
	if !strings.HasSuffix(msg, "\n") {
		msg += "\n"
	}
	errW.enqueue([]byte(msg))
}

// VerboseStderrf is Stderrf with optional rate limiting for per-kernel spam.
// Returns false if the line was skipped by the rate limiter (not an error).
func VerboseStderrf(format string, args ...any) bool {
	ensure()
	gap := verboseMinGap.Load()
	if gap > 0 {
		now := time.Now().UnixNano()
		prev := lastVerbose.Load()
		if now-prev < gap {
			return false
		}
		lastVerbose.Store(now)
	}
	Stderrf(format, args...)
	return true
}
