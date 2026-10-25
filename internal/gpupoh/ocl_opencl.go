//go:build opencl

package gpupoh

/*
#cgo windows LDFLAGS: -lOpenCL
#cgo linux LDFLAGS: -lOpenCL
#cgo darwin LDFLAGS: -framework OpenCL

#define CL_TARGET_OPENCL_VERSION 120
#ifdef __APPLE__
#include <OpenCL/opencl.h>
#else
#include <CL/cl.h>
#endif

#include <stdlib.h>
#include <string.h>

static cl_program poh_build_program(cl_context ctx, cl_device_id dev, const char *src, char **log_out) {
	cl_int err = 0;
	size_t srclen = strlen(src);
	cl_program p = clCreateProgramWithSource(ctx, 1, &src, &srclen, &err);
	if (err != CL_SUCCESS || !p) {
		return NULL;
	}
	const char *opts = "-cl-std=CL1.2";
	err = clBuildProgram(p, 1, &dev, opts, NULL, NULL);
	if (err != CL_SUCCESS) {
		size_t log_size = 0;
		clGetProgramBuildInfo(p, dev, CL_PROGRAM_BUILD_LOG, 0, NULL, &log_size);
		if (log_size > 0) {
			char *buf = (char *)malloc(log_size + 1);
			if (buf) {
				clGetProgramBuildInfo(p, dev, CL_PROGRAM_BUILD_LOG, log_size, buf, NULL);
				buf[log_size] = 0;
				*log_out = buf;
			}
		}
		clReleaseProgram(p);
		return NULL;
	}
	return p;
}

static cl_kernel poh_create_kernel_named(cl_program p, const char *name) {
	cl_int err = 0;
	cl_kernel k = clCreateKernel(p, name, &err);
	if (err != CL_SUCCESS || !k) {
		return NULL;
	}
	return k;
}
*/
import "C"

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
)

//go:embed poh_search.cl
var oclKernelSource string

// Must match BLAKE3_NONCES_PER_THREAD in poh_search.cl.
const oclBlake3NoncesPerThread = 2

type oclAccel struct {
	devID       int
	devName     string
	ctx         C.cl_context
	queue       C.cl_command_queue
	program     C.cl_program
	kernel      C.cl_kernel
	kernelB3    C.cl_kernel
	memOut      C.cl_mem
	memLock     C.cl_mem
	memPrefix   C.cl_mem
	memShareCnt C.cl_mem
	memShares   C.cl_mem
	shareCap    int
	prefixN     int
	haveB3      bool
}

func (a *oclAccel) DeviceIndex() int   { return a.devID }
func (a *oclAccel) DeviceName() string { return a.devName }
func (a *oclAccel) Backend() string    { return "opencl" }
func (a *oclAccel) Label() string      { return fmt.Sprintf("GPU #%d [OpenCL]", a.devID) }
func (a *oclAccel) Close() error       { return a.close() }

func (a *oclAccel) close() error {
	if a.memShares != nil {
		_ = C.clReleaseMemObject(a.memShares)
		a.memShares = nil
	}
	if a.memShareCnt != nil {
		_ = C.clReleaseMemObject(a.memShareCnt)
		a.memShareCnt = nil
	}
	if a.memPrefix != nil {
		_ = C.clReleaseMemObject(a.memPrefix)
		a.memPrefix = nil
	}
	if a.memLock != nil {
		_ = C.clReleaseMemObject(a.memLock)
		a.memLock = nil
	}
	if a.memOut != nil {
		_ = C.clReleaseMemObject(a.memOut)
		a.memOut = nil
	}
	if a.kernelB3 != nil {
		_ = C.clReleaseKernel(a.kernelB3)
		a.kernelB3 = nil
	}
	if a.kernel != nil {
		_ = C.clReleaseKernel(a.kernel)
		a.kernel = nil
	}
	if a.program != nil {
		_ = C.clReleaseProgram(a.program)
		a.program = nil
	}
	if a.queue != nil {
		_ = C.clReleaseCommandQueue(a.queue)
		a.queue = nil
	}
	if a.ctx != nil {
		_ = C.clReleaseContext(a.ctx)
		a.ctx = nil
	}
	a.haveB3 = false
	return nil
}

