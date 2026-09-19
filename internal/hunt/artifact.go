package hunt

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const maxHarnessArtifactBytes = 32 << 20 // 32 MiB

var harnessHashRe = regexp.MustCompile(`(?i)^[a-f0-9]{8,128}$`)

// ValidHarnessHash rejects path traversal / non-hex ids used in cache filenames.
func ValidHarnessHash(hash string) bool {
	hash = strings.TrimSpace(hash)
	return harnessHashRe.MatchString(hash)
}

// PutHarnessArtifact stores a published Hunt harness binary keyed by hash.
// When HarnessObjectDir is set, bytes go to disk and SQLite keeps metadata only
// (empty binary_blob + content_sha256) — issue #8 Phase 1.
func PutHarnessArtifact(ctx context.Context, db *sql.DB, hash string, data []byte, sourceRel string) error {
	if db == nil {
		return fmt.Errorf("hunt artifact: no database")
	}
	hash = strings.TrimSpace(strings.ToLower(hash))
	if !ValidHarnessHash(hash) {
		return fmt.Errorf("hunt artifact: invalid harness hash")
	}
	if len(data) == 0 {
		return fmt.Errorf("hunt artifact: empty binary")
	}
	if len(data) > maxHarnessArtifactBytes {
		return fmt.Errorf("hunt artifact: exceeds %d bytes", maxHarnessArtifactBytes)
	}
	fp := contentSHA256Hex(data)
	now := time.Now().Unix()

	// Overwrite guard: existing disk object or blob/fingerprint must match.
	if dir := HarnessObjectDir(); dir != "" && HarnessObjectExists(dir, hash) {
		existing, err := ReadHarnessObject(dir, hash)
		if err != nil {
			return err
		}
		if !bytesEqual(existing, data) {
			return fmt.Errorf("hunt artifact: harness_hash %s already bound to different binary", hash)
		}
		_, err = db.ExecContext(ctx,
			`INSERT INTO hunt_harness_artifacts (harness_hash, binary_blob, byte_size, source_rel, created_at, content_sha256)
			 VALUES (?, X'', ?, ?, ?, ?)
			 ON CONFLICT(harness_hash) DO UPDATE SET
			   byte_size=excluded.byte_size,
			   source_rel=CASE WHEN excluded.source_rel != '' THEN excluded.source_rel ELSE hunt_harness_artifacts.source_rel END,
			   content_sha256=excluded.content_sha256`,
			hash, len(data), strings.TrimSpace(sourceRel), now, fp)
		return err
	}

	var existingBlob []byte
	var existingFP string
	err := db.QueryRowContext(ctx,
		`SELECT binary_blob, COALESCE(content_sha256,'') FROM hunt_harness_artifacts WHERE harness_hash=?`, hash).
		Scan(&existingBlob, &existingFP)
	if err == nil {
		if existingFP != "" && existingFP != fp {
			return fmt.Errorf("hunt artifact: harness_hash %s already bound to different binary", hash)
		}
		if len(existingBlob) > 0 && !bytesEqual(existingBlob, data) {
			return fmt.Errorf("hunt artifact: harness_hash %s already bound to different binary", hash)
		}
		if existingFP == fp || bytesEqual(existingBlob, data) {
			// Ensure disk copy exists when object store is enabled.
			if dir := HarnessObjectDir(); dir != "" {
				if _, werr := WriteHarnessObject(dir, hash, data); werr != nil {
					return werr
				}
				_, _ = db.ExecContext(ctx,
					`UPDATE hunt_harness_artifacts SET binary_blob=X'', content_sha256=?, byte_size=? WHERE harness_hash=?`,
					fp, len(data), hash)
			}
			return nil
		}
	} else if err != sql.ErrNoRows {
		return err
	}

	storeBlob := data
	if dir := HarnessObjectDir(); dir != "" {
		if _, err := WriteHarnessObject(dir, hash, data); err != nil {
			return err
		}
		storeBlob = []byte{} // empty blob in SQLite (NOT NULL)
	}
	_, err = db.ExecContext(ctx,
		`INSERT INTO hunt_harness_artifacts (harness_hash, binary_blob, byte_size, source_rel, created_at, content_sha256)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(harness_hash) DO UPDATE SET
		   binary_blob=excluded.binary_blob,
		   byte_size=excluded.byte_size,
		   source_rel=CASE WHEN excluded.source_rel != '' THEN excluded.source_rel ELSE hunt_harness_artifacts.source_rel END,
		   content_sha256=excluded.content_sha256,
		   created_at=excluded.created_at`,
		hash, storeBlob, len(data), strings.TrimSpace(sourceRel), now, fp)
	return err
}

