package hunt

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// BuildSandboxEnabled reports whether harness compiles (clang/cargo/afl-cc) run
// under RunIsolated (bwrap + scrubbed env). Default ON.
//
// Desk opt-out when userns/bwrap breaks local builds:
//
//	HACKME_HUNT_BUILD_SANDBOX=0
//
// Token scrubbing still applies when the cage is off (ScrubBuildEnv / scrubIsolateEnv).
func BuildSandboxEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("HACKME_HUNT_BUILD_SANDBOX")))
	if v == "" {
		return true
	}
	switch v {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// BuildIsolateOpts configures a sandboxed compiler / cargo / afl-cc invocation.
type BuildIsolateOpts struct {
	Bin         string
	Args        []string
	Dir         string
	Env         []string // full env before scrub; empty → os.Environ()
	ExtraBind   []string // RW binds (pin, tmp, cargo target, …)
	ExtraRoBind []string
	Wall        time.Duration
	Stdout      io.Writer
	Stderr      io.Writer
}

// RunBuildIsolated runs a harness build under RunIsolated when BuildSandboxEnabled.
// Env is always scrubbed. With sandbox off (desk), scrubbed bare exec is used.
func RunBuildIsolated(ctx context.Context, opts BuildIsolateOpts) error {
	if strings.TrimSpace(opts.Bin) == "" {
		return fmt.Errorf("hunt build isolate: empty bin")
	}
	// Resolve PATH names before bwrap (Abs("cargo") would wrongly become $PWD/cargo).
	if !filepath.IsAbs(opts.Bin) {
		if p, err := exec.LookPath(opts.Bin); err == nil {
			opts.Bin = p
		}
	}
	env := opts.Env
	if len(env) == 0 {
		env = os.Environ()
	}
	env = scrubIsolateEnv(env)
	wall := opts.Wall
	if wall <= 0 {
		wall = 180 * time.Second
	}
	extraRo := append([]string{}, opts.ExtraRoBind...)
	extraRo = append(extraRo, buildToolchainRoBinds()...)
	extraBind := append([]string{}, opts.ExtraBind...)
	extraBind = append(extraBind, buildHomeBinds()...)

	if !BuildSandboxEnabled() {
		return runBareBuild(ctx, opts, env, wall)
	}
	// Prefer cage; AllowBare only when STRICT opted out (desk/broken userns).
	return RunIsolated(ctx, IsolateOpts{
		Bin:         opts.Bin,
		Args:        opts.Args,
		Dir:         opts.Dir,
		Env:         env,
		Wall:        wall,
		Stdout:      opts.Stdout,
		Stderr:      opts.Stderr,
		ExtraBind:   extraBind,
		ExtraRoBind: extraRo,
		AllowBare:   !IsolateStrict(),
	})
}

func runBareBuild(ctx context.Context, opts BuildIsolateOpts, env []string, wall time.Duration) error {
	return RunIsolated(ctx, IsolateOpts{
		Bin:       opts.Bin,
		Args:      opts.Args,
		Dir:       opts.Dir,
		Env:       env,
		Wall:      wall,
		Stdout:    opts.Stdout,
		Stderr:    opts.Stderr,
		AllowBare: true,
	})
}

func buildToolchainRoBinds() []string {
	out := []string{"/opt/hackme"}
	if p := strings.TrimSpace(os.Getenv("HACKME_PREFIX")); p != "" {
		out = append(out, p)
	}
	if home := strings.TrimSpace(os.Getenv("HOME")); home != "" {
		out = append(out, filepath.Join(home, ".local/share/hackme/toolchains"))
		out = append(out, filepath.Join(home, ".rustup"))
	}
	return out
}

func buildHomeBinds() []string {
	home := strings.TrimSpace(os.Getenv("HOME"))
	if home == "" {
		return nil
	}
	// Cargo/rustc write under ~/.cargo; clang needs nothing here but cargo fuzz does.
	return []string{
		filepath.Join(home, ".cargo"),
		filepath.Join(home, ".cache"),
	}
}

// uniqueBindPaths dedupes absolute paths for ExtraBind.
func uniqueBindPaths(paths ...string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		abs, err := filepath.Abs(p)
		if err != nil || abs == "" || abs == "/" {
			continue
		}
		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}
		out = append(out, abs)
	}
	return out
}

// runCompilerBuild is a convenience for clang/afl-cc with stderr capture.
func runCompilerBuild(ctx context.Context, bin string, args []string, binds []string, wall time.Duration) (stderr string, err error) {
	var buf bytes.Buffer
	err = RunBuildIsolated(ctx, BuildIsolateOpts{
		Bin:       bin,
		Args:      args,
		ExtraBind: uniqueBindPaths(binds...),
		Wall:      wall,
		Stderr:    &buf,
	})
	return buf.String(), err
}
