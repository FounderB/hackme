//go:build cuda

package gpupoh

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"unsafe"

	"hackme/internal/hotlog"

	"github.com/pkg/errors"
	"gorgonia.org/cu"
	"gorgonia.org/cu/nvrtc"
)

//go:embed poh_search.cu
var pohCuSource string

const blockThreads = 256

type pohPTXEntry struct {
	ptx     string
	kname   string
	knameB3 string
	arch    string
	err     error
}

var pohPTXByArch sync.Map // arch string -> *pohPTXEntry

func cudaDivUp(x, y int) int {
	if y <= 0 {
		return x
	}
	return (x + y - 1) / y
}

func compilePTXForArchChain(chain []string) (ptx string, kname, knameB3, usedArch string, err error) {
	if len(chain) == 0 {
		chain = nvrtcArchChain(0, 0)
	}
	var lastErr error
	for _, arch := range chain {
		if v, ok := pohPTXByArch.Load(arch); ok {
			e := v.(*pohPTXEntry)
			if e.err == nil && e.ptx != "" {
				return e.ptx, e.kname, e.knameB3, e.arch, nil
			}
			lastErr = e.err
			continue
		}
		ptxB, kn, knB3, e := compilePTXOnce(arch)
		entry := &pohPTXEntry{ptx: ptxB, kname: kn, knameB3: knB3, arch: arch, err: e}
		pohPTXByArch.Store(arch, entry)
		if e == nil && ptxB != "" {
			return ptxB, kn, knB3, arch, nil
		}
		lastErr = e
	}
	if lastErr != nil {
		return "", "", "", "", lastErr
	}
	return "", "", "", "", fmt.Errorf("gpupoh: nvrtc compile failed for all arch targets %v", chain)
}

func compilePTXOnce(arch string) (ptx string, kname, knameB3 string, err error) {
	prog, e := nvrtc.CreateProgram(pohCuSource, "poh_search.cu")
	if e != nil {
		return "", "", "", errors.Wrap(e, "gpupoh: nvrtc CreateProgram")
	}
	defer func() { _ = prog.Destroy() }()
	if e := prog.AddNameExpression("poh_search"); e != nil {
		return "", "", "", errors.Wrap(e, "gpupoh: AddNameExpression poh_search")
	}
	if e := prog.AddNameExpression("poh_search_blake3"); e != nil {
		return "", "", "", errors.Wrap(e, "gpupoh: AddNameExpression poh_search_blake3")
	}
	flag := arch
	if !strings.HasPrefix(flag, "compute_") && !strings.HasPrefix(flag, "sm_") {
		flag = "compute_" + flag
	}
	opts := []string{
		"--gpu-architecture=" + flag,
		"--std=c++14",
		"-default-device",
	}
	if e := prog.Compile(opts...); e != nil {
		log, _ := prog.GetLog()
		return "", "", "", fmt.Errorf("gpupoh: nvrtc Compile(%s): %w (log: %s)", flag, e, strings.TrimSpace(log))
	}
	ptxStr, e := prog.GetPTX()
	if e != nil {
		return "", "", "", errors.Wrap(e, "gpupoh: GetPTX")
	}
	kn, e := prog.GetLoweredName("poh_search")
	if e != nil {
		return "", "", "", errors.Wrap(e, "gpupoh: GetLoweredName poh_search")
	}
	knB3, e := prog.GetLoweredName("poh_search_blake3")
	if e != nil {
		return "", "", "", errors.Wrap(e, "gpupoh: GetLoweredName poh_search_blake3")
	}
	return ptxStr, kn, knB3, nil
}

type cudaAccel struct {
	devID    int
	devName  string
	arch     string
	ctx      cu.CUContext
	mod      cu.Module
	kernel   cu.Function
	kernelB3 cu.Function
	dOut     cu.DevicePtr
	dPrefix  cu.DevicePtr
	prefixN  int64
	blockSz  int
	haveCtx  bool
	haveMod  bool
	haveB3   bool
}

