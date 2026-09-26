package fuzzengine

import (
	"encoding/binary"
	"strings"
)

// DeepHavocV28 reports whether campaign config opts into the v2.8 deep stack.
// Gated so in-flight campaigns without the flag keep byte-identical replay.
func DeepHavocV28(cfg map[string]any) bool {
	if cfg == nil {
		return false
	}
	if v, ok := cfg["havoc_deep_v28"]; ok {
		return truthyCFG(v)
	}
	p := strings.TrimSpace(strings.ToLower(toString(cfg["hunt_mutator_profile"])))
	switch p {
	case "v28", "deep", "deep_v28", "havoc_deep":
		return true
	}
	return false
}

// EnableDeepHavocV28 sets the opt-in flag for new Hunt campaigns (pool + local).
func EnableDeepHavocV28(cfg map[string]any) {
	if cfg == nil {
		return
	}
	if _, ok := cfg["havoc_deep_v28"]; !ok {
		cfg["havoc_deep_v28"] = true
	}
}

func truthyCFG(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.TrimSpace(strings.ToLower(t))
		return s == "1" || s == "true" || s == "yes" || s == "on"
	case int:
		return t != 0
	case int64:
		return t != 0
	case float64:
		return t != 0
	default:
		return false
	}
}

// applyDeepHavocV28 stacks extra AFL+/structure-aware ops after the v2.7 core.
// Deterministic from stage+salt; never replaces the core path.
func applyDeepHavocV28(base []byte, stage MutationStage, salt uint64, maxLen int, dict []byte, corpus [][]byte) []byte {
	if maxLen <= 0 {
		maxLen = DefaultMaxInputBytesStd
	}
	if len(base) == 0 {
		return []byte{byte(salt)}
	}
	out := append([]byte(nil), base...)
	s := int(stage)
	rounds := 4 + int((salt^uint64(s*41))%7)
	if s >= StageHavocBase {
		rounds += 2 + (s-StageHavocBase)/3
	}
	if rounds > 24 {
		rounds = 24
	}
	for i := 0; i < rounds; i++ {
		mix := splitmix64(salt ^ 0xD28D28D28D28D28D ^ uint64(s) ^ uint64(i)*0x9e3779b97f4a7c15)
		switch mix % 16 {
		case 0:
			out = walkingNBitFlip(out, mix)
		case 1:
			out = interesting64Smash(out, mix)
		case 2:
			out = multiWindowArith(out, mix)
		case 3:
			out = utf8OverlongInsert(out, mix, maxLen)
		case 4:
			out = jsonEscapeUnescape(out, mix, maxLen)
		case 5:
			out = denseDictOverlay(out, mix, dict, maxLen)
		case 6:
			out = entropySpikeInsert(out, mix, maxLen)
		case 7:
			out = lengthCascadeCorrupt(out, mix)
		case 8:
			out = splice3wayRoundRobin(out, corpus, mix, maxLen)
		case 9:
			out = structureSmash(out, mix^0xD28DEAD0, maxLen)
		case 10:
			out = nestBraces(out, mix, maxLen)
			out = injectFloatBits(out, mix>>8)
		case 11:
			out = chunkLengthMismatch(out, mix)
			out = wrapLengthFrame(out, mix>>4, maxLen)
		case 12:
			out = bitReverseByte(out, mix)
			out = adjacentBitflip(out, mix>>8)
		case 13:
			out = insertFootgunToken(out, int(mix%uint64(len(out)+1)), mix, maxLen)
			out = caseFlipASCII(out, mix>>16)
		case 14:
			out = duplicateSkew(out, mix, maxLen)
		default:
			out = structureSmash(out, mix, maxLen)
			out = walkingNBitFlip(out, mix>>8)
		}
		if len(out) == 0 {
			out = []byte{byte(mix)}
		}
		if len(out) > maxLen {
			out = out[:maxLen]
		}
	}
	return out
}

