package hunt

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// IsolateOpts bounds a native Hunt harness / fuzzer process.
type IsolateOpts struct {
	Bin        string
	Args       []string
	Env        []string
	Dir        string
	Wall       time.Duration
	RSSLimitMB int
	Stdin      io.Reader
	Stdout     *os.File
	Stderr     *os.File
	// ExtraBind / ExtraRoBind are additional paths for the bwrap cage (corpus, stdin driver).
	ExtraBind   []string
	ExtraRoBind []string
	// AllowBare when true permits plain exec if bwrap is missing (tests / showmap helpers).
	AllowBare bool
}

// IsolateRequired reports whether coverage-engine shards should prefer bwrap.
// Default ON (HACKME_HUNT_ISOLATE_REQUIRE empty/1). Set 0 to skip bwrap entirely.
// Launch failures may bare-retry only when IsolateStrict is opted out.
func IsolateRequired() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("HACKME_HUNT_ISOLATE_REQUIRE")))
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

// IsolateStrict refuses bare exec when bwrap is missing or fails to start.
// Default ON — miners must not fall through to unsandboxed foreign harnesses.
// Opt out only on locked-down operator hosts: HACKME_HUNT_ISOLATE_STRICT=0.
func IsolateStrict() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("HACKME_HUNT_ISOLATE_STRICT")))
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

// BubblewrapAvailable reports whether bwrap is on PATH.
func BubblewrapAvailable() bool {
	_, err := exec.LookPath("bwrap")
	return err == nil
}

// RunIsolated executes bin under wall-clock + bubblewrap (--unshare-net) when available.
// When IsolateRequired and bwrap is missing (and AllowBare is false), returns an error
// instead of falling through to bare exec (M1 fail-closed for paid Hunt).
func RunIsolated(ctx context.Context, opts IsolateOpts) error {
	if strings.TrimSpace(opts.Bin) == "" {
		return fmt.Errorf("hunt isolate: empty bin")
	}
	wall := opts.Wall
	if wall <= 0 {
		wall = 60 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, wall+15*time.Second)
	defer cancel()

	env := append([]string{}, opts.Env...)
	if opts.RSSLimitMB > 0 {
		env = append(env, fmt.Sprintf("HACKME_ISOLATE_RSS_MB=%d", opts.RSSLimitMB))
	}
	env = scrubIsolateEnv(env)

	// AllowBare is for showmap/unit helpers on trusted local bins — never cage them.
	if opts.AllowBare {
		return runBareIsolated(runCtx, opts, env, wall)
	}

	argv, useBwrap := wrapBubblewrap(opts.Bin, opts.Args, opts.Dir, opts.ExtraBind, opts.ExtraRoBind)
	if useBwrap && !IsolateRequired() {
		// Explicit escape (hub/desktop): skip bwrap when userns cages are denied.
		useBwrap = false
		argv = append([]string{opts.Bin}, opts.Args...)
	}
	if !useBwrap {
		if IsolateStrict() && !opts.AllowBare {
			return fmt.Errorf("hunt isolate: bwrap required (HACKME_HUNT_ISOLATE_STRICT)")
		}
		// Missing bwrap with REQUIRE=1 but not STRICT → bare (fleet usability).
		return runBareIsolated(runCtx, opts, env, wall)
	}
	cmd := exec.CommandContext(runCtx, argv[0], argv[1:]...)
	cmd.Env = env
	if opts.Stdin != nil {
		cmd.Stdin = opts.Stdin
	}
	if opts.Stdout != nil {
		cmd.Stdout = opts.Stdout
	}
	if opts.Stderr != nil {
		cmd.Stderr = opts.Stderr
	}
	setProcessGroup(cmd)
	// Bare fallback only if bwrap fails to *launch*. Once started, never unsandbox-retry
	// (harness stderr must not look like a cage failure and drop the sandbox).
	if err := cmd.Start(); err != nil {
		if bwrapCageFailed(err, "") && (!IsolateStrict() || opts.AllowBare) {
			return runBareIsolated(runCtx, opts, env, wall)
		}
		if IsolateStrict() && !opts.AllowBare {
			return fmt.Errorf("hunt isolate: bwrap required (HACKME_HUNT_ISOLATE_STRICT): %v", err)
		}
		return err
	}
	if opts.RSSLimitMB > 0 && cmd.Process != nil {
		_ = setRSSLimitMB(cmd.Process.Pid, opts.RSSLimitMB)
	}
	err := cmd.Wait()
	if runCtx.Err() == context.DeadlineExceeded {
		_ = killProcessGroup(cmd)
		return fmt.Errorf("hunt isolate: wall exceeded (%s)", wall)
	}
	return err
}