// GetHarnessArtifact loads a published harness binary (disk first, then SQLite BLOB).
func GetHarnessArtifact(ctx context.Context, db *sql.DB, hash string) ([]byte, error) {
	if db == nil {
		return nil, fmt.Errorf("hunt artifact: no database")
	}
	hash = strings.TrimSpace(strings.ToLower(hash))
	if !ValidHarnessHash(hash) {
		return nil, fmt.Errorf("hunt artifact: invalid harness hash")
	}
	if dir := HarnessObjectDir(); dir != "" {
		if data, err := ReadHarnessObject(dir, hash); err == nil {
			return data, nil
		}
	}
	var blob []byte
	err := db.QueryRowContext(ctx,
		`SELECT binary_blob FROM hunt_harness_artifacts WHERE harness_hash=?`, hash).
		Scan(&blob)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("hunt artifact: %s not found", hash)
	}
	if err != nil {
		return nil, err
	}
	if len(blob) == 0 {
		// Metadata-only row without disk file.
		if dir := HarnessObjectDir(); dir != "" && HarnessObjectExists(dir, hash) {
			return ReadHarnessObject(dir, hash)
		}
		return nil, fmt.Errorf("hunt artifact: %s not found", hash)
	}
	// Lazy backfill to disk when store is configured.
	if dir := HarnessObjectDir(); dir != "" {
		if _, werr := WriteHarnessObject(dir, hash, blob); werr == nil {
			fp := contentSHA256Hex(blob)
			_, _ = db.ExecContext(ctx,
				`UPDATE hunt_harness_artifacts SET binary_blob=X'', content_sha256=? WHERE harness_hash=?`, fp, hash)
		}
	}
	return blob, nil
}

// GetHarnessArtifactPath returns the on-disk path when the object store holds the harness.
func GetHarnessArtifactPath(hash string) (string, bool) {
	dir := HarnessObjectDir()
	if dir == "" {
		return "", false
	}
	path, err := HarnessObjectPath(dir, hash)
	if err != nil || !HarnessObjectExists(dir, hash) {
		return "", false
	}
	return path, true
}