func (a *oclAccel) SearchPoH(ctx context.Context, req SearchRequest) (bool, uint64, error) {
	res, err := a.SearchPoHCollect(ctx, req)
	return res.Found, res.Nonce, err
}

func (a *oclAccel) SearchPoHCollect(ctx context.Context, req SearchRequest) (SearchResult, error) {
	if req.FormulaVersion >= 3 {
		forceHost := os.Getenv("HACKME_OPENCL_BLAKE3_HOST") == "1"
		if a.haveB3 && !forceHost {
			res, err := a.searchBlake3Device(ctx, req)
			if err == nil {
				return res, nil
			}
			oclBlake3DeviceErrOnce.Do(func() {
				hotlog.Stderrf("gpupoh: opencl blake3 device error, host fallback: %v", err)
			})
			if os.Getenv("HACKME_OPENCL_VERBOSE") == "1" {
				hotlog.VerboseStderrf("gpupoh: opencl blake3 device error, host fallback: %v", err)
			}
		} else if forceHost {
			oclBlake3ForcedHostOnce.Do(func() {
				hotlog.Stderrf("gpupoh: HACKME_OPENCL_BLAKE3_HOST=1; using host Blake3Search")
			})
		} else if !a.haveB3 {
			oclBlake3MissingHostOnce.Do(func() {
				hotlog.Stderrf("gpupoh: opencl blake3 device kernel missing; host Blake3Search fallback")
			})
		}
		return Blake3SearchCollect(ctx, req)
	}
	ok, n, err := a.Search(ctx, req.Base, req.Count, req.Mod)
	return SearchResult{Found: ok, Nonce: n, Scanned: req.Count}, err
}

var (
	oclBlake3ForcedHostOnce  sync.Once
	oclBlake3MissingHostOnce sync.Once
	oclBlake3DeviceErrOnce   sync.Once
	oclBlake3DeviceOKOnce    sync.Once
)

func oclDivUp(x, y int) int {
	if y <= 0 {
		return x
	}
	return (x + y - 1) / y
}

func (a *oclAccel) ensurePrefix(n int) error {
	if n <= 0 {
		return nil
	}
	if a.memPrefix != nil && a.prefixN >= n {
		return nil
	}
	if a.memPrefix != nil {
		_ = C.clReleaseMemObject(a.memPrefix)
		a.memPrefix = nil
		a.prefixN = 0
	}
	capN := n
	if capN < 256 {
		capN = 256
	}
	var err C.cl_int
	mem := C.clCreateBuffer(a.ctx, C.CL_MEM_READ_ONLY, C.size_t(capN), nil, &err)
	if mem == nil || err != C.CL_SUCCESS {
		return fmt.Errorf("gpupoh ocl: CreateBuffer prefix %d", int(err))
	}
	a.memPrefix = mem
	a.prefixN = capN
	return nil
}