func bwrapCageFailed(err error, stderr string) bool {
	msg := strings.ToLower("")
	if err != nil {
		msg += err.Error() + " "
	}
	msg += stderr
	msg = strings.ToLower(msg)
	for _, needle := range []string{
		"namespace failed", "creating new namespace", "permission denied",
		"operation not permitted", "rtm_newaddr", "loopback",
		"bwrap:",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

// bareIsolatedCommand execs the harness with no_new_privs when setpriv(1) is available
// (Go 1.26+ removed syscall.SysProcAttr.NoNewPrivs).
func bareIsolatedCommand(ctx context.Context, bin string, args []string) *exec.Cmd {
	if sp, err := exec.LookPath("setpriv"); err == nil {
		argv := append([]string{"--nnp", "--", bin}, args...)
		return exec.CommandContext(ctx, sp, argv...)
	}
	return exec.CommandContext(ctx, bin, args...)
}

func runBareIsolated(runCtx context.Context, opts IsolateOpts, env []string, wall time.Duration) error {
	if os.Geteuid() == 0 && !opts.AllowBare {
		// Paid Hunt must not exec untrusted harnesses as root on the bare path.
		// AllowBare keeps showmap/unit helpers usable under CI root.
		v := strings.TrimSpace(strings.ToLower(os.Getenv("HACKME_HUNT_ALLOW_ROOT")))
		if v != "1" && v != "true" && v != "yes" && v != "on" {
			return fmt.Errorf("hunt isolate: refusing bare root exec (set HACKME_HUNT_ALLOW_ROOT=1 to override)")
		}
	}
	cmd := bareIsolatedCommand(runCtx, opts.Bin, opts.Args)
	cmd.Env = env
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}
	if opts.Stdin != nil {
		cmd.Stdin = opts.Stdin
	}
	if opts.Stdout != nil {
		cmd.Stdout = opts.Stdout
	}
	if opts.Stderr != nil {
		cmd.Stderr = opts.Stderr
	}
	setProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	if opts.RSSLimitMB > 0 && cmd.Process != nil {
		_ = setRSSLimitMB(cmd.Process.Pid, opts.RSSLimitMB)
	}
	err := cmd.Wait()
	if runCtx.Err() == context.DeadlineExceeded {
		_ = killProcessGroup(cmd)
		return fmt.Errorf("hunt isolate: wall exceeded (%s)", wall)
	}
	return err
}

func wrapBubblewrap(bin string, args []string, dir string, extraBind, extraRo []string) ([]string, bool) {
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		out := append([]string{bin}, args...)
		return out, false
	}
	argv := []string{
		bwrap,
		"--die-with-parent",
		"--unshare-net",
		"--unshare-pid",
		"--proc", "/proc",
		"--dev", "/dev",
		"--ro-bind", "/usr", "/usr",
		"--ro-bind", "/lib", "/lib",
		"--ro-bind-try", "/lib64", "/lib64",
		"--ro-bind-try", "/bin", "/bin",
		"--ro-bind-try", "/sbin", "/sbin",
		"--ro-bind-try", "/etc/ld.so.cache", "/etc/ld.so.cache",
		"--ro-bind-try", "/etc/alternatives", "/etc/alternatives",
		"--tmpfs", "/tmp",
	}
	if dir != "" {
		abs, err := filepath.Abs(dir)
		if err == nil {
			argv = append(argv, "--bind", abs, abs, "--chdir", abs)
		}
	}
	for _, p := range extraBind {
		if abs, err := filepath.Abs(strings.TrimSpace(p)); err == nil && abs != "" {
			argv = append(argv, "--bind", abs, abs)
		}
	}
	for _, p := range extraRo {
		if abs, err := filepath.Abs(strings.TrimSpace(p)); err == nil && abs != "" {
			argv = append(argv, "--ro-bind-try", abs, abs)
		}
	}
	if absBin, err := filepath.Abs(bin); err == nil {
		binDir := filepath.Dir(absBin)
		argv = append(argv, "--ro-bind", binDir, binDir)
		bin = absBin
	}
	argv = append(argv, "--", bin)
	argv = append(argv, args...)
	return argv, true
}

func scrubIsolateEnv(env []string) []string {
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
	out := make([]string, 0, len(env)+2)
	for _, e := range env {
		key, _, _ := strings.Cut(e, "=")
		if denyExact[key] {
			continue
		}
		skip := false
		for _, p := range denyPrefix {
			if strings.HasPrefix(key, p) {
				// Keep non-secret pool knobs that engines need (URLs are OK; tokens scrubbed above).
				if key == "HACKME_POOL_COORDINATOR_URL" || key == "HACKME_POOL_SEED_FROM_RESEARCH" {
					break
				}
				if strings.Contains(strings.ToUpper(key), "TOKEN") ||
					strings.Contains(strings.ToUpper(key), "SECRET") ||
					strings.Contains(strings.ToUpper(key), "SEED") ||
					strings.Contains(strings.ToUpper(key), "KEY") {
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

// LookAFLFuzz resolves afl-fuzz from PATH or hybrid bundle prefixes.
func LookAFLFuzz() (string, error) {
	if v := strings.TrimSpace(os.Getenv("HACKME_AFL_FUZZ")); v != "" {
		if st, err := os.Stat(v); err == nil && !st.IsDir() {
			return v, nil
		}
	}
	if p, err := exec.LookPath("afl-fuzz"); err == nil {
		return p, nil
	}
	prefixes := []string{
		strings.TrimSpace(os.Getenv("HACKME_PREFIX")),
		"/opt/hackme",
		filepath.Join(os.Getenv("HOME"), ".local/share/hackme/toolchains"),
	}
	for _, pref := range prefixes {
		if pref == "" {
			continue
		}
		for _, rel := range []string{
			"bin/afl-fuzz",
			"aflpp/bin/afl-fuzz",
			"toolchains/aflpp/bin/afl-fuzz",
		} {
			p := filepath.Join(pref, rel)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p, nil
			}
		}
	}
	return "", fmt.Errorf("afl-fuzz not found (install via scripts/ops/install_hunt_engines.sh)")
}

// AFLFuzzVersion returns a short version string for claim/submit reports.
func AFLFuzzVersion(bin string) string {
	if bin == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-h")
	out, _ := cmd.CombinedOutput()
	s := string(out)
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(strings.ToLower(line), "afl++") || strings.HasPrefix(line, "afl-fuzz") {
			if len(line) > 80 {
				return line[:80]
			}
			return line
		}
	}
	return "afl++"
}

// LibFuzzerVersionHint is a stable report label (clang-linked LF has no --version).
func LibFuzzerVersionHint() string {
	if v := strings.TrimSpace(os.Getenv("HACKME_LIBFUZZER_VERSION")); v != "" {
		return v
	}
	return "libfuzzer"
}
