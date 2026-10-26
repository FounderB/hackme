//go:build !windows

package sandbox

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

func setWasmValidateChildAttr(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func setWasmValidateASLimit(pid, mb int) error {
	if pid <= 0 || mb <= 0 {
		return nil
	}
	bytes := uint64(mb) * 1024 * 1024
	lim := unix.Rlimit{Cur: bytes, Max: bytes}
	return unix.Prlimit(pid, unix.RLIMIT_AS, &lim, nil)
}
