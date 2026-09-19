package poolsync

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// UploadHuntHarness POSTs a published harness blob to the coordinator pool API.
// Prefers raw application/octet-stream (issue #8 Phase 1); falls back to JSON binary_b64
// when HACKME_HUNT_HARNESS_JSON_UPLOAD=1 or octet-stream is rejected with 415/400.
func UploadHuntHarness(ctx context.Context, coordURL, token, hash string, data []byte, sourceRel string) error {
	coordURL = strings.TrimRight(strings.TrimSpace(coordURL), "/")
	hash = strings.TrimSpace(hash)
	if coordURL == "" || hash == "" {
		return fmt.Errorf("pool harness sync: coordinator url and hash required")
	}
	if token == "" {
		return fmt.Errorf("pool harness sync: coordinator admin token not set")
	}
	if len(data) == 0 {
		return fmt.Errorf("pool harness sync: empty harness")
	}
	forceJSON := strings.TrimSpace(os.Getenv("HACKME_HUNT_HARNESS_JSON_UPLOAD")) == "1"
	if !forceJSON {
		if err := uploadHuntHarnessOctet(ctx, coordURL, token, hash, data, sourceRel); err == nil {
			return nil
		}
		// Fall through to legacy JSON for older coordinators.
	}
	return uploadHuntHarnessJSON(ctx, coordURL, token, hash, data, sourceRel)
}

func uploadHuntHarnessOctet(ctx context.Context, coordURL, token, hash string, data []byte, sourceRel string) error {
	reqCtx, cancel := context.WithTimeout(ctx, timeoutDuration())
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, coordURL+"/api/fuzz/pool/hunt/harness", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Hackme-Harness-Hash", hash)
	if strings.TrimSpace(sourceRel) != "" {
		req.Header.Set("X-Hackme-Source-Rel", sourceRel)
	}
	req.Header.Set("X-Hackme-Admin-Token", token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
		return fmt.Errorf("pool harness sync HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

func uploadHuntHarnessJSON(ctx context.Context, coordURL, token, hash string, data []byte, sourceRel string) error {
	body, err := json.Marshal(map[string]any{
		"harness_hash": hash,
		"source_rel":   strings.TrimSpace(sourceRel),
		"binary_b64":   base64.StdEncoding.EncodeToString(data),
	})
	if err != nil {
		return err
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeoutDuration())
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, coordURL+"/api/fuzz/pool/hunt/harness", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hackme-Admin-Token", token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
		return fmt.Errorf("pool harness sync HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

// IsHarnessAlreadyBound reports coordinator rejecting overwrite of an existing hash.
func IsHarnessAlreadyBound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already bound") || strings.Contains(msg, "already exists")
}

// CoordinatorHarnessAvailable GETs the published harness (admin token).
func CoordinatorHarnessAvailable(ctx context.Context, coordURL, token, hash string) bool {
	coordURL = strings.TrimRight(strings.TrimSpace(coordURL), "/")
	hash = strings.TrimSpace(hash)
	if coordURL == "" || hash == "" {
		return false
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeoutDuration())
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, coordURL+"/api/fuzz/pool/hunt/harness/"+hash, nil)
	if err != nil {
		return false
	}
	if token != "" {
		req.Header.Set("X-Hackme-Admin-Token", token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return false
	}
	n, _ := io.Copy(io.Discard, io.LimitReader(res.Body, 64<<20))
	return n > 0
}
