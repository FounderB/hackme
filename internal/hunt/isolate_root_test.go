package hunt

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRefuseRootIsolated(t *testing.T) {
	t.Setenv("HACKME_HUNT_ALLOW_ROOT", "")
	prev := isolateEffectiveUID
	t.Cleanup(func() { isolateEffectiveUID = prev })

	isolateEffectiveUID = func() int { return 1000 }
	if err := refuseRootIsolated(false); err != nil {
		t.Fatalf("non-root must pass: %v", err)
	}

	isolateEffectiveUID = func() int { return 0 }
	if err := refuseRootIsolated(false); err == nil || !strings.Contains(err.Error(), "refusing root") {
		t.Fatalf("root without allow must fail, got %v", err)
	}
	if err := refuseRootIsolated(true); err != nil {
		t.Fatalf("AllowBare must opt out: %v", err)
	}
	t.Setenv("HACKME_HUNT_ALLOW_ROOT", "1")
	if err := refuseRootIsolated(false); err != nil {
		t.Fatalf("ALLOW_ROOT=1 must pass: %v", err)
	}
}

func TestRunIsolatedRefusesRootBeforeBwrap(t *testing.T) {
	t.Setenv("HACKME_HUNT_ALLOW_ROOT", "")
	t.Setenv("HACKME_HUNT_ISOLATE_STRICT", "1")
	prev := isolateEffectiveUID
	t.Cleanup(func() { isolateEffectiveUID = prev })
	isolateEffectiveUID = func() int { return 0 }

	err := RunIsolated(context.Background(), IsolateOpts{
		Bin:  "/bin/true",
		Wall: time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "refusing root") {
		t.Fatalf("bwrap path must refuse root before exec, got %v", err)
	}
}
