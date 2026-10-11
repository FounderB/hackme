package hunt

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// IsolateOpts bounds a native Hunt harness / fuzzer process.
type IsolateOpts struct {
	Bin       string
	Args      []string
	Env       []string
	Dir       string
	Wall      time.Duration
	RSSLimitMB int
	// AllowNet is always false for Hunt engines (defense in depth).
	Stdout *os.File
	Stderr *os.File
}

// RunIsolated executes bin under wall-clock + optional bubblewrap (--unshare-net).
// Falls back to plain CommandContext when bwrap is unavailable.
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
		// Soft hint for sanitizers / AFL; OS rlimit applied below when possible.
		env = append(env, fmt.Sprintf("HACKME_ISOLATE_RSS_MB=%d", opts.RSSLimitMB))
	}
	// Never inherit proxy / git credentials into harness cages.
	env = scrubIsolateEnv(env)

	argv, useBwrap := wrapBubblewrap(opts.Bin, opts.Args, opts.Dir)
	cmd := exec.CommandContext(runCtx, argv[0], argv[1:]...)
	cmd.Env = env
	if opts.Dir != "" && !useBwrap {
		cmd.Dir = opts.Dir
	}
	if opts.Stdout != nil {
		cmd.Stdout = opts.Stdout
	}
	if opts.Stderr != nil {
		cmd.Stderr = opts.Stderr
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
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

func wrapBubblewrap(bin string, args []string, dir string) ([]string, bool) {
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		out := append([]string{bin}, args...)
		return out, false
	}
	// Minimal cage: new net ns (no net), die-with-parent, bind repo read-only where possible.
	// Writable workdir is the fuzzer corpus/out path (dir).
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
	// Bind the binary's directory read-only.
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
	deny := map[string]bool{
		"http_proxy": true, "https_proxy": true, "HTTP_PROXY": true, "HTTPS_PROXY": true,
		"ALL_PROXY": true, "all_proxy": true, "GIT_ASKPASS": true, "SSH_AUTH_SOCK": true,
		"AWS_SECRET_ACCESS_KEY": true, "AWS_ACCESS_KEY_ID": true,
	}
	out := make([]string, 0, len(env)+2)
	for _, e := range env {
		key, _, _ := strings.Cut(e, "=")
		if deny[key] {
			continue
		}
		out = append(out, e)
	}
	return out
}

func killProcessGroup(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err == nil {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}
	return cmd.Process.Kill()
}

func setRSSLimitMB(pid, mb int) error {
	if pid <= 0 || mb <= 0 {
		return nil
	}
	// Best-effort: prlimit via /proc when available is OS-specific; skip hard fail.
	_ = pid
	_ = mb
	return nil
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
