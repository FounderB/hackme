//go:build windows

package hunt

import "os/exec"

func setProcessGroup(cmd *exec.Cmd) {
	// Windows: no process-group Setpgid; Job Objects are out of scope for Hunt soft path.
}

func killProcessGroup(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}

func setRSSLimitMB(pid, mb int) error {
	// Windows: no RLIMIT_AS; soft RSS is enforced via wall clock + process Kill.
	return nil
}
