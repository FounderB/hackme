package hunt

import (
	"bytes"
	"io"
	"sync"
	"testing"
)

func TestSyncWriterConcurrent(t *testing.T) {
	var buf bytes.Buffer
	w := newSyncWriter(&buf)
	const n = 64
	var wg sync.WaitGroup
	wg.Add(n * 2)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, _ = io.WriteString(w, "out")
		}()
		go func() {
			defer wg.Done()
			_, _ = io.WriteString(w, "err")
		}()
	}
	wg.Wait()
	got := buf.Len()
	want := n * (3 + 3)
	if got != want {
		t.Fatalf("len=%d want %d (race corruption?)", got, want)
	}
}