func (a *oclAccel) searchBlake3Device(ctx context.Context, req SearchRequest) (SearchResult, error) {
	var out SearchResult
	total := req.Count
	mod := req.Mod
	if total == 0 || mod == 0 {
		return out, nil
	}
	prev := strings.ToLower(strings.TrimSpace(req.PrevHash))
	addr := strings.TrimSpace(req.MinerAddress)
	prefix := append([]byte(prev), addr...)
	if len(prefix) > 152 {
		return out, fmt.Errorf("gpupoh: blake3 prefix len %d > 152 (device stack cap)", len(prefix))
	}
	select {
	case <-ctx.Done():
		return out, ctx.Err()
	default:
	}

	div, ok := prepareBlake3ModDiv(mod)
	if !ok {
		return out, nil
	}
	shareMod := req.ShareMod
	maxShares := req.MaxShares
	if maxShares <= 0 {
		maxShares = a.shareCap
	}
	if maxShares > a.shareCap {
		maxShares = a.shareCap
	}
	var shareDiv blake3ModDiv
	if shareMod > 0 {
		shareDiv, ok = prepareBlake3ModDiv(shareMod)
		if !ok {
			shareMod = 0
		}
	}

	if err := a.ensurePrefix(len(prefix)); err != nil {
		return out, err
	}
	if len(prefix) > 0 {
		errN := C.clEnqueueWriteBuffer(a.queue, a.memPrefix, C.CL_TRUE, 0, C.size_t(len(prefix)), unsafe.Pointer(&prefix[0]), 0, nil, nil)
		if errN != C.CL_SUCCESS {
			return out, fmt.Errorf("gpupoh ocl: WriteBuffer prefix %d", int(errN))
		}
	}

	oclBlake3DeviceOKOnce.Do(func() {
		hotlog.Stderrf("gpupoh: OpenCL Blake3 device kernel active (%s)", a.devName)
	})

	best := ^uint64(0)
	var shareBuf []uint64
	if shareMod > 0 && maxShares > 0 {
		shareBuf = make([]uint64, 0, maxShares)
	}
	mergeShares := func(more []uint64) {
		for _, s := range more {
			if len(shareBuf) >= maxShares {
				return
			}
			shareBuf = append(shareBuf, s)
		}
	}

	remain := total
	cur := req.Base
	for remain > 0 {
		select {
		case <-ctx.Done():
			out.Shares = shareBuf
			out.Scanned = total - remain
			if best != ^uint64(0) {
				out.Found = true
				out.Nonce = best
			}
			return out, ctx.Err()
		default:
		}
		n := remain
		if n > maxBatch {
			n = maxBatch
		}
		chunk, err := a.launchBlake3Chunk(cur, n, mod, div, prefix, shareMod, shareDiv, maxShares)
		if err != nil {
			out.Shares = shareBuf
			out.Scanned = total - remain
			if best != ^uint64(0) {
				out.Found = true
				out.Nonce = best
			}
			return out, err
		}
		out.Scanned += n
		if chunk.Found && chunk.Nonce < best {
			best = chunk.Nonce
		}
		mergeShares(chunk.Shares)
		cur += n
		remain -= n
		// PPS: keep scanning for shares even after a block hit (match host/CUDA worker loops).
		if shareMod == 0 && best != ^uint64(0) {
			break
		}
		if shareMod > 0 && len(shareBuf) >= maxShares && best != ^uint64(0) {
			break
		}
	}
	out.Shares = shareBuf
	if best != ^uint64(0) {
		out.Found = true
		out.Nonce = best
	}
	return out, nil
}

