package sandbox

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// WasmEvalTimeout caps a single eval(i64)->i64 call (embedded lock is trivial; limit is for future/custom WASM).
const WasmEvalTimeout = 2 * time.Second

// lockWasm is a minimal valid Wasm 1.0 module exporting eval(i64)->i64: return n*7+13.
// type section payload = 6 bytes: 1 functype (i64)->(i64)
const lockWasmHex = "0061736d0100000001060160017e017e03020100070801046576616c00000a0c010a00200042077e420d7c0b"

var (
	lockMu  sync.Mutex
	rt      wazero.Runtime
	mod     api.Module
	evalFn  api.Function
	initErr error
	lockRaw []byte
)

var errNoEval = errors.New("sandbox: missing eval export")

func rebuildLockWasm(ctx context.Context) error {
	if mod != nil {
		_ = mod.Close(context.Background())
		mod = nil
	}
	if rt != nil {
		_ = rt.Close(context.Background())
		rt = nil
	}
	evalFn = nil
	initErr = nil
	if lockRaw == nil {
		raw, err := hex.DecodeString(lockWasmHex)
		if err != nil {
			initErr = err
			return err
		}
		lockRaw = raw
	}
	// Interpreter avoids executable-mmap on locked-down VPS kernels.
	// WithCloseOnContextDone so WasmEvalTimeout aborts a stuck eval; we rebuild after close.
	rt = wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigInterpreter().
		WithMemoryLimitPages(128).
		WithCloseOnContextDone(true))
	compiled, err := rt.CompileModule(ctx, lockRaw)
	if err != nil {
		initErr = err
		return err
	}
	mod, err = rt.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName("lock"))
	if err != nil {
		initErr = err
		return err
	}
	evalFn = mod.ExportedFunction("eval")
	if evalFn == nil {
		initErr = errNoEval
		return initErr
	}
	return nil
}

func isLockClosedErr(err error) bool {
	if err == nil {
		return false
	}
	low := strings.ToLower(err.Error())
	return strings.Contains(low, "closed") || strings.Contains(low, "context deadline")
}

// Eval runs the embedded lock WASM: eval(nonce) = nonce*7 + 13.
// Serialized: wazero module instances are not safe for concurrent Call.
func Eval(ctx context.Context, nonce uint64) (uint64, error) {
	lockMu.Lock()
	defer lockMu.Unlock()
	if evalFn == nil || initErr != nil {
		if err := rebuildLockWasm(ctx); err != nil {
			return 0, err
		}
	}
	callCtx, cancel := context.WithTimeout(ctx, WasmEvalTimeout)
	defer cancel()
	res, err := evalFn.Call(callCtx, nonce)
	if err != nil && isLockClosedErr(err) {
		if err2 := rebuildLockWasm(context.Background()); err2 != nil {
			return 0, err
		}
		callCtx2, cancel2 := context.WithTimeout(ctx, WasmEvalTimeout)
		defer cancel2()
		res, err = evalFn.Call(callCtx2, nonce)
	}
	if err != nil {
		return 0, err
	}
	return res[0], nil
}