func newCUDAAccelerator(devID int) (Accelerator, error) {
	chain, err := nvrtcArchChainForDevice(devID)
	if err != nil {
		return nil, err
	}
	ptx, kname, knameB3, usedArch, err := compilePTXForArchChain(chain)
	if err != nil {
		return nil, err
	}
	dev, err := cu.GetDevice(devID)
	if err != nil {
		return nil, errors.Wrap(err, "gpupoh: GetDevice")
	}
	name, _ := dev.Name()
	if name == "" {
		name = fmt.Sprintf("CUDA device %d", devID)
	}
	ctx, err := dev.MakeContext(cu.SchedAuto)
	if err != nil {
		return nil, errors.Wrap(err, "gpupoh: MakeContext")
	}
	mod, err := cu.LoadData(ptx)
	if err != nil {
		_ = ctx.Destroy()
		return nil, errors.Wrapf(err, "gpupoh: LoadData (arch=%s)", usedArch)
	}
	fn, err := mod.Function(kname)
	if err != nil {
		_ = mod.Unload()
		_ = ctx.Destroy()
		return nil, errors.Wrap(err, "gpupoh: Function poh_search")
	}
	fnB3, errB3 := mod.Function(knameB3)
	haveB3 := errB3 == nil
	if !haveB3 && os.Getenv("HACKME_CUDA_VERBOSE") == "1" {
		hotlog.VerboseStderrf("gpupoh: blake3 kernel unavailable (%v); host Blake3Search fallback", errB3)
	}
	dOut, err := cu.MemAlloc(8)
	if err != nil {
		_ = mod.Unload()
		_ = ctx.Destroy()
		return nil, errors.Wrap(err, "gpupoh: MemAlloc")
	}
	return &cudaAccel{
		devID:    devID,
		devName:  name,
		arch:     usedArch,
		ctx:      ctx,
		mod:      mod,
		kernel:   fn,
		kernelB3: fnB3,
		dOut:     dOut,
		blockSz:  envCUDABlockThreads(),
		haveCtx:  true,
		haveMod:  true,
		haveB3:   haveB3,
	}, nil
}

func (a *cudaAccel) DeviceIndex() int   { return a.devID }
func (a *cudaAccel) DeviceName() string { return a.devName }
func (a *cudaAccel) Backend() string    { return "cuda" }
func (a *cudaAccel) Label() string {
	if a.arch != "" {
		return fmt.Sprintf("GPU #%d [CUDA %s]", a.devID, a.arch)
	}
	return fmt.Sprintf("GPU #%d [CUDA]", a.devID)
}
func (a *cudaAccel) Close() error { return a.close() }

func (a *cudaAccel) close() error {
	var first error
	if a.dPrefix != 0 {
		if err := cu.MemFree(a.dPrefix); err != nil && first == nil {
			first = err
		}
		a.dPrefix = 0
		a.prefixN = 0
	}
	if a.dOut != 0 {
		if err := cu.MemFree(a.dOut); err != nil && first == nil {
			first = err
		}
		a.dOut = 0
	}
	if a.haveMod {
		if err := a.mod.Unload(); err != nil && first == nil {
			first = err
		}
		a.haveMod = false
	}
	if a.haveCtx {
		if err := a.ctx.Destroy(); err != nil && first == nil {
			first = err
		}
		a.haveCtx = false
	}
	return first
}

func (a *cudaAccel) SearchPoH(ctx context.Context, req SearchRequest) (bool, uint64, error) {
	if req.FormulaVersion >= 3 {
		if a.haveB3 && os.Getenv("HACKME_CUDA_BLAKE3_HOST") != "1" {
			found, nonce, err := a.searchBlake3Device(ctx, req)
			if err == nil {
				return found, nonce, nil
			}
			if os.Getenv("HACKME_CUDA_VERBOSE") == "1" {
				hotlog.VerboseStderrf("gpupoh: cuda blake3 device error, host fallback: %v", err)
			}
		}
		// Host path matches chain.PohWorkBlake3 exactly (correctness lock + NVRTC escape hatch).
		return Blake3Search(ctx, req.Base, req.Count, req.Mod, req.PrevHash, req.MinerAddress)
	}
	return a.Search(ctx, req.Base, req.Count, req.Mod)
}

func (a *cudaAccel) ensurePrefix(n int) error {
	if n <= 0 {
		return nil
	}
	need := int64(n)
	if a.dPrefix != 0 && a.prefixN >= need {
		return nil
	}
	if a.dPrefix != 0 {
		if err := cu.MemFree(a.dPrefix); err != nil {
			return errors.Wrap(err, "gpupoh: MemFree prefix")
		}
		a.dPrefix = 0
		a.prefixN = 0
	}
	cap := need
	if cap < 256 {
		cap = 256
	}
	ptr, err := cu.MemAlloc(int64(cap))
	if err != nil {
		return errors.Wrap(err, "gpupoh: MemAlloc prefix")
	}
	a.dPrefix = ptr
	a.prefixN = cap
	return nil
}