func (a *oclAccel) launchBlake3Chunk(base, count, mod uint64, div blake3ModDiv, prefix []byte,
	shareMod uint64, shareDiv blake3ModDiv, maxShares int) (SearchResult, error) {
	var res SearchResult
	if count == 0 || a.kernelB3 == nil {
		return res, nil
	}
	t0 := time.Now()

	var out uint64 = ^uint64(0)
	errN := C.clEnqueueWriteBuffer(a.queue, a.memOut, C.CL_TRUE, 0, C.size_t(8), unsafe.Pointer(&out), 0, nil, nil)
	if errN != C.CL_SUCCESS {
		return res, fmt.Errorf("gpupoh ocl: EnqueueWriteBuffer init %d", int(errN))
	}
	var lockZero C.cl_int
	errN = C.clEnqueueWriteBuffer(a.queue, a.memLock, C.CL_TRUE, 0, C.size_t(4), unsafe.Pointer(&lockZero), 0, nil, nil)
	if errN != C.CL_SUCCESS {
		return res, fmt.Errorf("gpupoh ocl: EnqueueWriteBuffer lock %d", int(errN))
	}
	var shareZero uint32
	if a.memShareCnt != nil {
		errN = C.clEnqueueWriteBuffer(a.queue, a.memShareCnt, C.CL_TRUE, 0, C.size_t(4), unsafe.Pointer(&shareZero), 0, nil, nil)
		if errN != C.CL_SUCCESS {
			return res, fmt.Errorf("gpupoh ocl: EnqueueWriteBuffer share_count %d", int(errN))
		}
	}

	b := C.cl_ulong(base)
	c := C.cl_ulong(count)
	m := C.cl_ulong(mod)
	inv := C.cl_ulong(div.Inv)
	lim := C.cl_ulong(div.Lim)
	tz := C.cl_uint(div.Tz)
	plen := C.cl_int(len(prefix))
	smod := C.cl_ulong(shareMod)
	sinv := C.cl_ulong(shareDiv.Inv)
	slim := C.cl_ulong(shareDiv.Lim)
	stz := C.cl_uint(shareDiv.Tz)
	ms := C.cl_uint(maxShares)
	if shareMod == 0 {
		ms = 0
	}
	// Local cl_mem copies — avoid &a.field (cgo: Go pointer to unpinned Go pointer).
	memPrefix := a.memPrefix
	memOut := a.memOut
	memLock := a.memLock
	memShareCnt := a.memShareCnt
	memShares := a.memShares
	kernB3 := a.kernelB3

	setU64 := func(idx int, v *C.cl_ulong) error {
		e := C.clSetKernelArg(kernB3, C.cl_uint(idx), C.size_t(unsafe.Sizeof(*v)), unsafe.Pointer(v))
		if e != C.CL_SUCCESS {
			return fmt.Errorf("gpupoh ocl: SetKernelArg blake3 %d: %d", idx, int(e))
		}
		return nil
	}
	setU32 := func(idx int, v *C.cl_uint) error {
		e := C.clSetKernelArg(kernB3, C.cl_uint(idx), C.size_t(unsafe.Sizeof(*v)), unsafe.Pointer(v))
		if e != C.CL_SUCCESS {
			return fmt.Errorf("gpupoh ocl: SetKernelArg blake3 %d: %d", idx, int(e))
		}
		return nil
	}
	setI32 := func(idx int, v *C.cl_int) error {
		e := C.clSetKernelArg(kernB3, C.cl_uint(idx), C.size_t(unsafe.Sizeof(*v)), unsafe.Pointer(v))
		if e != C.CL_SUCCESS {
			return fmt.Errorf("gpupoh ocl: SetKernelArg blake3 %d: %d", idx, int(e))
		}
		return nil
	}
	setMem := func(idx int, v *C.cl_mem) error {
		e := C.clSetKernelArg(kernB3, C.cl_uint(idx), C.size_t(unsafe.Sizeof(*v)), unsafe.Pointer(v))
		if e != C.CL_SUCCESS {
			return fmt.Errorf("gpupoh ocl: SetKernelArg blake3 %d: %d", idx, int(e))
		}
		return nil
	}
	if err := setU64(0, &b); err != nil {
		return res, err
	}
	if err := setU64(1, &c); err != nil {
		return res, err
	}
	if err := setU64(2, &m); err != nil {
		return res, err
	}
	if err := setU64(3, &inv); err != nil {
		return res, err
	}
	if err := setU64(4, &lim); err != nil {
		return res, err
	}
	if err := setU32(5, &tz); err != nil {
		return res, err
	}
	if err := setMem(6, &memPrefix); err != nil {
		return res, err
	}
	if err := setI32(7, &plen); err != nil {
		return res, err
	}
	if err := setMem(8, &memOut); err != nil {
		return res, err
	}
	if err := setMem(9, &memLock); err != nil {
		return res, err
	}
	if err := setU64(10, &smod); err != nil {
		return res, err
	}
	if err := setU64(11, &sinv); err != nil {
		return res, err
	}
	if err := setU64(12, &slim); err != nil {
		return res, err
	}
	if err := setU32(13, &stz); err != nil {
		return res, err
	}
	if err := setMem(14, &memShareCnt); err != nil {
		return res, err
	}
	if err := setMem(15, &memShares); err != nil {
		return res, err
	}
	if err := setU32(16, &ms); err != nil {
		return res, err
	}

	npt := oclBlake3NoncesPerThread
	if npt < 1 {
		npt = 1
	}
	global := C.size_t(oclDivUp(int(count), npt))
	if global < 1 {
		global = 1
	}
	errN = C.clEnqueueNDRangeKernel(a.queue, a.kernelB3, 1, nil, &global, nil, 0, nil, nil)
	if errN != C.CL_SUCCESS {
		return res, fmt.Errorf("gpupoh ocl: NDRangeKernel blake3 %d", int(errN))
	}
	errN = C.clFinish(a.queue)
	if errN != C.CL_SUCCESS {
		return res, fmt.Errorf("gpupoh ocl: Finish blake3 %d", int(errN))
	}
	kernelSec := time.Since(t0).Seconds()
	recordOCLKernelDuration(kernelSec)
	if os.Getenv("HACKME_OPENCL_VERBOSE") == "1" {
		ghs := float64(count) / kernelSec / 1e9
		hotlog.VerboseStderrf("gpupoh: opencl blake3 search count=%d elapsed=%s ~%.2f GH/s",
			count, time.Duration(kernelSec*float64(time.Second)).Round(time.Millisecond), ghs)
	}

	errN = C.clEnqueueReadBuffer(a.queue, a.memOut, C.CL_TRUE, 0, C.size_t(8), unsafe.Pointer(&out), 0, nil, nil)
	if errN != C.CL_SUCCESS {
		return res, fmt.Errorf("gpupoh ocl: ReadBuffer blake3 %d", int(errN))
	}
	if shareMod > 0 && a.memShareCnt != nil && a.memShares != nil && maxShares > 0 {
		var nShare uint32
		errN = C.clEnqueueReadBuffer(a.queue, a.memShareCnt, C.CL_TRUE, 0, C.size_t(4), unsafe.Pointer(&nShare), 0, nil, nil)
		if errN != C.CL_SUCCESS {
			return res, fmt.Errorf("gpupoh ocl: ReadBuffer share_count %d", int(errN))
		}
		ns := int(nShare)
		if ns > maxShares {
			ns = maxShares
		}
		if ns > 0 {
			buf := make([]uint64, ns)
			errN = C.clEnqueueReadBuffer(a.queue, a.memShares, C.CL_TRUE, 0, C.size_t(ns*8), unsafe.Pointer(&buf[0]), 0, nil, nil)
			if errN != C.CL_SUCCESS {
				return res, fmt.Errorf("gpupoh ocl: ReadBuffer shares %d", int(errN))
			}
			res.Shares = buf
		}
	}
	maxU := ^uint64(0)
	if out != maxU {
		res.Found = true
		res.Nonce = out
	}
	res.Scanned = count
	return res, nil
}

