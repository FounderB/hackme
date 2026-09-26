//go:build !windows

package workerlock

import (
	"errors"
	"syscall"
)

func isLockBusy(err error) bool {
	return errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN)
}