func (a *cudaAccel) searchBlake3Device(ctx context.Context, req SearchRequest) (bool, uint64, error) {
	count := req.Count
	mod := req.Mod
	if count == 0 || mod == 0 {
		return false, 0, nil
	}
	if count > maxBatch {
		count = maxBatch
	}
	prev := strings.ToLower(strings.TrimSpace(req.PrevHash))
	addr := strings.TrimSpace(req.MinerAddress)
	prefix := append([]byte(prev), addr...)
	if len(prefix) > 960 {
		return false, 0, fmt.Errorf("gpupoh: blake3 prefix len %d > 960", len(prefix))
	}
	select {
	case <-ctx.Done():
		return false, 0, ctx.Err()
	default:
	}
	if a.haveCtx {
		if err := a.ctx.Lock(); err != nil {
			return false, 0, errors.Wrap(err, "gpupoh: ctx.Lock")
		}
		defer func() { _ = a.ctx.Unlock() }()
	}
	t0 := time.Now()

	bt := a.blockSz
	if bt < 32 {
		bt = blockThreads
	}

	maxU := ^uint64(0)
	if err := cu.MemcpyHtoD(a.dOut, unsafe.Pointer(&maxU), 8); err != nil {
		return false, 0, errors.Wrap(err, "gpupoh: MemcpyHtoD init")
	}
	if err := a.ensurePrefix(len(prefix)); err != nil {
		return false, 0, err
	}
	if len(prefix) > 0 {
		if err := cu.MemcpyHtoD(a.dPrefix, unsafe.Pointer(&prefix[0]), int64(len(prefix))); err != nil {
			return false, 0, errors.Wrap(err, "gpupoh: MemcpyHtoD prefix")
		}
	}

	b := req.Base
	c := count
	m := mod
	plen := int32(len(prefix))
	dptr := a.dOut
	pptr := a.dPrefix
	args := []unsafe.Pointer{
		unsafe.Pointer(&b),
		unsafe.Pointer(&c),
		unsafe.Pointer(&m),
		unsafe.Pointer(&pptr),
		unsafe.Pointer(&plen),
		unsafe.Pointer(&dptr),
	}

	grid := cudaDivUp(int(count), bt)
	if grid < 1 {
		grid = 1
	}

	if err := a.kernelB3.Launch(grid, 1, 1, bt, 1, 1, 0, cu.Stream{}, args); err != nil {
		return false, 0, errors.Wrap(err, "gpupoh: Launch poh_search_blake3")
	}
	if err := cu.Synchronize(); err != nil {
		return false, 0, errors.Wrap(err, "gpupoh: Synchronize blake3")
	}
	kernelSec := time.Since(t0).Seconds()
	recordCUDAKernelDuration(kernelSec)
	if os.Getenv("HACKME_CUDA_VERBOSE") == "1" {
		ghs := float64(count) / kernelSec / 1e9
		hotlog.VerboseStderrf("gpupoh: cuda blake3 search count=%d elapsed=%s ~%.2f GH/s", count, time.Duration(kernelSec*float64(time.Second)).Round(time.Millisecond), ghs)
	}

	var out uint64
	if err := cu.MemcpyDtoH(unsafe.Pointer(&out), a.dOut, 8); err != nil {
		return false, 0, errors.Wrap(err, "gpupoh: MemcpyDtoH blake3")
	}
	if out == maxU {
		return false, 0, nil
	}
	return true, out, nil
}

func (a *cudaAccel) Search(ctx context.Context, base, count, mod uint64) (found bool, nonce uint64, err error) {
	if count == 0 || mod == 0 {
		return false, 0, nil
	}
	if count > maxBatch {
		count = maxBatch
	}
	select {
	case <-ctx.Done():
		return false, 0, ctx.Err()
	default:
	}
	// gorgonia/cu Memcpy/Launch use the driver current context — bind this device's ctx to this OS thread.
	if a.haveCtx {
		if err := a.ctx.Lock(); err != nil {
			return false, 0, errors.Wrap(err, "gpupoh: ctx.Lock")
		}
		defer func() { _ = a.ctx.Unlock() }()
	}
	t0 := time.Now()

	bt := a.blockSz
	if bt < 32 {
		bt = blockThreads
	}

	maxU := ^uint64(0)
	if err := cu.MemcpyHtoD(a.dOut, unsafe.Pointer(&maxU), 8); err != nil {
		return false, 0, errors.Wrap(err, "gpupoh: MemcpyHtoD init")
	}

	b := base
	c := count
	m := mod
	dptr := a.dOut
	args := []unsafe.Pointer{
		unsafe.Pointer(&b),
		unsafe.Pointer(&c),
		unsafe.Pointer(&m),
		unsafe.Pointer(&dptr),
	}

	grid := cudaDivUp(int(count), bt)
	if grid < 1 {
		grid = 1
	}

	if err := a.kernel.Launch(grid, 1, 1, bt, 1, 1, 0, cu.Stream{}, args); err != nil {
		return false, 0, errors.Wrap(err, "gpupoh: Launch")
	}
	if err := cu.Synchronize(); err != nil {
		return false, 0, errors.Wrap(err, "gpupoh: Synchronize")
	}
	kernelSec := time.Since(t0).Seconds()
	recordCUDAKernelDuration(kernelSec)
	if os.Getenv("HACKME_CUDA_VERBOSE") == "1" {
		ghs := float64(count) / kernelSec / 1e9
		// Non-blocking: AV/disk stalls on the log fd must not freeze the mining loop (#1).
		hotlog.VerboseStderrf("gpupoh: cuda search count=%d elapsed=%s ~%.2f GH/s", count, time.Duration(kernelSec*float64(time.Second)).Round(time.Millisecond), ghs)
	}

	var out uint64
	if err := cu.MemcpyDtoH(unsafe.Pointer(&out), a.dOut, 8); err != nil {
		return false, 0, errors.Wrap(err, "gpupoh: MemcpyDtoH")
	}
	if out == maxU {
		return false, 0, nil
	}
	return true, out, nil
}