func (a *oclAccel) Search(ctx context.Context, base, count, mod uint64) (found bool, nonce uint64, err error) {
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
	t0 := time.Now()

	var out uint64 = ^uint64(0)
	errN := C.clEnqueueWriteBuffer(a.queue, a.memOut, C.CL_TRUE, 0, C.size_t(8), unsafe.Pointer(&out), 0, nil, nil)
	if errN != C.CL_SUCCESS {
		return false, 0, fmt.Errorf("gpupoh ocl: EnqueueWriteBuffer init %d", int(errN))
	}
	var lockZero C.cl_int
	errN = C.clEnqueueWriteBuffer(a.queue, a.memLock, C.CL_TRUE, 0, C.size_t(4), unsafe.Pointer(&lockZero), 0, nil, nil)
	if errN != C.CL_SUCCESS {
		return false, 0, fmt.Errorf("gpupoh ocl: EnqueueWriteBuffer lock %d", int(errN))
	}

	b := C.cl_ulong(base)
	c := C.cl_ulong(count)
	m := C.cl_ulong(mod)
	errN = C.clSetKernelArg(a.kernel, 0, C.size_t(unsafe.Sizeof(b)), unsafe.Pointer(&b))
	if errN != C.CL_SUCCESS {
		return false, 0, fmt.Errorf("gpupoh ocl: SetKernelArg0 %d", int(errN))
	}
	errN = C.clSetKernelArg(a.kernel, 1, C.size_t(unsafe.Sizeof(c)), unsafe.Pointer(&c))
	if errN != C.CL_SUCCESS {
		return false, 0, fmt.Errorf("gpupoh ocl: SetKernelArg1 %d", int(errN))
	}
	errN = C.clSetKernelArg(a.kernel, 2, C.size_t(unsafe.Sizeof(m)), unsafe.Pointer(&m))
	if errN != C.CL_SUCCESS {
		return false, 0, fmt.Errorf("gpupoh ocl: SetKernelArg2 %d", int(errN))
	}
	errN = C.clSetKernelArg(a.kernel, 3, C.size_t(unsafe.Sizeof(a.memOut)), unsafe.Pointer(&a.memOut))
	if errN != C.CL_SUCCESS {
		return false, 0, fmt.Errorf("gpupoh ocl: SetKernelArg3 %d", int(errN))
	}
	errN = C.clSetKernelArg(a.kernel, 4, C.size_t(unsafe.Sizeof(a.memLock)), unsafe.Pointer(&a.memLock))
	if errN != C.CL_SUCCESS {
		return false, 0, fmt.Errorf("gpupoh ocl: SetKernelArg4 %d", int(errN))
	}

	global := C.size_t(count)
	errN = C.clEnqueueNDRangeKernel(a.queue, a.kernel, 1, nil, &global, nil, 0, nil, nil)
	if errN != C.CL_SUCCESS {
		return false, 0, fmt.Errorf("gpupoh ocl: NDRangeKernel %d", int(errN))
	}
	errN = C.clFinish(a.queue)
	if errN != C.CL_SUCCESS {
		return false, 0, fmt.Errorf("gpupoh ocl: Finish %d", int(errN))
	}
	kernelSec := time.Since(t0).Seconds()
	recordOCLKernelDuration(kernelSec)
	if os.Getenv("HACKME_OPENCL_VERBOSE") == "1" {
		ghs := float64(count) / kernelSec / 1e9
		hotlog.VerboseStderrf("gpupoh: opencl search count=%d elapsed=%s ~%.2f GH/s",
			count, time.Duration(kernelSec*float64(time.Second)).Round(time.Millisecond), ghs)
	}

	errN = C.clEnqueueReadBuffer(a.queue, a.memOut, C.CL_TRUE, 0, C.size_t(8), unsafe.Pointer(&out), 0, nil, nil)
	if errN != C.CL_SUCCESS {
		return false, 0, fmt.Errorf("gpupoh ocl: ReadBuffer %d", int(errN))
	}
	maxU := ^uint64(0)
	if out == maxU {
		return false, 0, nil
	}
	return true, out, nil
}

