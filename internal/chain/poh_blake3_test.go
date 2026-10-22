package chain

import (
	"encoding/binary"
	"os"
	"testing"

	"github.com/zeebo/blake3"
)

func TestPohWorkBlake3Endianness(t *testing.T) {
	prev := "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
	addr := "HMC-deadbeefcafebabe"
	nonce := uint64(0x0102030405060708)
	msg := pohBlake3Message(prev, addr, nonce)
	sum := blake3.Sum256(msg)
	want := binary.LittleEndian.Uint64(sum[0:8])
	got := PohWorkBlake3(prev, addr, nonce)
	if got != want {
		t.Fatalf("work LE mismatch: got=%d want=%d", got, want)
	}
	be := binary.BigEndian.Uint64(sum[0:8])
	if be == got && sum[0] != sum[7] {
		t.Fatal("unexpected: LE and BE identical for asymmetric digest")
	}
}

func TestPohWorkBlake3BindsMinerAndPrev(t *testing.T) {
	prev := "d940c235e61c496708bb97e9845319ca81f2991259fd71e9f3240182e6ee1d28"
	a1 := PohWorkBlake3(prev, "HMC-aaa", 42)
	a2 := PohWorkBlake3(prev, "HMC-bbb", 42)
	a3 := PohWorkBlake3("00"+prev[2:], "HMC-aaa", 42)
	if a1 == a2 {
		t.Fatal("different miner_address must change Blake3 work")
	}
	if a1 == a3 {
		t.Fatal("different prev_hash must change Blake3 work")
	}
}

func TestPoHFormulaV3ActivationHeightEnv(t *testing.T) {
	ResetPoHFormulaV3ActivationHeightForTest()
	t.Setenv("HACKME_POH_V3_ACTIVATION_HEIGHT", "12345")
	ResetPoHFormulaV3ActivationHeightForTest()
	if got := PoHFormulaV3ActivationHeight(); got != 12345 {
		t.Fatalf("env override: got %d", got)
	}
	ResetPoHFormulaV3ActivationHeightForTest()
	_ = os.Unsetenv("HACKME_POH_V3_ACTIVATION_HEIGHT")
	ResetPoHFormulaV3ActivationHeightForTest()
	if got := PoHFormulaV3ActivationHeight(); got != defaultPoHFormulaV3ActivationHeight {
		t.Fatalf("default: got %d want %d", got, defaultPoHFormulaV3ActivationHeight)
	}
}

func TestActivePoHTaskKindV3Gates(t *testing.T) {
	ResetPoHFormulaV3ActivationHeightForTest()
	t.Setenv("HACKME_POH_V3_ACTIVATION_HEIGHT", "1000")
	ResetPoHFormulaV3ActivationHeightForTest()
	defer func() {
		_ = os.Unsetenv("HACKME_POH_V3_ACTIVATION_HEIGHT")
		ResetPoHFormulaV3ActivationHeightForTest()
	}()
	if got := ActivePoHTaskKindForIndex(999); got != TaskKindSyntheticPoH {
		t.Fatalf("pre-v3 got %s", got)
	}
	if got := ActivePoHTaskKindForIndex(1000); got != TaskKindSyntheticPoHV3 {
		t.Fatalf("at v3 got %s", got)
	}
	if PoHFormulaVersionForIndex(1000) != 3 {
		t.Fatal("version")
	}
}

func TestValidatePoHSubmissionBlake3(t *testing.T) {
	ResetPoHFormulaV3ActivationHeightForTest()
	t.Setenv("HACKME_POH_V3_ACTIVATION_HEIGHT", "50")
	ResetPoHFormulaV3ActivationHeightForTest()
	defer func() {
		_ = os.Unsetenv("HACKME_POH_V3_ACTIVATION_HEIGHT")
		ResetPoHFormulaV3ActivationHeightForTest()
	}()
	prev := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	addr := "HMC-miner1"
	mod := uint64(1000)
	var nonce, work uint64
	found := false
	for n := uint64(0); n < 500_000; n++ {
		w := PohWorkBlake3(prev, addr, n)
		if w%mod == 0 {
			nonce, work = n, w
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no blake3 hit")
	}
	if err := validatePoHSubmission(50, nonce, work, mod, prev, addr); err != nil {
		t.Fatalf("valid: %v", err)
	}
	// Algebraic 7n+13 shortcut must fail post-activation.
	alg := PohEval(nonce)
	if alg%mod == 0 {
		// pick another nonce where algebraic hits but blake3 may not
		nonce = 0
		for n := uint64(0); n < 100_000; n++ {
			if PohEval(n)%mod == 0 {
				nonce = n
				break
			}
		}
		alg = PohEval(nonce)
	}
	if err := validatePoHSubmission(50, nonce, alg, mod, prev, addr); err == nil {
		t.Fatal("algebraic eval must not validate as blake3 work")
	}
	if err := validatePoHSubmission(50, nonce, work, mod, prev, "HMC-other"); err == nil {
		t.Fatal("wrong miner_address must reject")
	}
	if err := validatePoHSubmission(50, nonce, work, mod, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", addr); err == nil {
		t.Fatal("stale/wrong prev_hash must reject")
	}
}

func TestPohWorkForIndexSwitchesAtActivation(t *testing.T) {
	ResetPoHFormulaV3ActivationHeightForTest()
	t.Setenv("HACKME_POH_V3_ACTIVATION_HEIGHT", "77")
	ResetPoHFormulaV3ActivationHeightForTest()
	defer func() {
		_ = os.Unsetenv("HACKME_POH_V3_ACTIVATION_HEIGHT")
		ResetPoHFormulaV3ActivationHeightForTest()
	}()
	prev := "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	addr := "HMC-x"
	n := uint64(99)
	v1 := PohWorkForIndex(76, n, prev, addr)
	v3 := PohWorkForIndex(77, n, prev, addr)
	if v1 != PohEval(n) {
		t.Fatalf("pre-activation want algebraic, got %d", v1)
	}
	if v3 != PohWorkBlake3(prev, addr, n) {
		t.Fatalf("post-activation want blake3")
	}
	if v1 == v3 {
		t.Fatal("expected algebraic and blake3 to differ")
	}
}

func TestCannotReuseNonceAcrossMinerAddress(t *testing.T) {
	prev := "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	mod := uint64(251)
	var nonce uint64
	var workA uint64
	for n := uint64(0); n < 200_000; n++ {
		w := PohWorkBlake3(prev, "HMC-alice", n)
		if w%mod == 0 {
			nonce, workA = n, w
			break
		}
	}
	workB := PohWorkBlake3(prev, "HMC-bob", nonce)
	if workB == workA && workB%mod == 0 {
		// Extremely unlikely; if it happens, pick another hit.
		t.Skip("birthday collision on tiny mod; retrying not worth flake")
	}
	if workB%mod == 0 && workB == workA {
		t.Fatal("reuse")
	}
	if err := validatePoHSubmission(PoHFormulaV3ActivationHeight(), nonce, workA, mod, prev, "HMC-bob"); err == nil {
		t.Fatal("alice work must not validate for bob address")
	}
}
