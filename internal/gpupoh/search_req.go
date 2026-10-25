package gpupoh

import (
	"context"
	"encoding/binary"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/zeebo/blake3"
)

// Host Blake3 batch cap (GPU builds also use maxBatch from common_limits.go).
const blake3HostMaxBatch = 1 << 26

// SearchRequest describes a PoH nonce-range search (v1 algebraic or v3 Blake3).
type SearchRequest struct {
	Base           uint64
	Count          uint64
	Mod            uint64
	FormulaVersion int    // 1 = 7n+13, 3 = Blake3(prev||miner||nonce_le)
	PrevHash       string // tip hash hex (v3)
	MinerAddress   string // bound miner address (v3)
	// ShareMod > 0: collect PPS share nonces during the same hash pass (GPU kernel or host).
	ShareMod  uint64
	MaxShares int // 0 → DefaultMaxShares
}

// DefaultMaxShares matches coordinator maxSharesPerSubmit.
const DefaultMaxShares = 128

// SearchResult is a PoH search outcome including optional PPS shares.
type SearchResult struct {
	Found  bool
	Nonce  uint64
	Shares []uint64
}

// SearchPoHDefault implements SearchPoH for accelerators that only expose Search (v1).
// FormulaVersion >= 3 uses multithreaded host Blake3 (same encoding as chain.PohWorkBlake3).
func SearchPoHDefault(ctx context.Context, acc Accelerator, req SearchRequest) (bool, uint64, error) {
	if req.FormulaVersion >= 3 {
		return Blake3Search(ctx, req.Base, req.Count, req.Mod, req.PrevHash, req.MinerAddress)
	}
	if acc == nil {
		return false, 0, ErrNoGPU
	}
	return acc.Search(ctx, req.Base, req.Count, req.Mod)
}

// Blake3Search scans [base, base+count) for Blake3 work % mod == 0 (little-endian work uint64).
func Blake3Search(ctx context.Context, base, count, mod uint64, prevHash, minerAddress string) (bool, uint64, error) {
	res, err := Blake3SearchCollect(ctx, SearchRequest{
		Base: base, Count: count, Mod: mod, FormulaVersion: 3,
		PrevHash: prevHash, MinerAddress: minerAddress,
	})
	return res.Found, res.Nonce, err
}

// Blake3SearchCollect hashes each nonce once: block hit + optional PPS shares.
func Blake3SearchCollect(ctx context.Context, req SearchRequest) (SearchResult, error) {
	var out SearchResult
	count := req.Count
	mod := req.Mod
	if count == 0 || mod == 0 {
		return out, nil
	}
	if count > blake3HostMaxBatch {
		count = blake3HostMaxBatch
	}
	shareMod := req.ShareMod
	maxShares := req.MaxShares
	if maxShares <= 0 {
		maxShares = DefaultMaxShares
	}
	prev := strings.ToLower(strings.TrimSpace(req.PrevHash))
	addr := strings.TrimSpace(req.MinerAddress)
	prefix := make([]byte, 0, len(prev)+len(addr))
	prefix = append(prefix, prev...)
	prefix = append(prefix, addr...)

	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}
	if uint64(workers) > count {
		workers = int(count)
	}
	var (
		found atomic.Uint64 // 1 = hit
		best  atomic.Uint64
		wg    sync.WaitGroup
	)
	best.Store(^uint64(0))
	var shareBuf []uint64
	var shareMu sync.Mutex
	if shareMod > 0 && maxShares > 0 {
		shareBuf = make([]uint64, 0, maxShares)
	}
	appendShares := func(local []uint64) {
		if len(local) == 0 {
			return
		}
		shareMu.Lock()
		for _, s := range local {
			if len(shareBuf) >= maxShares {
				break
			}
			shareBuf = append(shareBuf, s)
		}
		shareMu.Unlock()
	}
	chunk := count / uint64(workers)
	if chunk == 0 {
		chunk = 1
	}
	base := req.Base
	for w := 0; w < workers; w++ {
		start := base + uint64(w)*chunk
		n := chunk
		if w == workers-1 {
			n = count - uint64(w)*chunk
		}
		if start < base || n == 0 {
			continue
		}
		wg.Add(1)
		go func(start, n uint64) {
			defer wg.Done()
			msg := make([]byte, len(prefix)+8)
			copy(msg, prefix)
			localShares := make([]uint64, 0, 8)
			for i := uint64(0); i < n; i++ {
				if i&0xffff == 0 {
					select {
					case <-ctx.Done():
						appendShares(localShares)
						return
					default:
					}
					// When collecting shares, do not early-exit on a block hit —
					// remaining nonces still prove GPU/CPU work for PPS.
					if shareMod == 0 && found.Load() == 1 && best.Load() < start+i {
						return
					}
				}
				nonce := start + i
				binary.LittleEndian.PutUint64(msg[len(prefix):], nonce)
				sum := blake3.Sum256(msg)
				work := binary.LittleEndian.Uint64(sum[0:8])
				if shareMod > 0 && work%shareMod == 0 {
					localShares = append(localShares, nonce)
				}
				if work%mod != 0 {
					continue
				}
				for {
					cur := best.Load()
					if nonce >= cur {
						break
					}
					if best.CompareAndSwap(cur, nonce) {
						found.Store(1)
						break
					}
				}
			}
			appendShares(localShares)
		}(start, n)
	}
	wg.Wait()
	if err := ctx.Err(); err != nil && found.Load() == 0 && len(shareBuf) == 0 {
		return out, err
	}
	out.Shares = shareBuf
	if found.Load() == 1 {
		out.Found = true
		out.Nonce = best.Load()
	}
	return out, nil
}
