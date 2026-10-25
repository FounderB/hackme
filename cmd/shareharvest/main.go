// shareharvest finds pseudo-share nonces for coordinator SHARE_STRICT submits.
//
//	shareharvest -base N -batch N -mod N -fv 3 -prev HEX -addr HMC-... [-max 4]
//
// Workers interleave nonces; each uses a reusable blake3.Hasher for speed.
// Prints JSON: {"share_nonces":[...],"scanned":N}
package main

import (
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"hackme/internal/chain"

	"github.com/zeebo/blake3"
)

func main() {
	base := flag.Uint64("base", 0, "lease base_nonce")
	batch := flag.Uint64("batch", 0, "lease batch_size")
	mod := flag.Uint64("mod", 0, "share_mod from claim")
	fv := flag.Int("fv", 3, "poh_formula_version")
	prev := flag.String("prev", "", "prev_hash from claim")
	addr := flag.String("addr", "", "miner_address from claim")
	maxShares := flag.Int("max", 4, "max share nonces to return")
	maxScan := flag.Uint64("scan", 0, "max hashes (0 = auto)")
	workers := flag.Int("workers", 0, "parallel hash workers (0 = GOMAXPROCS)")
	flag.Parse()

	if *batch == 0 || *mod == 0 || *maxShares <= 0 {
		fmt.Fprintf(os.Stderr, "shareharvest: need -batch -mod\n")
		os.Exit(2)
	}
	if *workers <= 0 {
		*workers = runtime.GOMAXPROCS(0)
		if *workers < 1 {
			*workers = 1
		}
	}
	if *maxScan == 0 {
		*maxScan = (*mod) * uint64(*maxShares) * 6
		if *maxScan < 1<<22 {
			*maxScan = 1 << 22
		}
		const hardCap = uint64(1) << 33
		if *maxScan > hardCap {
			*maxScan = hardCap
		}
	}

	end := *base + *batch
	if end < *base {
		end = ^uint64(0)
	}

	prevN := strings.ToLower(strings.TrimSpace(*prev))
	addrN := strings.TrimSpace(*addr)
	prefix := append(append([]byte{}, prevN...), addrN...)

	var (
		mu      sync.Mutex
		out     []uint64
		seen    = map[uint64]struct{}{}
		scanned atomic.Uint64
		stop    atomic.Bool
		wg      sync.WaitGroup
	)
	addHit := func(n uint64) {
		mu.Lock()
		defer mu.Unlock()
		if len(out) >= *maxShares {
			stop.Store(true)
			return
		}
		if _, ok := seen[n]; ok {
			return
		}
		seen[n] = struct{}{}
		out = append(out, n)
		if len(out) >= *maxShares {
			stop.Store(true)
		}
	}

	W := uint64(*workers)
	for w := 0; w < *workers; w++ {
		w := w
		wg.Add(1)
		go func() {
			defer wg.Done()
			h := blake3.New()
			var sum [32]byte
			var nb [8]byte
			for n := *base + uint64(w); n < end; n += W {
				if stop.Load() {
					return
				}
				if scanned.Add(1) > *maxScan {
					stop.Store(true)
					return
				}
				var work uint64
				if *fv >= 3 {
					h.Reset()
					_, _ = h.Write(prefix)
					binary.LittleEndian.PutUint64(nb[:], n)
					_, _ = h.Write(nb[:])
					_ = h.Sum(sum[:0])
					work = binary.LittleEndian.Uint64(sum[0:8])
				} else {
					work = chain.PohEval(n)
				}
				if work%*mod == 0 {
					addHit(n)
				}
			}
		}()
	}
	wg.Wait()

	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(map[string]any{
		"share_nonces": out,
		"scanned":      scanned.Load(),
		"workers":      *workers,
	})
}
