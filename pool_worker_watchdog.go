package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"time"
)

func envPoolWorkerWatchdogEnabled() bool {
	if v := strings.TrimSpace(os.Getenv("HACKME_WORKER_WATCHDOG")); v != "" {
		return envBool("HACKME_WORKER_WATCHDOG", false)
	}
	// Default on for desktop + public pool (Windows/Linux installers).
	return envBool("HACKME_DESKTOP_MODE", false) && strings.TrimSpace(os.Getenv("HACKME_POOL_COORDINATOR_URL")) != ""
}

func poolWorkerWatchdogInterval() time.Duration {
	sec := 45
	if v := strings.TrimSpace(os.Getenv("HACKME_WORKER_WATCHDOG_SEC")); v != "" {
		if x, err := strconv.Atoi(v); err == nil && x >= 20 && x <= 600 {
			sec = x
		}
	}
	return time.Duration(sec) * time.Second
}

func nodeLoopbackBase() string {
	addr := strings.TrimSpace(os.Getenv("HACKME_BIND_ADDR"))
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	if !strings.Contains(addr, "://") {
		addr = "http://" + addr
	}
	return strings.TrimRight(addr, "/")
}

// restartPoolWorkerViaAPI starts the pool worker using an in-process handler call.
// Avoids Windows loopback HTTP races and keeps the same auth path as the dashboard.
func (a *app) restartPoolWorkerViaAPI() error {
	admin := strings.TrimSpace(os.Getenv("HACKME_ADMIN_TOKEN"))
	if admin == "" {
		return fmt.Errorf("HACKME_ADMIN_TOKEN missing")
	}
	coord := a.coordinatorBaseURL()
	if coord == "" {
		return fmt.Errorf("pool coordinator URL not configured")
	}
	body := map[string]any{"coord_url": coord}
	if wid := strings.TrimSpace(os.Getenv("WORKER_ID")); wid != "" {
		body["worker_id"] = wid
	} else if wid := strings.TrimSpace(a.workerID); wid != "" {
		body["worker_id"] = wid
	}
	if v := strings.TrimSpace(os.Getenv("HACKME_GPU_BACKEND")); v != "" {
		body["gpu_backend"] = v
	}
	if v := strings.TrimSpace(os.Getenv("HACKME_WORKER_BATCH_SIZE")); v != "" {
		if x, err := strconv.ParseUint(v, 10, 64); err == nil && x > 0 {
			body["batch_size"] = x
		}
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/worker/start", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hackme-Admin-Token", admin)
	// Loopback Host so desktop CSRF / Host checks stay satisfied if added later.
	req.Host = "127.0.0.1:8080"
	req.RemoteAddr = "127.0.0.1:0"
	rec := httptest.NewRecorder()
	a.handleWorkerStart(rec, req)
	res := rec.Result()
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(b))
		if msg == "" {
			msg = res.Status
		}
		return fmt.Errorf("worker/start HTTP %d: %s (admin_token_len=%d)", res.StatusCode, msg, len(admin))
	}
	var out struct {
		OK bool `json:"ok"`
	}
	_ = json.Unmarshal(b, &out)
	if !out.OK {
		return fmt.Errorf("worker/start rejected: %s", strings.TrimSpace(string(b)))
	}
	return nil
}

func (a *app) startPoolWorkerWatchdog() {
	if !envPoolWorkerWatchdogEnabled() {
		return
	}
	if strings.TrimSpace(a.coordinatorBaseURL()) == "" {
		return
	}
	interval := poolWorkerWatchdogInterval()
	go func() {
		// Fast first start so desktop miners do not need a manual Start Worker click.
		time.Sleep(3 * time.Second)
		log.Printf("pool worker watchdog: enabled (every %s, in-process)", interval)
		var lastRestartUnix int64
		first := true
		for {
			if miningPaused() {
				time.Sleep(interval)
				continue
			}
			if !a.workerProcessRunning() {
				now := time.Now().Unix()
				if !first && now-lastRestartUnix < 30 {
					time.Sleep(interval)
					continue
				}
				first = false
				if err := a.restartPoolWorkerViaAPI(); err != nil {
					log.Printf("pool worker watchdog: restart failed: %v", err)
				} else {
					lastRestartUnix = now
					a.workerMu.Lock()
					wid := a.workerID
					a.workerMu.Unlock()
					log.Printf("pool worker watchdog: restarted pool worker (%s)", wid)
				}
			}
			time.Sleep(interval)
		}
	}()
}
