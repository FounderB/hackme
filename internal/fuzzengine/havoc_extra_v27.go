package fuzzengine

// v2.7 havoc helpers — deterministic, replay-safe.

func nestBraces(buf []byte, mix uint64, maxLen int) []byte {
	opens := []byte{'{', '[', '(', '<'}
	closes := []byte{'}', ']', ')', '>'}
	k := int(mix % 4)
	n := 1 + int((mix>>8)%4)
	prefix := make([]byte, 0, n)
	suffix := make([]byte, 0, n)
	for i := 0; i < n; i++ {
		prefix = append(prefix, opens[k])
		suffix = append(suffix, closes[k])
	}
	out := append(append(prefix, buf...), suffix...)
	if len(out) > maxLen {
		return out[:maxLen]
	}
	return out
}

func utf16LEExpand(buf []byte, mix uint64, maxLen int) []byte {
	if len(buf) == 0 || len(buf)*2 > maxLen {
		return buf
	}
	n := 1 + int(mix%uint64(min(32, len(buf))))
	if n > len(buf) {
		n = len(buf)
	}
	start := int((mix >> 8) % uint64(len(buf)-n+1))
	out := make([]byte, 0, len(buf)+n)
	out = append(out, buf[:start]...)
	for i := 0; i < n; i++ {
		out = append(out, buf[start+i], 0x00)
	}
	out = append(out, buf[start+n:]...)
	if len(out) > maxLen {
		return out[:maxLen]
	}
	return out
}

func bitReverseByte(buf []byte, mix uint64) []byte {
	if len(buf) == 0 {
		return buf
	}
	out := append([]byte(nil), buf...)
	idx := int(mix % uint64(len(out)))
	b := out[idx]
	var r byte
	for i := 0; i < 8; i++ {
		r = (r << 1) | (b & 1)
		b >>= 1
	}
	out[idx] = r
	return out
}

func setInterestingMagnitude(buf []byte, mix uint64) []byte {
	if len(buf) == 0 {
		return buf
	}
	out := append([]byte(nil), buf...)
	idx := int(mix % uint64(len(out)))
	switch mix % 5 {
	case 0:
		out[idx] = 0
	case 1:
		out[idx] = 1
	case 2:
		out[idx] = 0xff
	case 3:
		out[idx] = 0x7f
	default:
		out[idx] = 0x80
	}
	return out
}

func protobufWireSmash(buf []byte, mix uint64) []byte {
	if len(buf) < 2 {
		return buf
	}
	out := append([]byte(nil), buf...)
	// field_number << 3 | wire_type
	tag := byte(((mix >> 8) & 0x1f << 3) | (mix & 0x7))
	idx := int(mix % uint64(len(out)))
	out[idx] = tag
	if idx+1 < len(out) {
		out[idx+1] = byte(mix >> 16)
	}
	return out
}

func injectFloatBits(buf []byte, mix uint64) []byte {
	if len(buf) < 4 {
		return buf
	}
	out := append([]byte(nil), buf...)
	idx := int(mix % uint64(len(out)-3))
	// IEEE754 float32 NaN / Inf patterns
	patterns := []uint32{0x7fc00000, 0x7f800000, 0xff800000, 0x00000001, 0x3f800000}
	v := patterns[int(mix%uint64(len(patterns)))]
	writeU32LE(out, idx, v)
	return out
}

func deltaAdjacent(buf []byte, mix uint64) []byte {
	if len(buf) < 2 {
		return buf
	}
	out := append([]byte(nil), buf...)
	start := int(mix % uint64(len(out)-1))
	n := 1 + int((mix>>8)%8)
	for j := 0; j < n && start+j+1 < len(out); j++ {
		out[start+j+1] = out[start+j] + byte(mix>>(8*(j%4)))
	}
	return out
}

func wrapLengthFrame(buf []byte, mix uint64, maxLen int) []byte {
	if len(buf)+4 > maxLen {
		return buf
	}
	out := make([]byte, 0, len(buf)+4)
	claimed := uint32(len(buf))
	if mix&1 == 1 {
		claimed += uint32(mix & 0xff)
	}
	var hdr [4]byte
	if mix&2 == 0 {
		writeU32LE(hdr[:], 0, claimed)
	} else {
		writeU32BE(hdr[:], 0, claimed)
	}
	out = append(out, hdr[:]...)
	out = append(out, buf...)
	return out
}

func shuffleWindow8(buf []byte, mix uint64) []byte {
	if len(buf) < 8 {
		return buf
	}
	out := append([]byte(nil), buf...)
	start := int(mix % uint64(len(out)-7))
	// Fisher-Yates deterministic on 8-byte window
	for i := 7; i > 0; i-- {
		j := int((mix >> uint(4*i)) % uint64(i+1))
		out[start+i], out[start+j] = out[start+j], out[start+i]
	}
	return out
}

func corpusMaskMerge(buf []byte, corpus [][]byte, mix uint64, maxLen int) []byte {
	if len(corpus) == 0 {
		return buf
	}
	other := corpus[int(mix%uint64(len(corpus)))]
	if len(other) == 0 {
		return buf
	}
	n := min(len(buf), len(other), 64)
	out := append([]byte(nil), buf...)
	for i := 0; i < n; i++ {
		if other[i]&1 == 1 {
			out[i] ^= other[i]
		}
	}
	_ = maxLen
	return out
}

func arithEveryNth(buf []byte, mix uint64) []byte {
	if len(buf) == 0 {
		return buf
	}
	out := append([]byte(nil), buf...)
	step := 2 + int(mix%4)
	delta := byte(mix >> 8)
	for i := int(mix % uint64(step)); i < len(out); i += step {
		out[i] += delta
	}
	return out
}