func oclDeviceName(dev C.cl_device_id) string {
	var buf [512]C.char
	var ret C.size_t
	if C.clGetDeviceInfo(dev, C.CL_DEVICE_NAME, C.size_t(len(buf)), unsafe.Pointer(&buf[0]), &ret) != C.CL_SUCCESS {
		return "OpenCL device"
	}
	return C.GoString(&buf[0])
}

func newOCLAccelerator(globalIndex int, dev C.cl_device_id) (Accelerator, error) {
	var err C.cl_int
	ctx := C.clCreateContext(nil, 1, &dev, nil, nil, &err)
	if ctx == nil || err != C.CL_SUCCESS {
		return nil, fmt.Errorf("gpupoh ocl: CreateContext %d", int(err))
	}
	queue := C.clCreateCommandQueue(ctx, dev, 0, &err)
	if queue == nil || err != C.CL_SUCCESS {
		_ = C.clReleaseContext(ctx)
		return nil, fmt.Errorf("gpupoh ocl: CreateCommandQueue %d", int(err))
	}
	csrc := C.CString(oclKernelSource)
	defer C.free(unsafe.Pointer(csrc))
	var buildLog *C.char
	prog := C.poh_build_program(ctx, dev, csrc, &buildLog)
	if buildLog != nil {
		defer C.free(unsafe.Pointer(buildLog))
	}
	if prog == nil {
		_ = C.clReleaseCommandQueue(queue)
		_ = C.clReleaseContext(ctx)
		msg := "gpupoh ocl: program build failed"
		if buildLog != nil {
			msg += ": " + C.GoString(buildLog)
		}
		return nil, errors.New(msg)
	}
	cname := C.CString("poh_search")
	kern := C.poh_create_kernel_named(prog, cname)
	C.free(unsafe.Pointer(cname))
	if kern == nil {
		_ = C.clReleaseProgram(prog)
		_ = C.clReleaseCommandQueue(queue)
		_ = C.clReleaseContext(ctx)
		return nil, errors.New("gpupoh ocl: CreateKernel poh_search")
	}
	cnameB3 := C.CString("poh_search_blake3")
	kernB3 := C.poh_create_kernel_named(prog, cnameB3)
	C.free(unsafe.Pointer(cnameB3))
	haveB3 := kernB3 != nil
	if !haveB3 {
		hotlog.Stderrf("gpupoh: opencl blake3 kernel unavailable; host Blake3Search fallback")
	}

	memOut := C.clCreateBuffer(ctx, C.CL_MEM_READ_WRITE, C.size_t(8), nil, &err)
	if memOut == nil || err != C.CL_SUCCESS {
		if kernB3 != nil {
			_ = C.clReleaseKernel(kernB3)
		}
		_ = C.clReleaseKernel(kern)
		_ = C.clReleaseProgram(prog)
		_ = C.clReleaseCommandQueue(queue)
		_ = C.clReleaseContext(ctx)
		return nil, fmt.Errorf("gpupoh ocl: CreateBuffer out %d", int(err))
	}
	memLock := C.clCreateBuffer(ctx, C.CL_MEM_READ_WRITE, C.size_t(4), nil, &err)
	if memLock == nil || err != C.CL_SUCCESS {
		_ = C.clReleaseMemObject(memOut)
		if kernB3 != nil {
			_ = C.clReleaseKernel(kernB3)
		}
		_ = C.clReleaseKernel(kern)
		_ = C.clReleaseProgram(prog)
		_ = C.clReleaseCommandQueue(queue)
		_ = C.clReleaseContext(ctx)
		return nil, fmt.Errorf("gpupoh ocl: CreateBuffer lock %d", int(err))
	}
	shareCap := DefaultMaxShares
	memShareCnt := C.clCreateBuffer(ctx, C.CL_MEM_READ_WRITE, C.size_t(4), nil, &err)
	if memShareCnt == nil || err != C.CL_SUCCESS {
		_ = C.clReleaseMemObject(memLock)
		_ = C.clReleaseMemObject(memOut)
		if kernB3 != nil {
			_ = C.clReleaseKernel(kernB3)
		}
		_ = C.clReleaseKernel(kern)
		_ = C.clReleaseProgram(prog)
		_ = C.clReleaseCommandQueue(queue)
		_ = C.clReleaseContext(ctx)
		return nil, fmt.Errorf("gpupoh ocl: CreateBuffer share_count %d", int(err))
	}
	memShares := C.clCreateBuffer(ctx, C.CL_MEM_READ_WRITE, C.size_t(shareCap*8), nil, &err)
	if memShares == nil || err != C.CL_SUCCESS {
		_ = C.clReleaseMemObject(memShareCnt)
		_ = C.clReleaseMemObject(memLock)
		_ = C.clReleaseMemObject(memOut)
		if kernB3 != nil {
			_ = C.clReleaseKernel(kernB3)
		}
		_ = C.clReleaseKernel(kern)
		_ = C.clReleaseProgram(prog)
		_ = C.clReleaseCommandQueue(queue)
		_ = C.clReleaseContext(ctx)
		return nil, fmt.Errorf("gpupoh ocl: CreateBuffer shares %d", int(err))
	}
	// Always keep a non-nil prefix buffer — OpenCL rejects NULL __global args even when plen=0.
	memPrefix := C.clCreateBuffer(ctx, C.CL_MEM_READ_ONLY, C.size_t(256), nil, &err)
	if memPrefix == nil || err != C.CL_SUCCESS {
		_ = C.clReleaseMemObject(memShares)
		_ = C.clReleaseMemObject(memShareCnt)
		_ = C.clReleaseMemObject(memLock)
		_ = C.clReleaseMemObject(memOut)
		if kernB3 != nil {
			_ = C.clReleaseKernel(kernB3)
		}
		_ = C.clReleaseKernel(kern)
		_ = C.clReleaseProgram(prog)
		_ = C.clReleaseCommandQueue(queue)
		_ = C.clReleaseContext(ctx)
		return nil, fmt.Errorf("gpupoh ocl: CreateBuffer prefix %d", int(err))
	}
	name := strings.TrimSpace(oclDeviceName(dev))
	if name == "" {
		name = fmt.Sprintf("OpenCL device %d", globalIndex)
	}
	return &oclAccel{
		devID:       globalIndex,
		devName:     name,
		ctx:         ctx,
		queue:       queue,
		program:     prog,
		kernel:      kern,
		kernelB3:    kernB3,
		memOut:      memOut,
		memLock:     memLock,
		memPrefix:   memPrefix,
		memShareCnt: memShareCnt,
		memShares:   memShares,
		shareCap:    shareCap,
		prefixN:     256,
		haveB3:      haveB3,
	}, nil
}

