//go:build !windows

package main

import "os"

// replaceFileAtomic renames tmp over final (POSIX rename replaces atomically).
func replaceFileAtomic(tmpPath, finalPath string) error {
	return os.Rename(tmpPath, finalPath)
}
