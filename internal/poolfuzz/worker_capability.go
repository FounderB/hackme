package poolfuzz

import (
	"os"
	"strconv"
	"strings"
)

// HuntHarnessLibFuzzerOneshot is the claim capability for workers that run
// cargo-fuzz / libFuzzer binaries via temp-file + -runs=1 (not stdin hang).
const HuntHarnessLibFuzzerOneshot = "libfuzzer_oneshot"

// HuntHarnessCapable reports whether a worker can safely execute Hunt ASAN harnesses
// that look like libFuzzer/cargo-fuzz (Phase A fleet gate).
func HuntHarnessCapable(exec string) bool {
	return strings.EqualFold(strings.TrimSpace(exec), HuntHarnessLibFuzzerOneshot)
}

// MinWorkerVersion returns HACKME_POOL_MIN_WORKER_VERSION (empty = no floor).
func MinWorkerVersion() string {
	return strings.TrimSpace(os.Getenv("HACKME_POOL_MIN_WORKER_VERSION"))
}

// WorkerVersionAllowed is true when min is unset, or got is a valid version >= min.
// Empty got fails closed when min is set (old binaries omit the field).
func WorkerVersionAllowed(got, min string) bool {
	min = strings.TrimSpace(min)
	if min == "" {
		return true
	}
	got = strings.TrimSpace(got)
	if got == "" {
		return false
	}
	return versionCmpLoose(got, min) >= 0
}

// versionCmpLoose compares dotted versions with optional -rc / -pre suffix.
// Returns -1, 0, 1 like strings.Compare for numeric major.minor.patch first.
func versionCmpLoose(a, b string) int {
	a = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(a)), "v")
	b = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(b)), "v")
	as, asuf := splitVersionCore(a)
	bs, bsuf := splitVersionCore(b)
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		var ai, bi int
		if i < len(as) {
			ai = as[i]
		}
		if i < len(bs) {
			bi = bs[i]
		}
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}
	// No suffix (release) > any pre-release suffix.
	if asuf == "" && bsuf != "" {
		return 1
	}
	if asuf != "" && bsuf == "" {
		return -1
	}
	return strings.Compare(asuf, bsuf)
}

func splitVersionCore(v string) (nums []int, suffix string) {
	core := v
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		core = v[:i]
		suffix = v[i:]
	}
	for _, p := range strings.Split(core, ".") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Strip trailing non-digits in a segment (e.g. "0rc1" → 0).
		j := 0
		for j < len(p) && p[j] >= '0' && p[j] <= '9' {
			j++
		}
		if j == 0 {
			continue
		}
		n, _ := strconv.Atoi(p[:j])
		nums = append(nums, n)
	}
	return nums, suffix
}