// walkingNBitFlip flips a contiguous run of 2/4/8/16/32 bits (AFL walking flavours).
func walkingNBitFlip(buf []byte, mix uint64) []byte {
	if len(buf) == 0 {
		return buf
	}
	out := append([]byte(nil), buf...)
	widths := []int{2, 4, 8, 16, 32}
	w := widths[int(mix%uint64(len(widths)))]
	span := len(out)*8 - w + 1
	if span < 1 {
		span = 1
		w = 1
	}
	bitOff := int(mix>>8) % span
	for b := 0; b < w; b++ {
		abs := bitOff + b
		idx := abs / 8
		bit := abs % 8
		if idx < len(out) {
			out[idx] ^= byte(1 << bit)
		}
	}
	return out
}

func interesting64Smash(buf []byte, mix uint64) []byte {
	if len(buf) < 8 {
		return interesting64SmashSmall(buf, mix)
	}
	out := append([]byte(nil), buf...)
	idx := int(mix % uint64(len(out)-7))
	vals := []uint64{
		0, 1, 0x7f, 0x80, 0xff, 0xffff, 0x7fffffff, 0x80000000,
		0xffffffff, 0x7fffffffffffffff, 0x8000000000000000, 0xffffffffffffffff,
		0x3ff0000000000000, // 1.0 float64
		0x7ff0000000000000, // +Inf
		0xfff0000000000000, // -Inf
		0x7ff8000000000000, // NaN
	}
	v := vals[int(mix>>8)%len(vals)]
	if mix&1 == 0 {
		binary.LittleEndian.PutUint64(out[idx:], v)
	} else {
		binary.BigEndian.PutUint64(out[idx:], v)
	}
	return out
}

func interesting64SmashSmall(buf []byte, mix uint64) []byte {
	if len(buf) == 0 {
		return buf
	}
	out := append([]byte(nil), buf...)
	vals := Interesting8()
	out[int(mix%uint64(len(out)))] = vals[int(mix>>8)%len(vals)]
	return out
}

func multiWindowArith(buf []byte, mix uint64) []byte {
	if len(buf) == 0 {
		return buf
	}
	out := append([]byte(nil), buf...)
	windows := 1 + int(mix%3)
	for w := 0; w < windows; w++ {
		m := mix >> uint(8*w)
		idx := int(m % uint64(len(out)))
		delta := int8(1 + int(m>>8)%35)
		if (m>>16)&1 == 1 {
			delta = -delta
		}
		arithAdd8(out, idx, delta)
		if len(out) >= 2 && (m&2) == 2 {
			arithAdd16LE(out, idx%(len(out)-1), int16(delta)*3)
		}
	}
	return out
}

func utf8OverlongInsert(buf []byte, mix uint64, maxLen int) []byte {
	// Overlong encodings of ASCII '/' and NUL — classic parser footguns.
	tokens := [][]byte{
		{0xc0, 0xaf},       // overlong '/'
		{0xe0, 0x80, 0xaf}, // 3-byte overlong '/'
		{0xc0, 0x80},       // overlong NUL
		{0xf0, 0x80, 0x80, 0xaf},
		{0xed, 0xa0, 0x80}, // lone surrogate hi
		{0xed, 0xbf, 0xbf}, // lone surrogate lo
	}
	tok := tokens[int(mix%uint64(len(tokens)))]
	idx := int((mix >> 8) % uint64(len(buf)+1))
	return insertToken(buf, idx, tok, maxLen)
}

func jsonEscapeUnescape(buf []byte, mix uint64, maxLen int) []byte {
	tokens := [][]byte{
		[]byte(`\\u0000`),
		[]byte(`\\x00`),
		[]byte(`\\\"`),
		[]byte(`\\/`),
		[]byte(`\\n\\r\\t`),
		[]byte(`\\uD800\\uDC00`),
		[]byte(`\"\"`),
		[]byte(`,:,`),
	}
	tok := tokens[int(mix%uint64(len(tokens)))]
	idx := int((mix >> 8) % uint64(len(buf)+1))
	return insertToken(buf, idx, tok, maxLen)
}

