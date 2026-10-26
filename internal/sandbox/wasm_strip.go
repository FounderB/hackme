package sandbox

import "errors"

// stripWasmCustomSections returns a copy of wasm with all custom sections (id=0)
// removed. Dig/Hunt check gates never need name/producers metadata, and wazero's
// decodeFunctionNames allocates by declared LEB count (OOM). Stripping is the
// compatibility path: wat2wasm often emits a name section we simply drop.
func stripWasmCustomSections(wasm []byte) ([]byte, error) {
	if len(wasm) < 8 {
		return wasm, nil
	}
	if !wasmMagicAndVersionOK(wasm) {
		return nil, errors.New("sandbox: invalid wasm magic/version")
	}
	out := make([]byte, 0, len(wasm))
	out = append(out, wasm[:8]...)
	pos := 8
	kept := false
	for pos < len(wasm) {
		start := pos
		sectionID := wasm[pos]
		pos++
		size, err := readU32LEB(wasm, &pos)
		if err != nil {
			return nil, err
		}
		if pos+int(size) > len(wasm) {
			return nil, errors.New("sandbox: malformed wasm section size")
		}
		end := pos + int(size)
		if sectionID != 0 {
			out = append(out, wasm[start:end]...)
			kept = true
		}
		pos = end
	}
	if !kept && len(out) == 8 {
		// Header-only after strip is useless; keep original so callers get a
		// clear compile error rather than a silent empty module.
		return wasm, nil
	}
	return out, nil
}
