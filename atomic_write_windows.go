//go:build windows

package main

import (
	"fmt"
	"syscall"

	"golang.org/x/sys/windows"
)

func replaceFileAtomic(tmpPath, finalPath string) error {
	from, err := syscall.UTF16PtrFromString(tmpPath)
	if err != nil {
		return err
	}
	to, err := syscall.UTF16PtrFromString(finalPath)
	if err != nil {
		return err
	}
	// Replace without delete-first hole (crash between Remove+Rename lost the file).
	const flags = windows.MOVEFILE_REPLACE_EXISTING | windows.MOVEFILE_WRITE_THROUGH
	if err := windows.MoveFileEx(from, to, flags); err != nil {
		return fmt.Errorf("MoveFileEx replace: %w", err)
	}
	return nil
}
