// Command workersettle: settle coordinator PoH accrual to on-chain HMC transfers
// using the SQLite units ledger (replaces scripts/ops/settle_worker_payouts.sh).
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"hackme/internal/workersettle"
)

func main() {
	cfg := workersettle.DefaultConfig()
	if root := findRoot(); root != "" {
		if cfg.PublishScript == "" {
			p := filepath.Join(root, "scripts/ops/publish_settlement_state.sh")
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				cfg.PublishScript = p
			}
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	res, err := workersettle.Run(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[settle-workers] FATAL: %v\n", err)
		os.Exit(1)
	}
	if res.SkippedLock {
		os.Exit(0)
	}
}

func findRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir := wd
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return wd
}
