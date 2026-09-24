//go:build windows

package hotlog

import "os"

// On Windows, O_NONBLOCK is not meaningful for the worker log file path the way
// it is on Unix pipes. Hot-path protection is the async drop-on-full queue.
func tryNonblock(f *os.File) {}
