package gpupoh

import (
	"context"
	"errors"
)

// Accelerator is one GPU PoH search backend (CUDA or OpenCL).
type Accelerator interface {
	DeviceIndex() int
	DeviceName() string
	Backend() string
	// Label is a stable UI string, e.g. "GPU #0 [CUDA]".
	Label() string
	// Search is v1 algebraic PoH (eval=n*7+13). Prefer SearchPoH for version-aware mining.
	Search(ctx context.Context, base, count, mod uint64) (found bool, nonce uint64, err error)
	// SearchPoH searches v1 or Blake3 v3 according to req.FormulaVersion.
	SearchPoH(ctx context.Context, req SearchRequest) (found bool, nonce uint64, err error)
	Close() error
}

// ShareSearcher optionally collects PPS shares during the same hash pass.
type ShareSearcher interface {
	SearchPoHCollect(ctx context.Context, req SearchRequest) (SearchResult, error)
}

// SearchCollect runs PoH search and returns shares when req.ShareMod > 0.
// Prefers device ShareSearcher; falls back to host Blake3SearchCollect.
func SearchCollect(ctx context.Context, acc Accelerator, req SearchRequest) (SearchResult, error) {
	if req.FormulaVersion >= 3 {
		if ss, ok := acc.(ShareSearcher); ok {
			return ss.SearchPoHCollect(ctx, req)
		}
		if req.ShareMod > 0 {
			return Blake3SearchCollect(ctx, req)
		}
		ok, n, err := Blake3Search(ctx, req.Base, req.Count, req.Mod, req.PrevHash, req.MinerAddress)
		return SearchResult{Found: ok, Nonce: n}, err
	}
	if acc == nil {
		return SearchResult{}, ErrNoGPU
	}
	ok, n, err := acc.Search(ctx, req.Base, req.Count, req.Mod)
	return SearchResult{Found: ok, Nonce: n}, err
}

// ErrNoGPU is returned when no GPU backend could be initialized.
var ErrNoGPU = errors.New("gpupoh: no GPU accelerators available")

// GPUDeviceInfo is a lightweight device listing (no contexts created for CUDA).
type GPUDeviceInfo struct {
	Index   int    `json:"index"`
	Name    string `json:"name"`
	Backend string `json:"backend"` // "cuda" or "opencl"
}
