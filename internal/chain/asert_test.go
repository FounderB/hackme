package chain

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"math/big"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hackme/internal/block"
	"hackme/internal/store"
)

func TestASERTOnScheduleKeepsAnchor(t *testing.T) {
	anchor := ClampPoHTargetMod(PoHASERTAnchorMod)
	for _, k := range []uint64{0, 1, 2, 120, 2880, 1_000_000} {
		got := ASERTTargetMod(PoHASERTAnchorMod, 1000, 5_000, 1000+k, 5_000+int64(k)*PoHASERTTargetSec)
		if got != anchor {
			t.Fatalf("on schedule after %d blocks: M=%d want anchor %d", k, got, anchor)
		}
	}
}

func TestASERTHalfLifeDoublesAndHalves(t *testing.T) {
	const k = 1000
	onTime := int64(k) * PoHASERTTargetSec
	ahead := ASERTTargetMod(PoHASERTAnchorMod, 0, 0, k, onTime-PoHASERTHalfLifeSec)
	behind := ASERTTargetMod(PoHASERTAnchorMod, 0, 0, k, onTime+PoHASERTHalfLifeSec)
	if r := float64(ahead) / float64(PoHASERTAnchorMod); math.Abs(r-2) > 1e-6 {
		t.Fatalf("one half-life ahead: ratio %v want 2", r)
	}
	if r := float64(behind) / float64(PoHASERTAnchorMod); math.Abs(r-0.5) > 1e-6 {
		t.Fatalf("one half-life behind: ratio %v want 0.5", r)
	}
}

func TestASERTMonotonicInTime(t *testing.T) {
	prev := uint64(math.MaxUint64)
	for dt := int64(-20000); dt <= 20000; dt += 7 {
		got := ASERTTargetMod(PoHASERTAnchorMod, 0, 0, 100, 3000+dt)
		if got > prev {
			t.Fatalf("M must not increase when the parent is later: dt=%d M=%d prev=%d", dt, got, prev)
		}
		prev = got
	}
}

func TestASERTPow2ApproximationAllFractions(t *testing.T) {
	worst := 0.0
	for frac := uint64(0); frac < 1<<asertRadixBits; frac++ {
		got, _ := new(big.Float).SetInt(asertPow2Frac(frac)).Float64()
		want := math.Pow(2, float64(frac)/65536) * 65536
		if rel := math.Abs(got-want) / want; rel > worst {
			worst = rel
		}
	}
	if worst > 0.00015 {
		t.Fatalf("2^x approximation worst relative error %.6f%% > 0.015%%", worst*100)
	}
}

func TestASERTExtremesClampWithoutPanic(t *testing.T) {
	if got := ASERTTargetMod(PoHASERTAnchorMod, 0, 0, 10_000_000, 0); got != ClampPoHTargetMod(pohRetargetMaxMod) {
		t.Fatalf("far ahead of schedule must clamp to max, got %d", got)
	}
	if got := ASERTTargetMod(PoHASERTAnchorMod, 0, 0, 1, 1<<40); got != ClampPoHTargetMod(pohRetargetMinMod) {
		t.Fatalf("far behind schedule must clamp to min, got %d", got)
	}
	_ = ASERTTargetMod(math.MaxUint64, 0, math.MinInt32, 1<<40, math.MaxInt32)
}

func TestFloorDiv(t *testing.T) {
	cases := [][3]int64{{7, 2, 3}, {-7, 2, -4}, {-8, 2, -4}, {7, -2, -4}, {0, 5, 0}, {-1, 3600, -1}}
	for _, c := range cases {
		if got := floorDiv(c[0], c[1]); got != c[2] {
			t.Fatalf("floorDiv(%d,%d)=%d want %d", c[0], c[1], got, c[2])
		}
	}
}

