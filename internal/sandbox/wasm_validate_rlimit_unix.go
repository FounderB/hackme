//go:build !windows

package sandbox

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
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

// applySelfWasmValidateASLimit sets RLIMIT_AS to current VmSize + extraMB.
// Called as the first action in the validate child after Go's runtime has
// already reserved its VAS — avoids rejecting every module under a fixed
// 384 MiB absolute cap, and removes the parent-prlimit-after-Start race.
func applySelfWasmValidateASLimit(extraMB int) error {
	if extraMB < 64 {
		extraMB = 64
	}
	vmKB, err := readProcSelfVmSizeKB()
	if err != nil {
		// Fall back to absolute ceiling from env/default.
		return setWasmValidateASLimit(os.Getpid(), wasmValidateASMB())
	}
	limit := uint64(vmKB)*1024 + uint64(extraMB)*1024*1024
	// Never exceed the configured absolute ceiling.
	ceil := uint64(wasmValidateASMB()) * 1024 * 1024
	if limit > ceil {
		limit = ceil
	}
	lim := unix.Rlimit{Cur: limit, Max: limit}
	return unix.Setrlimit(unix.RLIMIT_AS, &lim)
}

func readProcSelfVmSizeKB() (int64, error) {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return 0, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "VmSize:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, fmt.Errorf("sandbox: bad VmSize line")
		}
		kb, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil || kb <= 0 {
			return 0, fmt.Errorf("sandbox: parse VmSize: %w", err)
		}
		return kb, nil
	}
	return 0, fmt.Errorf("sandbox: VmSize not found")
}
