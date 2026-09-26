package hunt

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hackme/internal/store"
)

func TestHarnessArtifactRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HACKME_HUNT_HARNESS_DIR", "")
	SetHarnessObjectDir("")
	db, err := store.Open(filepath.Join(dir, "hunt-artifact.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	data := []byte{0x7f, 'E', 'L', 'F'}
	if err := PutHarnessArtifact(ctx, db, "abc12345", data, "fuzz/target.c"); err != nil {
		t.Fatal(err)
	}
	got, err := GetHarnessArtifact(ctx, db, "abc12345")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Fatalf("blob mismatch")
	}
	if err := PutHarnessArtifact(ctx, db, "../etc/passwd", data, "x"); err == nil {
		t.Fatal("path traversal hash must be rejected")
	}
	if ValidHarnessHash("ab/cd") || ValidHarnessHash("short") {
		t.Fatal("invalid hashes accepted")
	}
}

func TestHarnessObjectStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	obj := filepath.Join(dir, "harness")
	SetHarnessObjectDir(obj)
	t.Cleanup(func() { SetHarnessObjectDir("") })
	db, err := store.Open(filepath.Join(dir, "hunt-obj.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	data := bytes.Repeat([]byte{0x41}, 64*1024)
	hash := "deadbeefcafe0011"
	if err := PutHarnessArtifact(ctx, db, hash, data, "t.c"); err != nil {
		t.Fatal(err)
	}
	if !HarnessObjectExists(obj, hash) {
		t.Fatal("expected on-disk object")
	}
	var blobLen int
	if err := db.QueryRow(`SELECT length(binary_blob) FROM hunt_harness_artifacts WHERE harness_hash=?`, hash).Scan(&blobLen); err != nil {
		t.Fatal(err)
	}
	if blobLen != 0 {
		t.Fatalf("sqlite blob should be empty, got %d", blobLen)
	}
	got, err := GetHarnessArtifact(ctx, db, hash)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("disk roundtrip mismatch")
	}
	n, err := BackfillHarnessArtifactsToDisk(ctx, db, obj)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("already on disk; backfill want 0 got %d", n)
	}
}

