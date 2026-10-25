package main

import (
	"strings"
	"unicode"
)

// sanitizeLogToken strips control chars / newlines so worker IDs cannot inject log lines
// (code-scanning go/log-injection on accrual persist/revoke).
// Prefer strings.ReplaceAll for newlines so CodeQL recognizes the barrier.
func sanitizeLogToken(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "?"
	}
	s = strings.ReplaceAll(s, "\n", "_")
	s = strings.ReplaceAll(s, "\r", "_")
	s = strings.ReplaceAll(s, "\t", "_")
	var b strings.Builder
	b.Grow(len(s))
	n := 0
	for _, r := range s {
		if n >= 128 {
			b.WriteString("…")
			break
		}
		if !unicode.IsPrint(r) {
			b.WriteByte('_')
			n++
			continue
		}
		b.WriteRune(r)
		n++
	}
	return b.String()
}
