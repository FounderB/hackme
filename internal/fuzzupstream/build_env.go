package fuzzupstream

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

// ScrubBuildEnv drops admin/settlement/API tokens from a compiler/cargo env so
// customer build.rs / proc-macros cannot inherit hub secrets (finding #5).
// Mirrors hunt.scrubIsolateEnv deny lists.
func ScrubBuildEnv(env []string) []string {
	denyExact := map[string]bool{
		"http_proxy": true, "https_proxy": true, "HTTP_PROXY": true, "HTTPS_PROXY": true,
		"ALL_PROXY": true, "all_proxy": true, "GIT_ASKPASS": true, "SSH_AUTH_SOCK": true,
		"AWS_SECRET_ACCESS_KEY": true, "AWS_ACCESS_KEY_ID": true, "AWS_SESSION_TOKEN": true,
		"COORD_TOKEN": true, "GITHUB_TOKEN": true, "GH_TOKEN": true,
		"HACKME_ADMIN_TOKEN": true, "HACKME_POOL_COORDINATOR_TOKEN": true,
		"HACKME_MINER_ED25519_SEED_HEX": true, "HACKME_COORDINATOR_ADMIN_TOKEN": true,
		"HACKME_SETTLEMENT_ADMIN_TOKEN": true, "OPENAI_API_KEY": true, "ANTHROPIC_API_KEY": true,
	}
	denyPrefix := []string{
		"HACKME_MINER_", "HACKME_POOL_", "HACKME_SETTLE", "AWS_", "GITHUB_",
	}
	out := make([]string, 0, len(env))
	for _, e := range env {
		key, _, _ := strings.Cut(e, "=")
		if denyExact[key] {
			continue
		}
		skip := false
		for _, p := range denyPrefix {
			if strings.HasPrefix(key, p) {
				if key == "HACKME_POOL_COORDINATOR_URL" || key == "HACKME_POOL_SEED_FROM_RESEARCH" {
					break
				}
				up := strings.ToUpper(key)
				if strings.Contains(up, "TOKEN") || strings.Contains(up, "SECRET") ||
					strings.Contains(up, "SEED") || strings.Contains(up, "KEY") {
					skip = true
					break
				}
			}
		}
		if skip {
			continue
		}
		out = append(out, e)
	}
	return out
}

// BuildCmdRunner optionally replaces bare compiler/cargo exec for catalog builds.
// Hunt sets this to hunt.RunBuildIsolated so hub builds inherit bwrap + scrubbed env.
type BuildCmdRunner func(ctx context.Context, bin string, args []string, env []string, dir string, extraBind []string, wall time.Duration, stdout, stderr io.Writer) error

// BuildRunner is the active catalog build executor (default: scrubbed bare exec).
var BuildRunner BuildCmdRunner = defaultBuildRunner

func defaultBuildRunner(ctx context.Context, bin string, args []string, env []string, dir string, _ []string, wall time.Duration, stdout, stderr io.Writer) error {
	if wall > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, wall)
		defer cancel()
	}
	if !filepath.IsAbs(bin) {
		if p, err := exec.LookPath(bin); err == nil {
			bin = p
		}
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if len(env) == 0 {
		env = os.Environ()
	}
	cmd.Env = ScrubBuildEnv(env)
	if stdout != nil {
		cmd.Stdout = stdout
	}
	if stderr != nil {
		cmd.Stderr = stderr
	}
	return cmd.Run()
}

// runBuildCmd executes a compiler/cargo step via BuildRunner (scrubbed; optional cage).
func runBuildCmd(ctx context.Context, bin string, args []string, env []string, dir string, binds []string, wall time.Duration) (stderr string, err error) {
	var buf bytes.Buffer
	runner := BuildRunner
	if runner == nil {
		runner = defaultBuildRunner
	}
	err = runner(ctx, bin, args, env, dir, binds, wall, nil, &buf)
	if err != nil {
		return buf.String(), err
	}
	return buf.String(), nil
}

// BuildSandboxOptOutHint documents desk escape for operators (also in hunt docs).
func BuildSandboxOptOutHint() string {
	return "HACKME_HUNT_BUILD_SANDBOX=0 disables bwrap for local desk harness builds (env still scrubbed)"
}

// ErrBuildSandbox documents a missing cage when STRICT builds are required.
var ErrBuildSandbox = fmt.Errorf("fuzzupstream: build sandbox required")
