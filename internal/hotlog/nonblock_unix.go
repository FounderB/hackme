//go:build unix

package hotlog

import (
	"os"
	"syscall"
)

func tryNonblock(f *os.File) {
	if f == nil {
		return
	}
	_ = syscall.SetNonblock(int(f.Fd()), true)
}
