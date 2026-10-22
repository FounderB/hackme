package main

import "strings"

// testTipHash is a non-empty tip for unit tests that exercise found submit.
// Production found path is fail-closed without tip_hash (Blake3 / formula gate).
const testTipHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func withTestTip(m *workManager) *workManager {
	if m == nil {
		return nil
	}
	if strings.TrimSpace(m.tipHash) == "" {
		m.tipHash = testTipHash
	}
	if m.tipHeight == 0 {
		m.tipHeight = 1
	}
	return m
}
