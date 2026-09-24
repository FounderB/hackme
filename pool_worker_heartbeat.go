package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"hackme/internal/pathsafe"
)

// poolWorkerHeartbeatStaleSec is how long submit activity may be silent before
// the in-process watchdog treats a still-alive worker as frozen (Windows DoS-class hang).
func poolWorkerHeartbeatStaleSec() int64 {
	sec := int64(180)
	if v := strings.TrimSpace(os.Getenv("HACKME_WORKER_HEARTBEAT_STALE_SEC")); v != "" {
		if x, err := strconv.ParseInt(v, 10, 64); err == nil && x >= 60 && x <= 1800 {
			sec = x
		}
	}
	return sec
}

// poolWorkerHeartbeatGraceSec skips freeze detection right after start (CUDA init / first claim).
func poolWorkerHeartbeatGraceSec() int64 {
	sec := int64(120)
	if v := strings.TrimSpace(os.Getenv("HACKME_WORKER_HEARTBEAT_GRACE_SEC")); v != "" {
		if x, err := strconv.ParseInt(v, 10, 64); err == nil && x >= 30 && x <= 900 {
			sec = x
		}
	}
	return sec
}

func sanitizeWorkerIDForNonce(workerID string) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, strings.TrimSpace(workerID))
	if safe == "" {
		return "worker"
	}
	return safe
}

// workerSubmitNonceHeartbeatUnix returns the newest mtime among miner_submit_nonce*.seq
// files for this worker (hybrid submit counter). 0 if none exist.
func workerSubmitNonceHeartbeatUnix(logDir, workerID string) int64 {
	logDir = strings.TrimSpace(logDir)
	if logDir == "" {
		return 0
	}
	safe := sanitizeWorkerIDForNonce(workerID)
	// Exact + ".gpuN" suffixes only — avoid prefix collisions (rig vs rig-extra).
	patterns := []string{
		filepath.Join(logDir, "miner_submit_nonce."+safe+".seq"),
		filepath.Join(logDir, "miner_submit_nonce."+safe+".gpu*.seq"),
	}
	var best int64
	seen := map[string]struct{}{}
	for _, pat := range patterns {
		matches, err := filepath.Glob(pat)
		if err != nil {
			continue
		}
		for _, p := range matches {
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			base := filepath.Base(p)
			if !workerNonceFilenameOwnsWorker(base, safe) {
				continue
			}
			// Confine Glob hits to logDir (CodeQL path-injection / symlink escape).
			safePath, ok := pathsafe.WithinRoot(logDir, p)
			if !ok {
				continue
			}
			fi, err := os.Stat(safePath)
			if err != nil || fi.IsDir() {
				continue
			}
			ux := fi.ModTime().Unix()
			if ux > best {
				best = ux
			}
		}
	}
	return best
}

// workerNonceFilenameOwnsWorker accepts miner_submit_nonce.<id>.seq and
// miner_submit_nonce.<id>.gpu<digits>.seq only.
func workerNonceFilenameOwnsWorker(base, safeID string) bool {
	prefix := "miner_submit_nonce." + safeID
	if base == prefix+".seq" {
		return true
	}
	rest, ok := strings.CutPrefix(base, prefix+".gpu")
	if !ok || !strings.HasSuffix(rest, ".seq") {
		return false
	}
	mid := strings.TrimSuffix(rest, ".seq")
	if mid == "" {
		return false
	}
	for _, r := range mid {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// workerLogSubmitHeartbeatUnix returns mtime of the newest worker log that still
// shows a recent "submit ok" line (or the log mtime if the file is the participant log
// attached to the managed subprocess). 0 if no usable signal.
func workerLogSubmitHeartbeatUnix(logDir string) int64 {
	logDir = strings.TrimSpace(logDir)
	if logDir == "" {
		return 0
	}
	candidates := []string{
		filepath.Join(logDir, "worker_participant.log"),
		latestWorkerpohWorkerLogPath(logDir),
		latestWorkerpohLogPath(logDir),
	}
	var best int64
	for _, p := range candidates {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		fi, err := os.Stat(p)
		if err != nil || fi.IsDir() {
			continue
		}
		ux := fi.ModTime().Unix()
		// Prefer logs that actually contain a submit ok (avoids mistaking a truncated
		// open-at-start for healthy mining when nonce files are missing).
		if workerLogHasSubmitOK(p) {
			if ux > best {
				best = ux
			}
			continue
		}
		// Still count mtime: a frozen writer stops updating the file — useful freeze signal
		// once grace has passed and we already saw submit ok earlier in the session.
		_ = ux
	}
	return best
}

func workerLogHasSubmitOK(path string) bool {
	lines, err := tailFileLastLines(path, 40, 64*1024)
	if err != nil {
		return false
	}
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(lines[i], "submit ok") {
			return true
		}
	}
	return false
}

// workerSubmitHeartbeatUnix picks the strongest submit-activity signal.
// If sinceUnix > 0, ignore signals older than this session start (stale nonce/log
// from a previous run must not look like a live heartbeat — that caused false
// post-restart freezes / restart loops).
func workerSubmitHeartbeatUnix(logDir, workerID string) int64 {
	return workerSubmitHeartbeatUnixSince(logDir, workerID, 0)
}

func workerSubmitHeartbeatUnixSince(logDir, workerID string, sinceUnix int64) int64 {
	nonce := workerSubmitNonceHeartbeatUnix(logDir, workerID)
	logUX := workerLogSubmitHeartbeatUnix(logDir)
	hb := nonce
	if logUX > hb {
		hb = logUX
	}
	if sinceUnix > 0 && hb > 0 && hb < sinceUnix {
		return 0
	}
	return hb
}

// workerHeartbeatNeedsRestart reports whether a managed (or detected) worker should be
// force-restarted because submit activity went silent while the process may still be alive.
//
// restart=false during grace after startedAt, or when staleSec/graceSec invalid.
func workerHeartbeatNeedsRestart(logDir, workerID string, startedAt, nowUnix, staleSec, graceSec int64) (bool, string) {
	if nowUnix <= 0 {
		nowUnix = time.Now().Unix()
	}
	if staleSec < 60 {
		staleSec = 60
	}
	if graceSec < 0 {
		graceSec = 0
	}
	if startedAt > 0 && nowUnix-startedAt < graceSec {
		return false, ""
	}
	hb := workerSubmitHeartbeatUnixSince(logDir, workerID, startedAt)
	why := "submit_heartbeat_stale"
	if hb <= 0 {
		// No in-session submit yet: silence counts from process start, not from grace
		// expiry — otherwise CUDA cold-start gets killed at grace+1s.
		if startedAt <= 0 {
			return false, ""
		}
		hb = startedAt
		why = "no_submit_heartbeat_after_grace"
	}
	if nowUnix-hb > staleSec {
		return true, why
	}
	return false, ""
}
