//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// withSettlementStateLock takes an exclusive LockFileEx on path+".flock" (blocking).
// Fail-closed: OpenFile / lock errors return without running fn (no unlocked write).
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
	// Blocking exclusive lock — parity with Unix flock(LOCK_EX). Do NOT use
	// LOCKFILE_FAIL_IMMEDIATELY: concurrent dashboard refreshes must serialize.
	if err := windows.LockFileEx(
		windows.Handle(lf.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK,
		0,
		1,
		0,
		&windows.Overlapped{},
	); err != nil {
		return err
	}
	defer func() {
		_ = windows.UnlockFileEx(windows.Handle(lf.Fd()), 0, 1, 0, &windows.Overlapped{})
	}()
	return fn()
}
