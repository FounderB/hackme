package sandbox

import "errors"

// maxWasmTableInitial caps funcref/externref table minimum size before wazero compile.
// Unbounded table min (H44) allocates pointer arrays and can OOM the process.
const maxWasmTableInitial = 256

// maxWasmElementSectionBytes bounds element-section payload size for check-gate modules.
const maxWasmElementSectionBytes = 8192

// Per-section vector caps for check-gate modules (customer order / Dig WASM).
const (
	maxWasmTypeCount     = 64
	maxWasmFuncCount     = 64
	maxWasmExportCount   = 32
	maxWasmCodeCount     = 64
	maxWasmGlobalCount   = 32
	maxWasmDataCount     = 32
	maxWasmMemoryCount   = 4
	maxWasmElementCount  = 16
	maxWasmDeclaredCount = 64
)

// rejectWasmHostileSections blocks start routines, imports, oversized tables/elements,
// and inflated LEB vector counts before CompileModule.
//
// Critical: a tiny module can declare export_count ≈ 2e8 via crafted LEB; wazero
// pre-allocates and OOMs the whole process (cannot recover in-process). Reject when
// declared count exceeds remaining section bytes (each entry ≥ 1 byte) or hard caps.
func rejectWasmHostileSections(wasm []byte) error {
	if len(wasm) < 8 {
		return nil
	}
	pos := 8
	for pos < len(wasm) {
		sectionID := wasm[pos]
		pos++
		size, err := readU32LEB(wasm, &pos)
		if err != nil {
			return err
		}
		if pos+int(size) > len(wasm) {
			return errors.New("sandbox: malformed wasm section size")
		}
		body := wasm[pos : pos+int(size)]
		switch sectionID {
		case 1: // type
			if err := rejectWasmVectorPrefix(body, maxWasmTypeCount); err != nil {
				return err
			}
		case 2:
			return errors.New("sandbox: wasm import section not allowed")
		case 3: // function
			if err := rejectWasmVectorPrefix(body, maxWasmFuncCount); err != nil {
				return err
			}
		case 4:
			if err := rejectWasmTableSection(body); err != nil {
				return err
			}
		case 5: // memory
			if err := rejectWasmVectorPrefix(body, maxWasmMemoryCount); err != nil {
				return err
			}
		case 6: // global
			if err := rejectWasmVectorPrefix(body, maxWasmGlobalCount); err != nil {
				return err
			}
		case 7: // export — primary OOM vector (inflated count LEB)
			if err := rejectWasmVectorPrefix(body, maxWasmExportCount); err != nil {
				return err
			}
		case 8:
			return errors.New("sandbox: wasm start section not allowed")
		case 9:
			if int(size) > maxWasmElementSectionBytes {
				return errors.New("sandbox: wasm element section too large")
			}
			if err := rejectWasmElementSection(body); err != nil {
				return err
			}
		case 10: // code
			if err := rejectWasmVectorPrefix(body, maxWasmCodeCount); err != nil {
				return err
			}
		case 11: // data
			if err := rejectWasmVectorPrefix(body, maxWasmDataCount); err != nil {
				return err
			}
		case 12: // data count
			if err := rejectWasmVectorPrefix(body, maxWasmDataCount); err != nil {
				return err
			}
		case 13: // element count / declarative (treat as vector)
			if err := rejectWasmVectorPrefix(body, maxWasmDeclaredCount); err != nil {
				return err
			}
		}
		pos += int(size)
	}
	return nil
}

// rejectWasmStartSection is kept for callers/tests that name the start check explicitly.
func rejectWasmStartSection(wasm []byte) error {
	return rejectWasmHostileSections(wasm)
}

// rejectWasmVectorPrefix reads a leading LEB count and rejects inflated counts that
// cannot fit in the remaining section body (wazero prealloc DoS).
func rejectWasmVectorPrefix(body []byte, maxCount uint32) error {
	pos := 0
	count, err := readU32LEB(body, &pos)
	if err != nil {
		return err
	}
	if count > maxCount {
		return errors.New("sandbox: wasm vector count too large")
	}
	remain := len(body) - pos
	if remain < 0 {
		return errors.New("sandbox: malformed wasm vector section")
	}
	// Every vector entry consumes at least one payload byte.
	if int(count) > remain {
		return errors.New("sandbox: wasm vector count exceeds section bytes")
	}
	return nil
}

func rejectWasmTableSection(body []byte) error {
	pos := 0
	count, err := readU32LEB(body, &pos)
	if err != nil {
		return err
	}
	if count > 4 {
		return errors.New("sandbox: too many wasm tables")
	}
	remain := len(body) - pos
	if int(count) > remain {
		return errors.New("sandbox: wasm vector count exceeds section bytes")
	}
	for i := uint32(0); i < count; i++ {
		if pos >= len(body) {
			return errors.New("sandbox: malformed wasm table section")
		}
		// reftype (funcref 0x70 / externref 0x6f)
		pos++
		if pos >= len(body) {
			return errors.New("sandbox: malformed wasm table limits")
		}
		flag := body[pos]
		pos++
		minimum, err := readU32LEB(body, &pos)
		if err != nil {
			return err
		}
		if minimum > maxWasmTableInitial {
			return errors.New("sandbox: wasm table initial size too large")
		}
		if flag&0x01 != 0 {
			if _, err := readU32LEB(body, &pos); err != nil {
				return err
			}
		}
	}
	return nil
}

func rejectWasmElementSection(body []byte) error {
	pos := 0
	count, err := readU32LEB(body, &pos)
	if err != nil {
		return err
	}
	if count > maxWasmElementCount {
		return errors.New("sandbox: too many wasm element segments")
	}
	remain := len(body) - pos
	if int(count) > remain {
		return errors.New("sandbox: wasm vector count exceeds section bytes")
	}
	return nil
}

func readU32LEB(data []byte, pos *int) (uint32, error) {
	var result uint32
	var shift uint
	for i := 0; i < 5; i++ {
		if *pos >= len(data) {
			return 0, errors.New("sandbox: unexpected eof in leb128")
		}
		b := data[*pos]
		*pos = *pos + 1
		result |= uint32(b&0x7f) << shift
		if (b & 0x80) == 0 {
			return result, nil
		}
		shift += 7
	}
	return 0, errors.New("sandbox: leb128 too long")
}
