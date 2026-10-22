package chain

import (
	"encoding/binary"
	"encoding/hex"
	"strings"

	"github.com/zeebo/blake3"
)

// PoH v3 work proof (Option A):
//
//	digest = Blake3( prev_hash_hex_ascii_lower || miner_address_utf8 || nonce_u64_le )
//	work   = uint64(little-endian first 8 bytes of digest)
//	accept iff work % M == 0
//
// Endianness is little-endian for both the nonce encoding and the work uint64
// extracted from the digest. This is locked by TestPohWorkBlake3Endianness.

// PohWorkBlake3 returns the v3 PoH work value for the given tip hash, miner address, and nonce.
func PohWorkBlake3(prevHashHex, minerAddress string, nonce uint64) uint64 {
	msg := pohBlake3Message(prevHashHex, minerAddress, nonce)
	sum := blake3.Sum256(msg)
	return binary.LittleEndian.Uint64(sum[0:8])
}

// PohWorkBlake3Digest returns the full 32-byte Blake3 digest (for tests / debugging).
func PohWorkBlake3Digest(prevHashHex, minerAddress string, nonce uint64) [32]byte {
	return blake3.Sum256(pohBlake3Message(prevHashHex, minerAddress, nonce))
}

func pohBlake3Message(prevHashHex, minerAddress string, nonce uint64) []byte {
	prev := strings.ToLower(strings.TrimSpace(prevHashHex))
	addr := strings.TrimSpace(minerAddress)
	out := make([]byte, 0, len(prev)+len(addr)+8)
	out = append(out, prev...)
	out = append(out, addr...)
	var nb [8]byte
	binary.LittleEndian.PutUint64(nb[:], nonce)
	out = append(out, nb[:]...)
	return out
}

// NormalizePoHPrevHash lowercases and trims a tip/prev hash hex string.
// Invalid hex is still passed through lowercased (validation happens elsewhere).
func NormalizePoHPrevHash(prevHashHex string) string {
	s := strings.ToLower(strings.TrimSpace(prevHashHex))
	if s == "" {
		return s
	}
	// Reject non-hex early for callers that need a hard fail — optional helper.
	if _, err := hex.DecodeString(s); err != nil {
		return s
	}
	return s
}
