//go:build unix

package hunt

import (
	"io/fs"
	"syscall"
)

func fileInode(fi fs.FileInfo) (uint64, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || st == nil {
		return 0, false
	}
	return st.Ino, true
}
