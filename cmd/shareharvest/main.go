// shareharvest finds pseudo-share nonces for coordinator SHARE_STRICT submits.
//
//	shareharvest -base N -batch N -mod N -fv 3 -prev HEX -addr HMC-... [-max 64] [-scan 4194304]
//
// Prints JSON: {"share_nonces":[...]}
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"hackme/internal/chain"
)

func main() {
	base := flag.Uint64("base", 0, "lease base_nonce")
	batch := flag.Uint64("batch", 0, "lease batch_size")
	mod := flag.Uint64("mod", 0, "share_mod from claim")
	fv := flag.Int("fv", 3, "poh_formula_version")
	prev := flag.String("prev", "", "prev_hash from claim")
	addr := flag.String("addr", "", "miner_address from claim")
	maxShares := flag.Int("max", 64, "max share nonces to return")
	maxScan := flag.Uint64("scan", 256*1024, "max hashes to evaluate (share_scan_max)")
	flag.Parse()

	if *batch == 0 || *mod == 0 || *maxShares <= 0 {
		fmt.Fprintf(os.Stderr, "shareharvest: need -batch -mod\n")
		os.Exit(2)
	}
	if *maxScan == 0 {
		*maxScan = 256 * 1024
	}
	stride := uint64(1)
	if *batch > *maxScan {
		stride = *batch / *maxScan
		if stride < 1 {
			stride = 1
		}
	}
	out := make([]uint64, 0, *maxShares)
	end := *base + *batch
	if end < *base {
		end = ^uint64(0)
	}
	scanned := uint64(0)
	for n := *base; n < end && len(out) < *maxShares && scanned < *maxScan; n += stride {
		scanned++
		var work uint64
		if *fv >= 3 {
			work = chain.PohWorkBlake3(*prev, *addr, n)
		} else {
			work = chain.PohEval(n)
		}
		if work%*mod != 0 {
			continue
		}
		out = append(out, n)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(map[string]any{"share_nonces": out, "scanned": scanned, "stride": stride})
}
