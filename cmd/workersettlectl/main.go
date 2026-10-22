// Command workersettlectl: SQLite settlement ledger helpers for settle_worker_payouts.sh.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"hackme/internal/workersettle"
)

func usage() {
	fmt.Fprintf(os.Stderr, `usage:
  workersettlectl migrate <db> <json>
  workersettlectl get <db> <worker_id>
  workersettlectl get-json <db> <worker_id>
  workersettlectl set-pending <db> <worker_id> <addr> <delta_hmc> <nonce>
  workersettlectl attach-tx <db> <worker_id> <tx_hash>
  workersettlectl clear-pending <db> <worker_id>
  workersettlectl apply <db> <worker_id> <addr> <tx_hash> <new_settled> [session_anchor|clear]
  workersettlectl set-sup <db> <worker_id> <settled_sup> [payout_address]
  workersettlectl set-force <db> <unix_ts>
  workersettlectl export-json <db> [out.json]
  workersettlectl pending-count <db>
`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	ctx := context.Background()
	switch cmd {
	case "migrate":
		need(4)
		s := mustOpen(os.Args[2])
		defer s.Close()
		ok, err := s.MigrateFromJSON(ctx, os.Args[3])
		must(err)
		if ok {
			fmt.Println("migrated")
		} else {
			fmt.Println("noop")
		}
	case "get":
		need(4)
		s := mustOpen(os.Args[2])
		defer s.Close()
		w, err := s.GetWorker(ctx, os.Args[3])
		must(err)
		fmt.Printf("settled_hmc=%.12f settled_sup=%.12f session_anchor_hmc=%.12f payout_address=%s last_tx_hash=%s pending=%q\n",
			w.SettledHMC, w.SettledSUP, w.SessionAnchorHMC, w.PayoutAddress, w.LastTxHash, w.PendingJSON)
	case "get-json":
		need(4)
		s := mustOpen(os.Args[2])
		defer s.Close()
		w, err := s.GetWorker(ctx, os.Args[3])
		must(err)
		ent := map[string]any{
			"worker_id":          w.WorkerID,
			"settled_hmc":        w.SettledHMC,
			"settled_sup":        w.SettledSUP,
			"session_anchor_hmc": w.SessionAnchorHMC,
			"payout_address":     w.PayoutAddress,
			"last_tx_hash":       w.LastTxHash,
			"pending_json":       w.PendingJSON,
		}
		b, err := json.Marshal(ent)
		must(err)
		fmt.Println(string(b))
	case "set-pending":
		need(7)
		s := mustOpen(os.Args[2])
		defer s.Close()
		delta, err := strconv.ParseFloat(os.Args[5], 64)
		must(err)
		nonce, err := strconv.ParseInt(os.Args[6], 10, 64)
		must(err)
		id, err := s.SetPending(ctx, os.Args[3], os.Args[4], delta, nonce)
		must(err)
		fmt.Printf("outbox_id=%d\n", id)
	case "attach-tx":
		need(5)
		s := mustOpen(os.Args[2])
		defer s.Close()
		must(s.AttachTxHash(ctx, os.Args[3], os.Args[4]))
		fmt.Println("ok")
	case "clear-pending":
		need(4)
		s := mustOpen(os.Args[2])
		defer s.Close()
		must(s.ClearPending(ctx, os.Args[3]))
		fmt.Println("ok")
	case "apply":
		need(7)
		s := mustOpen(os.Args[2])
		defer s.Close()
		settled, err := strconv.ParseFloat(os.Args[6], 64)
		must(err)
		anchor := 0.0
		clear := true
		if len(os.Args) >= 8 {
			if strings.EqualFold(os.Args[7], "clear") {
				clear = true
			} else {
				anchor, err = strconv.ParseFloat(os.Args[7], 64)
				must(err)
				clear = false
			}
		}
		must(s.ApplySettle(ctx, os.Args[3], os.Args[4], os.Args[5], settled, anchor, clear))
		fmt.Println("ok")
	case "set-sup":
		need(5)
		s := mustOpen(os.Args[2])
		defer s.Close()
		sup, err := strconv.ParseFloat(os.Args[4], 64)
		must(err)
		addr := ""
		if len(os.Args) >= 6 {
			addr = os.Args[5]
		}
		must(s.SetSettledSUP(ctx, os.Args[3], sup, addr))
		fmt.Println("ok")
	case "set-force":
		need(4)
		s := mustOpen(os.Args[2])
		defer s.Close()
		ts, err := strconv.ParseInt(os.Args[3], 10, 64)
		must(err)
		must(s.SetLastForceUnix(ctx, ts))
		fmt.Println("ok")
	case "export-json":
		need(3)
		s := mustOpen(os.Args[2])
		defer s.Close()
		if len(os.Args) >= 4 {
			must(s.ExportJSONToFile(ctx, os.Args[3]))
			fmt.Println(os.Args[3])
		} else {
			b, err := s.ExportJSON(ctx)
			must(err)
			fmt.Println(string(b))
		}
	case "pending-count":
		need(3)
		s := mustOpen(os.Args[2])
		defer s.Close()
		n, err := s.CountPendingOutbox(ctx)
		must(err)
		fmt.Println(n)
	default:
		usage()
		os.Exit(2)
	}
}

func need(n int) {
	if len(os.Args) < n {
		usage()
		os.Exit(2)
	}
}

func mustOpen(path string) *workersettle.Store {
	s, err := workersettle.Open(path)
	must(err)
	return s
}

func must(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "workersettlectl: %v\n", err)
		os.Exit(1)
	}
}