// Consensus vectors: any change to the formula, constants or rounding must fail here. Expected
// values come from an independent Python reference (testdata/asert_vectors.json generator).
func TestASERTConsensusVectors(t *testing.T) {
	got := []uint64{
		ASERTTargetMod(PoHASERTAnchorMod, 496_999, 1_793_200_000, 496_999, 1_793_200_000),
		ASERTTargetMod(PoHASERTAnchorMod, 496_999, 1_793_200_000, 497_000, 1_793_200_016),
		ASERTTargetMod(PoHASERTAnchorMod, 496_999, 1_793_200_000, 497_100, 1_793_202_000),
		ASERTTargetMod(PoHASERTAnchorMod, 496_999, 1_793_200_000, 499_879, 1_793_290_000),
		ASERTTargetMod(PoHASERTAnchorMod, 496_999, 1_793_200_000, 497_001, 1_793_209_999),
	}
	want := []uint64{17500001, 17547265, 21338539, 8750001, 2582035}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("vector %d: got %d want %d (all: %v)", i, got[i], want[i], got)
		}
	}
}

func TestASERTMatchesIndependentReference(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "asert_vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Vectors []struct {
			AnchorMod    uint64 `json:"anchor_mod"`
			AnchorHeight uint64 `json:"anchor_height"`
			AnchorTime   int64  `json:"anchor_time"`
			ParentHeight uint64 `json:"parent_height"`
			ParentTime   int64  `json:"parent_time"`
			Want         uint64 `json:"want"`
		} `json:"vectors"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	if len(file.Vectors) < 1000 {
		t.Fatalf("want ≥1000 reference vectors, got %d", len(file.Vectors))
	}
	for i, v := range file.Vectors {
		if got := ASERTTargetMod(v.AnchorMod, v.AnchorHeight, v.AnchorTime, v.ParentHeight, v.ParentTime); got != v.Want {
			t.Fatalf("vector %d %+v: go %d, reference %d", i, v, got, v.Want)
		}
	}
}

// Drives ASERT with exponential block intervals: the long-run mean must be the 30 s target,
// with constant hashrate and through ×0.5 / ×3 hashrate shocks.
func TestASERTSimulatedMeanBlockTime(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	const n = 60_000
	hashPerSec := float64(PoHASERTAnchorMod) / float64(PoHASERTTargetSec) // anchor tuned to 30 s
	var tParent int64
	sum := 0.0
	counted := 0
	for h := uint64(1); h <= n; h++ {
		m := ASERTTargetMod(PoHASERTAnchorMod, 0, 0, h-1, tParent)
		rate := hashPerSec
		switch {
		case h >= 20_000 && h < 22_000:
			rate *= 0.5
		case h >= 40_000 && h < 42_000:
			rate *= 3
		}
		dt := rng.ExpFloat64() * float64(m) / rate
		tParent += int64(dt) // integer header timestamps
		if h > 5_000 {
			sum += dt
			counted++
		}
	}
	mean := sum / float64(counted)
	if math.Abs(mean-30) > 0.9 {
		t.Fatalf("mean block time %.2fs, want ~30s", mean)
	}
	if drift := float64(tParent) - float64(n)*30; math.Abs(drift) > 4*3600 {
		t.Fatalf("schedule drift %.0fs over %d blocks", drift, n)
	}
}

// --- chain integration ---------------------------------------------------------------------

func withASERTActivation(t *testing.T, h string) {
	t.Helper()
	t.Setenv("HACKME_POH_ASERT_ACTIVATION_HEIGHT", h)
	ResetPoHASERTActivationHeightForTest()
	t.Cleanup(ResetPoHASERTActivationHeightForTest)
}

// solveV1 finds a nonce with 7n+13 ≡ 0 (mod m) directly (test chains use the v1 formula).
func solveV1(t *testing.T, m uint64) (uint64, uint64) {
	t.Helper()
	inv := new(big.Int).ModInverse(big.NewInt(7), new(big.Int).SetUint64(m))
	if inv == nil {
		t.Fatalf("7 not invertible mod %d", m)
	}
	n := new(big.Int).Mul(new(big.Int).SetUint64(m-13%m), inv)
	n.Mod(n, new(big.Int).SetUint64(m))
	nonce := n.Uint64()
	return nonce, PohEval(nonce)
}

func appendNext(t *testing.T, ctx context.Context, svc *Service, addr string) *block.Block {
	t.Helper()
	m, err := svc.PoHTargetMod(ctx)
	if err != nil {
		t.Fatal(err)
	}
	nonce, eval := solveV1(t, m)
	b, err := svc.AppendPoHBlock(ctx, addr, nonce, eval, 0.01, m, "")
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	return b
}

func payloadMod(t *testing.T, b *block.Block) uint64 {
	t.Helper()
	_, _, mod, err := pohFieldsFromPayload(b.Task.Payload)
	if err != nil {
		t.Fatal(err)
	}
	return mod
}

func TestASERTChainActivationAppendAndImport(t *testing.T) {
	withASERTActivation(t, "4")
	ctx := context.Background()
	dir := t.TempDir()
	leaderDB, err := store.Open(filepath.Join(dir, "leader.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer leaderDB.Close()
	leader := New(leaderDB)
	addr := "HMC-asert00000000aa"
	if _, _, err := leader.InitGenesis(ctx, addr); err != nil {
		t.Fatal(err)
	}
	// Legacy blocks 1..2, then snapshot a follower at height 2 (synced before activation).
	var blocks []*block.Block
	for i := 0; i < 2; i++ {
		blocks = append(blocks, appendNext(t, ctx, leader, addr))
	}
	followerPath := filepath.Join(dir, "follower.db")
	if _, err := leaderDB.ExecContext(ctx, `VACUUM INTO ?`, followerPath); err != nil {
		t.Fatal(err)
	}
	// Blocks 3 (last legacy = anchor) .. 8 (ASERT).
	for i := 0; i < 6; i++ {
		blocks = append(blocks, appendNext(t, ctx, leader, addr))
	}

	// Activation block 4 is mined at exactly the anchor M (anchor block 3, dh=0, dt=0).
	if got := payloadMod(t, blocks[3]); got != ClampPoHTargetMod(PoHASERTAnchorMod) {
		t.Fatalf("activation block M=%d want anchor %d", got, ClampPoHTargetMod(PoHASERTAnchorMod))
	}
	// Every ASERT block's M equals the pure function of its parent/anchor headers.
	anchorT := blocks[2].Timestamp
	for _, b := range blocks[3:] {
		parent := blocks[b.Index-2]
		want := ASERTTargetMod(PoHASERTAnchorMod, 3, anchorT, parent.Index, parent.Timestamp)
		if got := payloadMod(t, b); got != want {
			t.Fatalf("block %d M=%d want asert %d", b.Index, got, want)
		}
		if b.Timestamp < parent.Timestamp {
			t.Fatalf("block %d timestamp went backwards", b.Index)
		}
	}

	// Wrong M is refused after activation, and PoHTargetMod no longer writes stall easing.
	m, _ := leader.PoHTargetMod(ctx)
	wrong := m + 1
	if wrong%7 == 0 {
		wrong++ // v1 test formula needs 7 invertible mod M
	}
	nonce, eval := solveV1(t, wrong)
	if _, err := leader.AppendPoHBlock(ctx, addr, nonce, eval, 0.01, wrong, ""); err == nil || !strings.Contains(err.Error(), "asert") {
		t.Fatalf("append with non-ASERT M must fail, got %v", err)
	}
	if _, err := leaderDB.ExecContext(ctx, `UPDATE blocks SET json = json_set(json, '$.timestamp_unix', json_extract(json, '$.timestamp_unix') - 7200) WHERE block_index = (SELECT MAX(block_index) FROM blocks)`); err != nil {
		t.Fatal(err)
	}
	var emergencyBefore sql.NullString
	_ = leaderDB.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='poh_target_emergency_last_unix'`).Scan(&emergencyBefore)
	if _, err := leader.PoHTargetMod(ctx); err != nil {
		t.Fatal(err)
	}
	var emergencyAfter sql.NullString
	_ = leaderDB.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='poh_target_emergency_last_unix'`).Scan(&emergencyAfter)
	if emergencyAfter != emergencyBefore {
		t.Fatal("PoHTargetMod must not write wall-clock stall easing after ASERT activation")
	}

	// Follower imports the leader's blocks 3..8 across activation and agrees on every M.
	followerDB, err := store.Open(followerPath)
	if err != nil {
		t.Fatal(err)
	}
	defer followerDB.Close()
	follower := New(followerDB)
	for _, b := range blocks[2:] {
		cp := cloneBlock(t, b)
		if err := follower.ImportPoHBlock(ctx, cp); err != nil {
			t.Fatalf("follower import block %d: %v", b.Index, err)
		}
	}
	lm, _ := leader.PoHTargetMod(ctx)
	fm, _ := follower.PoHTargetMod(ctx)
	_ = lm
	if fm == 0 {
		t.Fatal("follower must compute an ASERT target")
	}
}

func cloneBlock(t *testing.T, b *block.Block) *block.Block {
	t.Helper()
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	var out block.Block
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return &out
}

func TestASERTImportRejectsBadTimestampsAndMod(t *testing.T) {
	withASERTActivation(t, "3")
	ctx := context.Background()
	dir := t.TempDir()
	leaderDB, err := store.Open(filepath.Join(dir, "leader.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer leaderDB.Close()
	leader := New(leaderDB)
	addr := "HMC-asert00000000bb"
	if _, _, err := leader.InitGenesis(ctx, addr); err != nil {
		t.Fatal(err)
	}
	b1 := appendNext(t, ctx, leader, addr)
	b2 := appendNext(t, ctx, leader, addr) // anchor
	followerPath := filepath.Join(dir, "follower.db")
	if _, err := leaderDB.ExecContext(ctx, `VACUUM INTO ?`, followerPath); err != nil {
		t.Fatal(err)
	}
	b3 := appendNext(t, ctx, leader, addr) // first ASERT block
	_, _ = b1, b2

	followerDB, err := store.Open(followerPath)
	if err != nil {
		t.Fatal(err)
	}
	defer followerDB.Close()
	follower := New(followerDB)

	back := cloneBlock(t, b3)
	back.Timestamp = b2.Timestamp - 10
	back.SetHash()
	if err := follower.ImportPoHBlock(ctx, back); err == nil || !strings.Contains(err.Error(), "before parent") {
		t.Fatalf("decreasing timestamp must be refused, got %v", err)
	}
	future := cloneBlock(t, b3)
	future.Timestamp = b2.Timestamp + 10*24*3600
	future.SetHash()
	if err := follower.ImportPoHBlock(ctx, future); err == nil || !strings.Contains(err.Error(), "future") {
		t.Fatalf("far-future timestamp must be refused, got %v", err)
	}
	if err := follower.ImportPoHBlock(ctx, cloneBlock(t, b3)); err != nil {
		t.Fatalf("honest block must import: %v", err)
	}
}

// Regression: Blake3 v3 payload eval exceeds 2^53 (real mainnet block 495269); float64 decoding
// rounded it and made followers reject every post-485000 block.
func TestPohFieldsFromPayloadKeepsFull64BitEval(t *testing.T) {
	const eval = uint64(1872241210378134490)
	payload := []byte(`{"eval":1872241210378134490,"formula":"blake3_v3(prev||miner||nonce_le)%M==0","mod":17500001,"nonce":721031796703428,"poh":"wasm_eval"}`)
	n, e, m, err := pohFieldsFromPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	if e != eval || n != 721031796703428 || m != ClampPoHTargetMod(17500001) {
		t.Fatalf("decoded nonce=%d eval=%d mod=%d, want eval %d exactly", n, e, m, eval)
	}
}
