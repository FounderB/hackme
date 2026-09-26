package fuzzengine

import "crypto/sha256"

// MutationDepthStats summarizes uniqueness / depth of a mutation sample set.
type MutationDepthStats struct {
	Samples       int     `json:"samples"`
	UniqueSHA256  int     `json:"unique_sha256"`
	UniqueLens    int     `json:"unique_lengths"`
	AvgLen        float64 `json:"avg_len"`
	MaxLen        int     `json:"max_len"`
	MinLen        int     `json:"min_len"`
	UniqueRatio   float64 `json:"unique_ratio"`
	HavocSamples  int     `json:"havoc_samples"`
	HavocUnique   int     `json:"havoc_unique"`
	HavocUniqueRt float64 `json:"havoc_unique_ratio"`
}

// MeasureMutationDepth runs deterministic mutations and reports uniqueness metrics.
// Useful for A/B engine calibration (not a substitute for ASAN exec/s).
func MeasureMutationDepth(base []byte, dict []byte, corpus [][]byte, samples int, maxLen int) MutationDepthStats {
	if samples < 1 {
		samples = 1
	}
	if maxLen <= 0 {
		maxLen = DefaultMaxInputBytesStd
	}
	if len(base) == 0 {
		base = []byte(`{"a":1}`)
	}
	seen := map[string]struct{}{}
	lens := map[int]struct{}{}
	havocSeen := map[string]struct{}{}
	totalLen := 0
	minL, maxL := int(^uint(0)>>1), 0
	havocN := 0
	for i := 0; i < samples; i++ {
		stage := MutationStage(i % (StageDeterministicMax + HavocOpModulo))
		salt := uint64(i)*0x9E3779B97F4A7C15 + 0xDEAD
		out := mutateBytesWithDict(base, stage, salt, maxLen, dict, corpus)
		sum := sha256.Sum256(out)
		key := string(sum[:])
		seen[key] = struct{}{}
		lens[len(out)] = struct{}{}
		totalLen += len(out)
		if len(out) < minL {
			minL = len(out)
		}
		if len(out) > maxL {
			maxL = len(out)
		}
		if int(stage) >= StageHavocBase {
			havocN++
			havocSeen[key] = struct{}{}
		}
	}
	st := MutationDepthStats{
		Samples:      samples,
		UniqueSHA256: len(seen),
		UniqueLens:   len(lens),
		AvgLen:       float64(totalLen) / float64(samples),
		MaxLen:       maxL,
		MinLen:       minL,
		HavocSamples: havocN,
		HavocUnique:  len(havocSeen),
	}
	if samples > 0 {
		st.UniqueRatio = float64(st.UniqueSHA256) / float64(samples)
	}
	if havocN > 0 {
		st.HavocUniqueRt = float64(st.HavocUnique) / float64(havocN)
	}
	if minL == int(^uint(0)>>1) {
		st.MinLen = 0
	}
	return st
}

// EngineABReport compares upstream baseline havoc vs current engine on the same inputs.
type EngineABReport struct {
	Baseline MutationDepthStats `json:"baseline"`
	Current  MutationDepthStats `json:"current"`
	// UniqueGainPct = (current.unique - baseline.unique) / baseline.unique * 100
	UniqueGainPct float64 `json:"unique_gain_pct"`
	// LensGainPct = unique length diversity improvement
	LensGainPct float64 `json:"lens_gain_pct"`
}

// CompareEngineAB runs the same sample grid against baseline (v2.0 upstream) and current (v2.8).
func CompareEngineAB(base []byte, dict []byte, corpus [][]byte, samples int, maxLen int) EngineABReport {
	if samples < 1 {
		samples = 1
	}
	if maxLen <= 0 {
		maxLen = DefaultMaxInputBytesStd
	}
	if len(base) == 0 {
		base = []byte(`{"a":1}`)
	}
	baseSeen := map[string]struct{}{}
	curSeen := map[string]struct{}{}
	baseLens := map[int]struct{}{}
	curLens := map[int]struct{}{}
	for i := 0; i < samples; i++ {
		// Havoc-only grid: deterministic bitflips are identical upstream vs current.
		stage := MutationStage(StageHavocBase + (i % HavocOpModulo))
		salt := uint64(i)*0x9E3779B97F4A7C15 + 0xDEAD
		bOut := mutateBytesBaseline(base, stage, salt, maxLen, dict)
		cOut := mutateBytesWithDict(base, stage, salt, maxLen, dict, corpus)
		bs := sha256.Sum256(bOut)
		cs := sha256.Sum256(cOut)
		baseSeen[string(bs[:])] = struct{}{}
		curSeen[string(cs[:])] = struct{}{}
		baseLens[len(bOut)] = struct{}{}
		curLens[len(cOut)] = struct{}{}
	}
	bl := MutationDepthStats{Samples: samples, UniqueSHA256: len(baseSeen), UniqueLens: len(baseLens)}
	cr := MutationDepthStats{Samples: samples, UniqueSHA256: len(curSeen), UniqueLens: len(curLens)}
	if samples > 0 {
		bl.UniqueRatio = float64(bl.UniqueSHA256) / float64(samples)
		cr.UniqueRatio = float64(cr.UniqueSHA256) / float64(samples)
	}
	rep := EngineABReport{Baseline: bl, Current: cr}
	if bl.UniqueSHA256 > 0 {
		rep.UniqueGainPct = (float64(cr.UniqueSHA256-bl.UniqueSHA256) / float64(bl.UniqueSHA256)) * 100
	}
	if bl.UniqueLens > 0 {
		rep.LensGainPct = (float64(cr.UniqueLens-bl.UniqueLens) / float64(bl.UniqueLens)) * 100
	}
	return rep
}

// MeasureMutationDepthMulti averages uniqueness across several format bases (local stress).
func MeasureMutationDepthMulti(samplesPerBase, maxLen int) MutationDepthStats {
	bases := [][]byte{
		[]byte(`{"a":1,"nested":{"b":[1,2,3],"tag":"<x/>"}}`),
		[]byte(`<root attr="1"><child>text</child></root>`),
		[]byte("GET /api?x=1 HTTP/1.1\r\nHost: t\r\n\r\n"),
		{0x00, 0x01, 0x02, 0xff, 0xfe, 0x7f, 0x80, 0x00, 0x10, 0x20},
		[]byte("%PDF-1.4\n1 0 obj<<>>endobj"),
	}
	dict := []byte(`"null""true""false""a""nested"{}[]<>`)
	corpus := [][]byte{
		[]byte(`{}`), []byte(`[1,2]`), []byte(`{"x":null}`),
		[]byte(`<root/>`), []byte("\x1f\x8b\x08\x00"),
	}
	agg := MutationDepthStats{}
	for _, base := range bases {
		st := MeasureMutationDepth(base, dict, corpus, samplesPerBase, maxLen)
		agg.Samples += st.Samples
		agg.UniqueSHA256 += st.UniqueSHA256
		agg.UniqueLens += st.UniqueLens
		agg.AvgLen += st.AvgLen
		if st.MaxLen > agg.MaxLen {
			agg.MaxLen = st.MaxLen
		}
		agg.HavocSamples += st.HavocSamples
		agg.HavocUnique += st.HavocUnique
	}
	n := len(bases)
	if n > 0 {
		agg.AvgLen /= float64(n)
	}
	if agg.Samples > 0 {
		agg.UniqueRatio = float64(agg.UniqueSHA256) / float64(agg.Samples)
	}
	if agg.HavocSamples > 0 {
		agg.HavocUniqueRt = float64(agg.HavocUnique) / float64(agg.HavocSamples)
	}
	return agg
}
