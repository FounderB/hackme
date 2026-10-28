//go:build !linux && !windows

package main

import "os/exec"

// Non-Linux Unix (macOS): exact process-name match (-x), never a substring of the full command
// line (-f), so unrelated processes that merely mention a worker name are left alone.
func killPoolWorkerProcs(_ func(pid int) bool) {
	for _, n := range []string{"workerpoh", "workerpoh-cuda", "workerpoh-opencl", "workerpoh-cpu"} {
		_ = exec.Command("pkill", "-x", n).Run()
	}
}

func killWorkerfuzzProcs(_ func(pid int) bool) {
	_ = exec.Command("pkill", "-x", "workerfuzz").Run()
}
