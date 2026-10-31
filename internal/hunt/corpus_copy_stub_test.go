//go:build !unix

package hunt

import "io/fs"

func fileInode(fi fs.FileInfo) (uint64, bool) {
	return 0, false
}