// oclListGPUClDevices returns GPU-class OpenCL device IDs from all platforms (stable order).
func oclListGPUClDevices() []C.cl_device_id {
	var nplat C.cl_uint
	if C.clGetPlatformIDs(0, nil, &nplat) != C.CL_SUCCESS || nplat == 0 {
		return nil
	}
	plats := make([]C.cl_platform_id, nplat)
	if C.clGetPlatformIDs(nplat, &plats[0], nil) != C.CL_SUCCESS {
		return nil
	}
	var devices []C.cl_device_id
	for _, plat := range plats {
		var ndev C.cl_uint
		if C.clGetDeviceIDs(plat, C.CL_DEVICE_TYPE_GPU|C.CL_DEVICE_TYPE_ACCELERATOR, 0, nil, &ndev) != C.CL_SUCCESS || ndev == 0 {
			continue
		}
		devs := make([]C.cl_device_id, ndev)
		if C.clGetDeviceIDs(plat, C.CL_DEVICE_TYPE_GPU|C.CL_DEVICE_TYPE_ACCELERATOR, ndev, &devs[0], nil) != C.CL_SUCCESS {
			continue
		}
		for _, d := range devs {
			devices = append(devices, d)
		}
	}
	return devices
}

// OpenCLAcceleratorInitDiagnostics runs a full kernel build per GPU device (same as mining init).
// Use for operator probes when DiscoverAccelerators returns empty (driver / rusticl issues).
func OpenCLAcceleratorInitDiagnostics() []string {
	devices := oclListGPUClDevices()
	var lines []string
	for i, dev := range devices {
		name := strings.TrimSpace(oclDeviceName(dev))
		if name == "" {
			name = fmt.Sprintf("OpenCL device %d", i)
		}
		acc, err := newOCLAccelerator(i, dev)
		if err != nil {
			lines = append(lines, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		oa, ok := acc.(*oclAccel)
		b3 := "blake3=missing"
		if ok && oa.haveB3 {
			b3 = "blake3=ok"
		}
		_ = acc.Close()
		lines = append(lines, fmt.Sprintf("%s: kernel init ok (%s)", name, b3))
	}
	return lines
}

// tryOpenCLAccelerators enumerates GPU-class devices on all platforms.
func tryOpenCLAccelerators() ([]Accelerator, error) {
	devices := oclListGPUClDevices()
	if len(devices) == 0 {
		return nil, nil
	}
	var out []Accelerator
	for i, dev := range devices {
		a, err := newOCLAccelerator(i, dev)
		if err != nil {
			// Some OpenCL devices/drivers are flaky; skip failed ones
			// and keep the devices that can actually execute kernels.
			continue
		}
		out = append(out, a)
	}
	return out, nil
}

// listOpenCLGPUDevices returns GPU-class OpenCL devices (names only).
func listOpenCLGPUDevices() []GPUDeviceInfo {
	devices := oclListGPUClDevices()
	out := make([]GPUDeviceInfo, 0, len(devices))
	for i, dev := range devices {
		name := strings.TrimSpace(oclDeviceName(dev))
		if name == "" {
			name = fmt.Sprintf("OpenCL device %d", i)
		}
		out = append(out, GPUDeviceInfo{Index: i, Name: name, Backend: "opencl"})
	}
	return out
}
