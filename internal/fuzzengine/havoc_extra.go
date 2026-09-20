package fuzzengine

// Extra havoc helpers for fuzz_engine_v2.6 (deterministic, replay-safe).

func reverseWindow(buf []byte, mix uint64) []byte {
	if len(buf) < 2 {
		return buf
	}
	out := append([]byte(nil), buf...)
	start := int(mix % uint64(len(out)-1))
	n := 2 + int((mix>>8)%uint64(min(16, len(out)-start)))
	if start+n > len(out) {
		n = len(out) - start
	}
	for i, j := start, start+n-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func swapEndian64(buf []byte, mix uint64) []byte {
	if len(buf) < 8 {
		return buf
	}
	out := append([]byte(nil), buf...)
	idx := int(mix % uint64(len(out)-7))
	for i := 0; i < 4; i++ {
		out[idx+i], out[idx+7-i] = out[idx+7-i], out[idx+i]
	}
	return out
}

func sieveReplaceByte(buf []byte, mix uint64) []byte {
	if len(buf) == 0 {
		return buf
	}
	out := append([]byte(nil), buf...)
	from := byte(mix)
	to := byte(mix >> 8)
	if from == to {
		to ^= 0xff
	}
	n := 0
	for i := range out {
		if out[i] == from {
			out[i] = to
			n++
			if n >= 8 {
				break
			}
		}
	}
	return out
}

func insertFootgunToken(buf []byte, idx int, mix uint64, maxLen int) []byte {
	tokens := [][]byte{
		[]byte("%n%s%x"),
		[]byte("../"),
		[]byte("..\\"),
		[]byte("${jndi:"),
		[]byte("{{7*7}}"),
		[]byte("\r\n\r\n"),
		[]byte("Content-Length: 0\r\n"),
		[]byte("\\u0000"),
		[]byte("\\x00"),
		[]byte("NaN"),
		[]byte("Infinity"),
		[]byte("-Infinity"),
		[]byte("&lt;&amp;&quot;"),
		[]byte("<!ENTITY xxe SYSTEM"),
		[]byte("\x1f\x8b\x08"), // gzip magic
		[]byte("\x78\x9c"),     // zlib
		[]byte("PK\x03\x04"),   // zip local header
		[]byte("%PDF-1.4"),
		[]byte("\xff\xfe"), // UTF-16 LE BOM
		[]byte("\xfe\xff"), // UTF-16 BE BOM
		[]byte("' OR '1'='1"),
		[]byte("\"></script>"),
		[]byte(";;;;"),
		[]byte("0x7fffffff"),
		[]byte("18446744073709551615"),
	}
	tok := tokens[int(mix%uint64(len(tokens)))]
	return insertToken(buf, idx, tok, maxLen)
}

func caseFlipASCII(buf []byte, mix uint64) []byte {
	if len(buf) == 0 {
		return buf
	}
	out := append([]byte(nil), buf...)
	start := int(mix % uint64(len(out)))
	n := 1 + int((mix>>8)%8)
	for j := 0; j < n && start+j < len(out); j++ {
		c := out[start+j]
		if c >= 'a' && c <= 'z' {
			out[start+j] = c - 32
		} else if c >= 'A' && c <= 'Z' {
			out[start+j] = c + 32
		}
	}
	return out
}

func repeatTokenBurst(buf []byte, mix uint64, dict []byte, maxLen int) []byte {
	tok := dictTokenAt(dict, mix)
	if len(tok) == 0 {
		tok = []byte{byte(mix), byte(mix >> 8)}
	}
	n := 2 + int(mix%6)
	out := append([]byte(nil), buf...)
	idx := int((mix >> 16) % uint64(len(out)+1))
	for i := 0; i < n; i++ {
		out = insertToken(out, idx, tok, maxLen)
		if len(out) >= maxLen {
			break
		}
	}
	return out
}

func adjacentBitflip(buf []byte, mix uint64) []byte {
	if len(buf) < 2 {
		return buf
	}
	out := append([]byte(nil), buf...)
	idx := int(mix % uint64(len(out)-1))
	bit := byte(1 << (mix % 8))
	out[idx] ^= bit
	out[idx+1] ^= bit
	return out
}

func chunkLengthMismatch(buf []byte, mix uint64) []byte {
	if len(buf) < 4 {
		return buf
	}
	out := append([]byte(nil), buf...)
	// Claimed length far from actual — classic TLV / HTTP chunk footgun.
	claimed := uint32(len(out)) + uint32(mix&0xff) + 64
	if mix&1 == 0 {
		writeU32LE(out, 0, claimed)
	} else {
		writeU32BE(out, 0, claimed)
	}
	return out
}

func spliceCorpusSlice(buf []byte, corpus [][]byte, mix uint64, maxLen int) []byte {
	if len(corpus) == 0 {
		return buf
	}
	other := corpus[int(mix%uint64(len(corpus)))]
	if len(other) == 0 {
		return buf
	}
	start := int((mix >> 8) % uint64(len(other)))
	n := 1 + int((mix>>16)%uint64(min(32, len(other)-start)))
	if start+n > len(other) {
		n = len(other) - start
	}
	idx := int((mix >> 24) % uint64(len(buf)+1))
	return insertToken(buf, idx, other[start:start+n], maxLen)
}
