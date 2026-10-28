//go:build windows

package main

// Windows stops workers with taskkill /IM (exact image name); see killExternalWorkerFleet.
func killPoolWorkerProcs(_ func(pid int) bool) {}

func killWorkerfuzzProcs(_ func(pid int) bool) {}