func TestHarnessBackfillClearsBlob(t *testing.T) {
	dir := t.TempDir()
	SetHarnessObjectDir("")
	db, err := store.Open(filepath.Join(dir, "hunt-bf.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	data := []byte("ELF-BACKFILL-TEST")
	hash := "aabbccddeeff0011"
	// Insert legacy blob without object store.
	if err := PutHarnessArtifact(ctx, db, hash, data, "x.c"); err != nil {
		t.Fatal(err)
	}
	obj := filepath.Join(dir, "harness")
	SetHarnessObjectDir(obj)
	t.Cleanup(func() { SetHarnessObjectDir("") })
	n, err := BackfillHarnessArtifactsToDisk(ctx, db, obj)
	if err != nil || n != 1 {
		t.Fatalf("backfill n=%d err=%v", n, err)
	}
	var blobLen int
	_ = db.QueryRow(`SELECT length(binary_blob) FROM hunt_harness_artifacts WHERE harness_hash=?`, hash).Scan(&blobLen)
	if blobLen != 0 {
		t.Fatalf("blob not cleared: %d", blobLen)
	}
	got, err := GetHarnessArtifact(ctx, db, hash)
	if err != nil || string(got) != string(data) {
		t.Fatalf("get after backfill: %v len=%d", err, len(got))
	}
}

func TestSafeHarnessFetchURL(t *testing.T) {
	t.Setenv("HACKME_POOL_COORDINATOR_URL", "")
	t.Setenv("HACKME_COORDINATOR_URL", "")
	t.Setenv("COORD_URL", "")
	t.Setenv("HACKME_HUNT_HARNESS_ALLOW_HTTP", "")
	t.Setenv("HACKME_HUNT_HARNESS_REQUIRE_TLS", "")

	if !SafeHarnessFetchURL("/api/fuzz/pool/hunt/harness/abc12345") {
		t.Fatal("relative harness path")
	}
	if !SafeHarnessFetchURL("/pool/coordinator/api/fuzz/pool/hunt/harness/abc12345") {
		t.Fatal("pool coordinator relative path")
	}
	if SafeHarnessFetchURL("http://127.0.0.1/api/fuzz/pool/hunt/harness/abc12345") {
		t.Fatal("absolute URL without coordinator env must be rejected")
	}
	if SafeHarnessFetchURL("https://evil.example/api/fuzz/pool/hunt/harness/abc12345") {
		t.Fatal("arbitrary public HTTPS must be rejected")
	}
	if SafeHarnessFetchURL("https://hackme.tech/pool/coordinator/api/fuzz/pool/hunt/harness/abc12345") {
		t.Fatal("public https without matching coordinator env must be rejected")
	}

	t.Setenv("HACKME_POOL_COORDINATOR_URL", "http://203.0.113.10:18083")
	if !SafeHarnessFetchURL("http://203.0.113.10:18083/api/fuzz/pool/hunt/harness/abc12345") {
		t.Fatal("pool-direct host:port must match coordinator env")
	}
	if SafeHarnessFetchURL("https://evil.example/api/fuzz/pool/hunt/harness/abc12345") {
		t.Fatal("third-party https still rejected when coord set")
	}

	t.Setenv("HACKME_POOL_COORDINATOR_URL", "https://hackme.tech/pool/coordinator")
	if !SafeHarnessFetchURL("https://hackme.tech/pool/coordinator/api/fuzz/pool/hunt/harness/abc12345") {
		t.Fatal("same-host https coordinator path")
	}
	if SafeHarnessFetchURL("http://hackme.tech/pool/coordinator/api/fuzz/pool/hunt/harness/abc12345") {
		t.Fatal("http absolute must be rejected when coordinator is https")
	}
	if SafeHarnessFetchURL("http://169.254.169.254/api/fuzz/pool/hunt/harness/abc12345") {
		t.Fatal("link-local SSRF")
	}
}

func TestPutHarnessArtifactRejectsOverwrite(t *testing.T) {
	dir := t.TempDir()
	SetHarnessObjectDir("")
	t.Setenv("HACKME_HUNT_HARNESS_DIR", "")
	db, err := store.Open(filepath.Join(dir, "hunt-ow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := PutHarnessArtifact(ctx, db, "abc12345", []byte("AAAA"), "a.c"); err != nil {
		t.Fatal(err)
	}
	if err := PutHarnessArtifact(ctx, db, "abc12345", []byte("BBBB"), "a.c"); err == nil {
		t.Fatal("different blob overwrite must fail")
	}
	if err := PutHarnessArtifact(ctx, db, "abc12345", []byte("AAAA"), "a.c"); err != nil {
		t.Fatal("identical re-publish must be ok")
	}
}

func TestHarnessArtifactReady(t *testing.T) {
	dir := t.TempDir()
	obj := filepath.Join(dir, "harness")
	SetHarnessObjectDir(obj)
	t.Cleanup(func() { SetHarnessObjectDir("") })
	db, err := store.Open(filepath.Join(dir, "ready.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := HarnessArtifactReady(ctx, db, "deadbeefdeadbeef"); err == nil {
		t.Fatal("expected missing")
	}
	data := []byte{0x7f, 'E', 'L', 'F', 0, 1, 2, 3}
	if err := PutHarnessArtifact(ctx, db, "deadbeefdeadbeef", data, "t.c"); err != nil {
		t.Fatal(err)
	}
	if err := HarnessArtifactReady(ctx, db, "deadbeefdeadbeef"); err != nil {
		t.Fatal(err)
	}
}

func TestHarnessFetchURL(t *testing.T) {
	u := HarnessFetchURL("deadbeef")
	if u != "/api/fuzz/pool/hunt/harness/deadbeef" {
		t.Fatalf("url=%q", u)
	}
}

func TestMaterializeHarnessRequiresAttestationForHTTP(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	_, err := MaterializeHarness(ctx, dir, "abc12345deadbeef", "https://example.invalid/api/fuzz/pool/hunt/harness/abc12345deadbeef", "", nil)
	if err == nil {
		t.Fatal("HTTP fetch without attestation must fail")
	}
}

func TestMaterializeHarnessRejectsTamperedCache(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	hash := "aabbccddeeff0011"
	good := []byte("GOOD-HARNESS-BYTES-AAAAAAAA")
	want := ContentFingerprint(good)
	cachePath := huntHarnessCachePath(dir, hash)
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachePath, []byte("POISONED-BINARY-XXXXXXXX"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeHarnessCacheAttestation(cachePath, want); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(dir, "mat.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := PutHarnessArtifact(ctx, db, hash, good, "t.c"); err != nil {
		t.Fatal(err)
	}
	gotPath, err := MaterializeHarness(ctx, dir, hash, "", want, db)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(gotPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(good) {
		t.Fatalf("expected good bytes after quarantine+reload")
	}
}

func TestMaterializeHarnessHTTPVerifiesContent(t *testing.T) {
	good := []byte("HTTP-HARNESS-PAYLOAD-OK")
	want := ContentFingerprint(good)
	hash := "1122334455667788"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, hash) {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(good)
	}))
	defer srv.Close()
	t.Setenv("HACKME_POOL_COORDINATOR_URL", srv.URL)
	dir := t.TempDir()
	ctx := context.Background()
	fetch := srv.URL + "/api/fuzz/pool/hunt/harness/" + hash
	if !SafeHarnessFetchURL(fetch) {
		t.Fatal("test server URL should be allowed as coordinator")
	}
	// Wrong attestation
	if _, err := MaterializeHarness(ctx, dir, hash, fetch, strings.Repeat("ab", 32), nil); err == nil {
		t.Fatal("wrong attestation must fail")
	}
	path, err := MaterializeHarness(ctx, dir, hash, fetch, want, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(good) {
		t.Fatal("payload mismatch")
	}
	// Poison cache then re-materialize with same attestation from HTTP
	_ = os.WriteFile(path, []byte("EVIL"), 0o755)
	path2, err := MaterializeHarness(ctx, dir, hash, fetch, want, nil)
	if err != nil {
		t.Fatal(err)
	}
	got2, _ := os.ReadFile(path2)
	if string(got2) != string(good) {
		t.Fatal("poisoned cache must be replaced")
	}
}

func TestGetHarnessContentSHA256(t *testing.T) {
	dir := t.TempDir()
	SetHarnessObjectDir("")
	db, err := store.Open(filepath.Join(dir, "sha.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	data := []byte("sha-lookup-bytes")
	hash := "9988776655443322"
	if err := PutHarnessArtifact(ctx, db, hash, data, "a.c"); err != nil {
		t.Fatal(err)
	}
	fp, err := GetHarnessContentSHA256(ctx, db, hash)
	if err != nil {
		t.Fatal(err)
	}
	if fp != ContentFingerprint(data) {
		t.Fatalf("fp=%s", fp)
	}
}
