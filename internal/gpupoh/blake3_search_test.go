package gpupoh

import (
	"context"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/zeebo/blake3"
)

// pohWorkBlake3Mirror mirrors chain.PohWorkBlake3 without importing chain
// (chain → gpupoh would create a test import cycle).
func pohWorkBlake3Mirror(prevHashHex, minerAddress string, nonce uint64) uint64 {
	prev := strings.ToLower(strings.TrimSpace(prevHashHex))
	addr := strings.TrimSpace(minerAddress)
	msg := make([]byte, 0, len(prev)+len(addr)+8)
	msg = append(msg, prev...)
	msg = append(msg, addr...)
	var nb [8]byte
	binary.LittleEndian.PutUint64(nb[:], nonce)
	msg = append(msg, nb[:]...)
	sum := blake3.Sum256(msg)
	return binary.LittleEndian.Uint64(sum[0:8])
}

func TestBlake3SearchMatchesPohWorkEncoding(t *testing.T) {
	prev := "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
	addr := "HMC-testminer"
	const mod = 97
	var hit uint64
	var found bool
	for n := uint64(0); n < 50_000; n++ {
		if pohWorkBlake3Mirror(prev, addr, n)%mod == 0 {
			hit = n
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no solvable nonce in range")
	}
	ok, nonce, err := Blake3Search(context.Background(), 0, hit+1, mod, prev, addr)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || nonce != hit {
		t.Fatalf("Blake3Search got ok=%v nonce=%d want %d", ok, nonce, hit)
	}
	if pohWorkBlake3Mirror(prev, addr, nonce)%mod != 0 {
		t.Fatal("hit failed Blake3 work check")
	}
}

func TestBlake3SearchMessageLayoutLE(t *testing.T) {
	prev := "deadbeef"
	addr := "HMC-x"
	nonce := uint64(0x0102030405060708)
	want := pohWorkBlake3Mirror(prev, addr, nonce)

	msg := append(append([]byte{}, []byte(prev)...), []byte(addr)...)
	var nb [8]byte
	binary.LittleEndian.PutUint64(nb[:], nonce)
	msg = append(msg, nb[:]...)
	sum := blake3.Sum256(msg)
	got := binary.LittleEndian.Uint64(sum[0:8])
	if got != want {
		t.Fatalf("layout mismatch: got %d want %d", got, want)
	}
}

func TestBlake3SearchEmptyModOrCount(t *testing.T) {
	ok, _, err := Blake3Search(context.Background(), 0, 0, 97, "aa", "HMC-a")
	if err != nil || ok {
		t.Fatalf("count=0: ok=%v err=%v", ok, err)
	}
	ok, _, err = Blake3Search(context.Background(), 0, 10, 0, "aa", "HMC-a")
	if err != nil || ok {
		t.Fatalf("mod=0: ok=%v err=%v", ok, err)
	}
}
