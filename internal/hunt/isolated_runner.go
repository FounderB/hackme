package hunt

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"time"

	"hackme/internal/fuzzupstream"
)

// IsolatedHarnessRunner returns a fuzzupstream.HarnessRunner that executes under
// RunIsolated (bwrap when available; STRICT fail-closed). Used by coordinator
// Hunt prove / trim so customer harnesses never get a bare exec on the hub.
func IsolatedHarnessRunner() fuzzupstream.HarnessRunner {
	return func(ctx context.Context, bin string, args []string, env []string, stdin []byte, wall time.Duration) (string, error) {
		if wall <= 0 {
			wall = 3 * time.Second
		}
		var out bytes.Buffer
		var stdinR io.Reader
		if len(stdin) > 0 {
			stdinR = bytes.NewReader(stdin)
		}
		extraRo := []string{}
		if d := filepath.Dir(bin); d != "" && d != "." {
			extraRo = append(extraRo, d)
		}
		err := RunIsolated(ctx, IsolateOpts{
			Bin:         bin,
			Args:        args,
			Env:         env,
			Wall:        wall,
			Stdin:       stdinR,
			Stdout:      &out,
			Stderr:      &out,
			AllowBare:   false,
			ExtraRoBind: extraRo,
		})
		return out.String(), err
	}
}