// HarnessArtifactReady reports whether workers can fetch this harness (disk object or SQLite blob).
// Cheap claim/Tick gate — does not load the full binary into memory.
func HarnessArtifactReady(ctx context.Context, db *sql.DB, hash string) error {
	hash = strings.TrimSpace(strings.ToLower(hash))
	if !ValidHarnessHash(hash) {
		return fmt.Errorf("hunt artifact: invalid harness hash")
	}
	if dir := HarnessObjectDir(); dir != "" && HarnessObjectExists(dir, hash) {
		return nil
	}
	if db == nil {
		return fmt.Errorf("hunt artifact: %s not found", hash)
	}
	var blobLen int
	err := db.QueryRowContext(ctx,
		`SELECT length(binary_blob) FROM hunt_harness_artifacts WHERE harness_hash=?`, hash).
		Scan(&blobLen)
	if err == sql.ErrNoRows {
		return fmt.Errorf("hunt artifact: %s not found", hash)
	}
	if err != nil {
		return err
	}
	if blobLen > 0 {
		return nil
	}
	// Metadata-only row: ready only if object store file exists.
	if dir := HarnessObjectDir(); dir != "" && HarnessObjectExists(dir, hash) {
		return nil
	}
	return fmt.Errorf("hunt artifact: %s not found", hash)
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

// PublishHarnessFile reads a local harness binary into the artifact store.
func PublishHarnessFile(ctx context.Context, db *sql.DB, hash, path, sourceRel string) error {
	root := RepoRoot()
	if root != "" {
		var err error
		path, err = MustUnderRoot(root, path)
		if err != nil {
			return fmt.Errorf("hunt artifact: harness path outside repo root: %w", err)
		}
		data, err := SafeReadFileUnder(root, path)
		if err != nil {
			return err
		}
		return PutHarnessArtifact(ctx, db, hash, data, sourceRel)
	}
	safe, err := allowlistedAbs(path)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(safe)
	if err != nil {
		return err
	}
	return PutHarnessArtifact(ctx, db, hash, data, sourceRel)
}

// MaterializeHarness writes a harness to repo cache, loading from DB or HTTP fetch URL when needed.
func MaterializeHarness(ctx context.Context, repoRoot, hash, fetchURL string, db *sql.DB) (string, error) {
	hash = strings.TrimSpace(hash)
	if !ValidHarnessHash(hash) {
		return "", fmt.Errorf("hunt artifact: invalid harness hash")
	}
	if repoRoot == "" {
		repoRoot = RepoRoot()
	}
	cachePath := huntHarnessCachePath(repoRoot, hash)
	if st, err := osStat(cachePath); err == nil && st {
		harnessCache.Store(hash, cachePath)
		return cachePath, nil
	}
	var data []byte
	var err error
	if db != nil {
		data, err = GetHarnessArtifact(ctx, db, hash)
		if err != nil && strings.TrimSpace(fetchURL) == "" {
			return "", err
		}
	}
	if len(data) == 0 && strings.TrimSpace(fetchURL) != "" {
		data, err = fetchHarnessHTTP(ctx, fetchURL)
		if err != nil {
			return "", err
		}
	}
	if len(data) == 0 {
		return "", fmt.Errorf("hunt artifact: %s not available locally", hash)
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return "", err
	}
	tmp, err := SafeCacheFile(repoRoot, "hunt-harness", hash, "bin.tmp")
	if err != nil {
		return "", err
	}
	if err := SafeWriteFileUnder(repoRoot, tmp, data, 0o755); err != nil {
		return "", err
	}
	if err := SafeRenameUnder(repoRoot, tmp, cachePath); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	harnessCache.Store(hash, cachePath)
	return cachePath, nil
}

func fetchHarnessHTTP(ctx context.Context, rawURL string) ([]byte, error) {
	rawURL = strings.TrimSpace(rawURL)
	if strings.HasPrefix(rawURL, "/api/fuzz/pool/hunt/harness/") {
		base := strings.TrimSpace(os.Getenv("HACKME_POOL_COORDINATOR_URL"))
		if base == "" {
			base = strings.TrimSpace(os.Getenv("HACKME_COORDINATOR_URL"))
		}
		if base == "" {
			base = strings.TrimSpace(os.Getenv("COORD_URL"))
		}
		if base == "" {
			return nil, fmt.Errorf("hunt artifact: relative fetch needs HACKME_POOL_COORDINATOR_URL")
		}
		rawURL = strings.TrimRight(base, "/") + rawURL
	}
	if !SafeHarnessFetchURL(rawURL) {
		return nil, fmt.Errorf("hunt artifact: fetch url not allowed")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	u, _ := url.Parse(rawURL)
	_, harnessOK := harnessFetchPathHash(u)
	attachBearer := harnessOK
	if attachBearer {
		if coord := strings.TrimSpace(os.Getenv("HACKME_POOL_COORDINATOR_URL")); coord != "" {
			if !sameCoordinatorHost(coord, u) {
				attachBearer = false
			}
		} else if coord := strings.TrimSpace(os.Getenv("COORD_URL")); coord != "" {
			if !sameCoordinatorHost(coord, u) {
				attachBearer = false
			}
		}
	}
	if attachBearer {
		token := strings.TrimSpace(os.Getenv("HACKME_COORDINATOR_WORKER_TOKEN"))
		if token == "" {
			token = strings.TrimSpace(os.Getenv("HACKME_POOL_COORDINATOR_WORKER_TOKEN"))
		}
		if token == "" {
			token = strings.TrimSpace(os.Getenv("COORD_TOKEN"))
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return nil, fmt.Errorf("hunt artifact fetch HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxHarnessArtifactBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxHarnessArtifactBytes {
		return nil, fmt.Errorf("hunt artifact: fetch exceeds max size")
	}
	return data, nil
}

// SafeHarnessFetchURL allows relative coordinator harness paths, same-host coordinator
// absolute URLs (incl. :port and /pool/coordinator prefix), or https public hosts
// with the harness path (blocks SSRF/private IPs).
func SafeHarnessFetchURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	if strings.HasPrefix(raw, "/") {
		_, ok := harnessFetchPathHash(&url.URL{Path: raw})
		return ok
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return false
	}
	if _, ok := harnessFetchPathHash(u); !ok {
		return false
	}
	if coord := strings.TrimSpace(os.Getenv("HACKME_POOL_COORDINATOR_URL")); coord != "" {
		if sameCoordinatorHost(coord, u) {
			return true
		}
	}
	if coord := strings.TrimSpace(os.Getenv("HACKME_COORDINATOR_URL")); coord != "" {
		if sameCoordinatorHost(coord, u) {
			return true
		}
	}
	if coord := strings.TrimSpace(os.Getenv("COORD_URL")); coord != "" {
		if sameCoordinatorHost(coord, u) {
			return true
		}
	}
	if u.Scheme != "https" {
		return false
	}
	host := u.Hostname()
	if host == "localhost" || host == "127.0.0.1" || host == "::1" || strings.HasSuffix(host, ".local") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return false
		}
	}
	return true
}

func harnessFetchPathHash(u *url.URL) (string, bool) {
	if u == nil {
		return "", false
	}
	p := path.Clean(u.Path)
	for _, prefix := range []string{
		"/api/fuzz/pool/hunt/harness/",
		"/pool/coordinator/api/fuzz/pool/hunt/harness/",
	} {
		if !strings.HasPrefix(p, prefix) {
			continue
		}
		h := strings.Trim(strings.TrimPrefix(p, prefix), "/")
		if ValidHarnessHash(h) && !strings.Contains(h, "/") && !strings.Contains(h, "..") {
			return h, true
		}
	}
	return "", false
}

func sameCoordinatorHost(coordURL string, u *url.URL) bool {
	cu, err := url.Parse(strings.TrimSpace(coordURL))
	if err != nil || cu.Host == "" || u == nil || u.Host == "" {
		return false
	}
	if !strings.EqualFold(cu.Hostname(), u.Hostname()) {
		return false
	}
	return urlPortOrDefault(cu) == urlPortOrDefault(u)
}

func urlPortOrDefault(u *url.URL) string {
	if u == nil {
		return ""
	}
	if p := u.Port(); p != "" {
		return p
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}

// HarnessFetchURL builds coordinator-relative fetch path for workers.
func HarnessFetchURL(hash string) string {
	hash = strings.TrimSpace(hash)
	if !ValidHarnessHash(hash) {
		return ""
	}
	return "/api/fuzz/pool/hunt/harness/" + hash
}

// ContentFingerprint returns sha256 hex of harness bytes (ops / integrity checks).
func ContentFingerprint(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
