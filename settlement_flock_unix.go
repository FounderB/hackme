//go:build !windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// withSettlementStateLock takes an exclusive flock on path+".flock" (blocking).
// Fail-closed: OpenFile / flock errors return without running fn (no unlocked write).
func withSettlementStateLock(path string, fn func() error) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("settlement lock: empty path")
	}
	lockPath := path + ".flock"
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	lf, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lf.Close()
	if err := syscall.Flock(int(lf.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer func() { _ = syscall.Flock(int(lf.Fd()), syscall.LOCK_UN) }()
	return fn()
}
