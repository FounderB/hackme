package logsafe

import "strings"

// ID strips control characters and truncates so operator logs cannot be forged
// via CR/LF/ANSI in worker IDs, campaign IDs, or similar identifiers.
// ReplaceAll of newlines is a CodeQL-recognized log-injection barrier.
func ID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "-"
	}
	s = strings.ReplaceAll(s, "\n", "_")
	s = strings.ReplaceAll(s, "\r", "_")
	s = strings.ReplaceAll(s, "\t", "_")
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r < 0x20, r == 0x7f:
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	out := b.String()
	const max = 160
	if len(out) > max {
		return out[:max] + "…"
	}
	return out
}

// Err sanitizes error text for logs (CodeQL log-injection barrier on %v err sinks).
func Err(err error) string {
	if err == nil {
		return "-"
	}
	return ID(err.Error())
}
