//go:build !windows

package hunt

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

func setProcessGroup(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killProcessGroup(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err == nil {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}
	return cmd.Process.Kill()
}

func setRSSLimitMB(pid, mb int) error {
	if pid <= 0 || mb <= 0 {
		return nil
	}
	bytes := uint64(mb) * 1024 * 1024
	lim := unix.Rlimit{Cur: bytes, Max: bytes}
	// Address-space cap (closest portable RSS stand-in for native harnesses).
	if err := unix.Prlimit(pid, unix.RLIMIT_AS, &lim, nil); err != nil {
		return err
	}
	return nil
}
