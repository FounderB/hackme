//go:build windows

package workerlock

import (
	"errors"

	"golang.org/x/sys/windows"
)

func isLockBusy(err error) bool {
	return errors.Is(err, windows.ERROR_LOCK_VIOLATION) ||
		errors.Is(err, windows.ERROR_IO_PENDING)
}
