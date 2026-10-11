package workerfuzzloop

import (
	"encoding/hex"
	"testing"
)

func TestCollectCoverageEngineSeedsMergesSeedByteCorpus(t *testing.T) {
	tlv := []byte{'H', 'W', 'I', 'R', 1, 0, 8, 0, 0x10, 6, 0, 4, 0, 'A', 'B', 'C', 'D'}
	anchor := []byte{0x00}
	cr := ClaimResp{
		InputBytesHex: hex.EncodeToString(anchor),
		CorpusSeeds: []map[string]any{
			{"input_bytes_hex": hex.EncodeToString([]byte{0x01, 0x02})},
		},
		SeedByteCorpus: []any{
			hex.EncodeToString(tlv),
			hex.EncodeToString(anchor), // dup of InputBytesHex — should dedupe
		},
	}
	got := collectCoverageEngineSeeds(cr)
	if len(got) < 3 {
		t.Fatalf("want >=3 unique seeds (anchor+corpus+tlv), got %d", len(got))
	}
	foundTLV := false
	for _, b := range got {
		if len(b) == len(tlv) && b[0] == 'H' && b[8] == 0x10 {
			foundTLV = true
		}
	}
	if !foundTLV {
		t.Fatal("seed_byte_corpus TLV pack seed missing from LF/AFL SeedFiles")
	}
}

func TestCollectCoverageEngineSeedsEmpty(t *testing.T) {
	if got := collectCoverageEngineSeeds(ClaimResp{}); len(got) != 0 {
		t.Fatalf("empty claim should yield 0 seeds, got %d", len(got))
	}
}
