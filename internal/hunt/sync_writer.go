package hunt

import (
	"io"
	"sync"
)

// syncWriter serializes Write calls so stdout and stderr can safely share one
// destination (IsolatedHarnessRunner used to pass the same bytes.Buffer to both).
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func newSyncWriter(w io.Writer) *syncWriter {
	if w == nil {
		return nil
	}
	return &syncWriter{w: w}
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}