func denseDictOverlay(buf []byte, mix uint64, dict []byte, maxLen int) []byte {
	out := append([]byte(nil), buf...)
	n := 2 + int(mix%3)
	for i := 0; i < n; i++ {
		tok := dictTokenAt(dict, mix>>uint(8*i))
		if len(tok) == 0 {
			tok = []byte{byte(mix >> uint(8*i)), 0xff}
		}
		idx := int((mix >> uint(4*(i+1))) % uint64(len(out)+1))
		if (mix>>uint(i))&1 == 0 {
			out = insertToken(out, idx, tok, maxLen)
		} else {
			den := len(out)
			if den < 1 {
				den = 1
			}
			out = overwriteWithToken(out, idx%den, tok)
		}
		if len(out) >= maxLen {
			break
		}
	}
	return out
}

func entropySpikeInsert(buf []byte, mix uint64, maxLen int) []byte {
	n := 4 + int(mix%12)
	spike := make([]byte, n)
	x := mix ^ 0xA5A5A5A5A5A5A5A5
	for i := 0; i < n; i++ {
		x = splitmix64(x)
		spike[i] = byte(x)
	}
	idx := int((mix >> 16) % uint64(len(buf)+1))
	return insertToken(buf, idx, spike, maxLen)
}

func lengthCascadeCorrupt(buf []byte, mix uint64) []byte {
	if len(buf) < 4 {
		return buf
	}
	out := append([]byte(nil), buf...)
	// Stamp 2–3 length-ish fields at different offsets (TLV / chunked parsers).
	n := 2 + int(mix%2)
	for i := 0; i < n; i++ {
		off := int((mix >> uint(8*i)) % uint64(len(out)-3))
		claimed := uint32(mix>>(uint(8*i)+3)) ^ uint32(len(out)*i+17)
		if (mix>>uint(i))&1 == 0 {
			writeU32LE(out, off, claimed)
		} else {
			writeU32BE(out, off, claimed)
		}
	}
	return out
}

func splice3wayRoundRobin(buf []byte, corpus [][]byte, mix uint64, maxLen int) []byte {
	if len(corpus) < 2 {
		return buf
	}
	a := corpus[int(mix%uint64(len(corpus)))]
	b := corpus[int((mix>>8)%uint64(len(corpus)))]
	c := buf
	if len(corpus) >= 3 {
		c = corpus[int((mix>>16)%uint64(len(corpus)))]
	}
	// Take thirds round-robin.
	parts := [][]byte{a, b, c}
	out := make([]byte, 0, maxLen)
	for i := 0; i < 3; i++ {
		p := parts[i]
		if len(p) == 0 {
			continue
		}
		start := int((mix >> uint(4*i)) % uint64(len(p)))
		n := 1 + int((mix>>uint(8+4*i))%uint64(min(32, len(p)-start)))
		out = append(out, p[start:start+n]...)
		if len(out) >= maxLen {
			return out[:maxLen]
		}
	}
	if len(out) == 0 {
		return buf
	}
	return out
}

func duplicateSkew(buf []byte, mix uint64, maxLen int) []byte {
	if len(buf) < 2 {
		return buf
	}
	out := append([]byte(nil), buf...)
	start := int(mix % uint64(len(out)))
	n := 1 + int((mix>>8)%uint64(min(24, len(out))))
	if start+n > len(out) {
		n = len(out) - start
	}
	chunk := append([]byte(nil), out[start:start+n]...)
	// Skew: duplicate then XOR first byte of copy.
	if len(chunk) > 0 {
		chunk[0] ^= byte(mix >> 16)
	}
	idx := int((mix >> 24) % uint64(len(out)+1))
	return insertToken(out, idx, chunk, maxLen)
}
