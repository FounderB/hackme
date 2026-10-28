package chain

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"hackme/internal/block"
	"hackme/internal/store"
)

// Opt-in rehearsal of ASERT activation on a copy of the real chain DB:
//
//	HACKME_ASERT_PROD_SNAPSHOT=/path/to/hackme.db go test -run TestASERTActivationOnProdSnapshot ./internal/chain/
//
// The snapshot is never modified (two private copies are used). Activation is set to tip+1,
// so the anchor is the real last mainnet block; five Blake3 v3 blocks are mined by the real
// AppendPoHBlock path and imported by a follower copy.
func TestASERTActivationOnProdSnapshot(t *testing.T) {
	snap := os.Getenv("HACKME_ASERT_PROD_SNAPSHOT")
	if snap == "" {
		t.Skip("set HACKME_ASERT_PROD_SNAPSHOT to a chain DB snapshot")
	}
	ctx := context.Background()
	dir := t.TempDir()
	leaderPath := filepath.Join(dir, "leader.db")
	followerPath := filepath.Join(dir, "follower.db")
	for _, p := range []string{leaderPath, followerPath} {
		copyFile(t, snap, p)
	}
	leaderDB, err := store.Open(leaderPath)
	if err != nil {
		t.Fatal(err)
	}
	defer leaderDB.Close()
	leader := New(leaderDB)
	tip, tipHash, err := leader.Tip(ctx)
	if err != nil {
		t.Fatal(err)
	}
	withASERTActivation(t, strconv.FormatUint(tip+1, 10))
	anchorTime, err := blockTimestampQ(ctx, leaderDB, tip)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("snapshot tip %d (%s…), anchor time %d", tip, tipHash[:12], anchorTime)

	addr := "HMC-dfc143732a1fc3bc"
	var mined []*block.Block
	for i := 0; i < 5; i++ {
		m, err := leader.PoHTargetMod(ctx)
		if err != nil {
			t.Fatal(err)
		}
		h, prevHash, _ := leader.Tip(ctx)
		nonce, work := searchBlake3(t, h+1, prevHash, addr, m)
		b, err := leader.AppendPoHBlock(ctx, addr, nonce, work, BaseRewardForBlockIndex(h+1), m, "")
		if err != nil {
			t.Fatalf("append %d at M=%d: %v", h+1, m, err)
		}
		parentTime, _ := blockTimestampQ(ctx, leaderDB, h)
		want := ASERTTargetMod(PoHASERTAnchorMod, tip, anchorTime, h, parentTime)
		if got := payloadMod(t, b); got != want {
			t.Fatalf("block %d M=%d, pure ASERT %d", b.Index, got, want)
		}
		if b.Timestamp < parentTime {
			t.Fatalf("block %d timestamp %d before parent %d", b.Index, b.Timestamp, parentTime)
		}
		t.Logf("block %d mined at M=%d (parent %ds after anchor)", b.Index, payloadMod(t, b), parentTime-anchorTime)
		mined = append(mined, b)
	}
	if got := payloadMod(t, mined[0]); got != ClampPoHTargetMod(PoHASERTAnchorMod) {
		t.Fatalf("activation block M=%d want anchor %d", got, ClampPoHTargetMod(PoHASERTAnchorMod))
	}

	followerDB, err := store.Open(followerPath)
	if err != nil {
		t.Fatal(err)
	}
	defer followerDB.Close()
	follower := New(followerDB)
	for _, b := range mined {
		if err := follower.ImportPoHBlock(ctx, cloneBlock(t, b)); err != nil {
			t.Fatalf("follower import %d: %v", b.Index, err)
		}
	}
	lm, err := leader.PoHTargetMod(ctx)
	if err != nil {
		t.Fatal(err)
	}
	fm, err := follower.PoHTargetMod(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if lm != fm {
		t.Fatalf("leader next M %d != follower next M %d", lm, fm)
	}
	if err := leader.checkEconomicInvariants(ctx, leaderDB); err != nil {
		t.Fatalf("economic invariants after ASERT blocks: %v", err)
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

// searchBlake3 finds a nonce whose Blake3 v3 work is divisible by m, on all cores.
func searchBlake3(t *testing.T, index uint64, prevHash, addr string, m uint64) (uint64, uint64) {
	t.Helper()
	var found atomic.Bool
	var nonce, work uint64
	var mu sync.Mutex
	var wg sync.WaitGroup
	workers := runtime.NumCPU()
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(start uint64) {
			defer wg.Done()
			for n := start; !found.Load(); n += uint64(workers) {
				if v := PohWorkForIndex(index, n, prevHash, addr); v%m == 0 {
					mu.Lock()
					if !found.Load() {
						nonce, work = n, v
						found.Store(true)
					}
					mu.Unlock()
					return
				}
			}
		}(uint64(w))
	}
	wg.Wait()
	return nonce, work
}
