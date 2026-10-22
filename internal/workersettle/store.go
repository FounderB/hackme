// Package workersettle is the HMC worker settlement ledger (SQLite + outbox).
// It extends the JSON flock model used by scripts/ops/settle_worker_payouts.sh
// toward transactional pending intents (same spirit as poolfuzz settle_outbox).
package workersettle

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"hackme/internal/store"

	_ "modernc.org/sqlite"
)

// WorkerRow is one worker's settlement watermark + optional pending intent.
type WorkerRow struct {
	WorkerID         string
	SettledHMC       float64
	SettledSUP       float64
	SessionAnchorHMC float64 // 0 = unset
	SessionAnchorSUP float64 // 0 = unset (coordinator SUP counter reset)
	PayoutAddress    string
	LastTxHash       string
	LastSettleUnix   int64
	PendingJSON      string // raw pending_settle object or empty
}

// Meta holds ledger-wide fields.
type Meta struct {
	LastForceUnix int64
}

// Store is a SQLite-backed settlement ledger.
type Store struct {
	db   *sql.DB
	path string
	mu   sync.Mutex
}

// Open opens (or creates) the settlement DB at path.
func Open(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("workersettle: empty path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil && filepath.Dir(path) != "." {
		return nil, err
	}
	db, err := sql.Open("sqlite", store.SQLiteDSN(path))
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	store.TuneSQLitePool(db, "main")
	s := &Store{db: db, path: path}
	if err := s.ensureSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// SQLiteDSNPublic is exposed for tests; prefer Open.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) ensureSchema() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS settle_meta (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  last_force_unix INTEGER NOT NULL DEFAULT 0,
  migrated_from_json INTEGER NOT NULL DEFAULT 0
);
INSERT OR IGNORE INTO settle_meta (id, last_force_unix, migrated_from_json) VALUES (1, 0, 0);

CREATE TABLE IF NOT EXISTS settle_workers (
  worker_id TEXT PRIMARY KEY,
  settled_hmc REAL NOT NULL DEFAULT 0,
  settled_sup REAL NOT NULL DEFAULT 0,
  session_anchor_hmc REAL NOT NULL DEFAULT 0,
  session_anchor_sup REAL NOT NULL DEFAULT 0,
  payout_address TEXT NOT NULL DEFAULT '',
  last_tx_hash TEXT NOT NULL DEFAULT '',
  last_settle_unix INTEGER NOT NULL DEFAULT 0,
  pending_json TEXT NOT NULL DEFAULT '',
  updated_at INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS settle_outbox (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  worker_id TEXT NOT NULL,
  payout_address TEXT NOT NULL,
  delta_hmc REAL NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  tx_hash TEXT NOT NULL DEFAULT '',
  nonce INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  applied_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_settle_outbox_status ON settle_outbox(status, id);
`)
	if err != nil {
		return err
	}
	// Idempotent column add for DBs created before session_anchor_sup.
	_, _ = s.db.Exec(`ALTER TABLE settle_workers ADD COLUMN session_anchor_sup REAL NOT NULL DEFAULT 0`)
	return nil
}

// MigrateFromJSON imports a legacy worker_settlement_state.json once.
func (s *Store) MigrateFromJSON(ctx context.Context, jsonPath string) (bool, error) {
	if s == nil || s.db == nil {
		return false, fmt.Errorf("workersettle: nil store")
	}
	jsonPath = strings.TrimSpace(jsonPath)
	if jsonPath == "" {
		return false, nil
	}
	var migrated int
	if err := s.db.QueryRowContext(ctx, `SELECT migrated_from_json FROM settle_meta WHERE id=1`).Scan(&migrated); err != nil {
		return false, err
	}
	if migrated != 0 {
		return false, nil
	}
	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		// Refuse to mark migrated on a missing path — that previously allowed a later
		// export-json to wipe a real JSON history with an empty ledger (audit 5c).
		if os.IsNotExist(err) {
			return false, fmt.Errorf("workersettle: migrate json missing %s (refusing empty migration)", jsonPath)
		}
		return false, err
	}
	var st struct {
		Workers map[string]struct {
			SettledHMC       float64         `json:"settled_hmc"`
			SettledSUP       float64         `json:"settled_sup"`
			SessionAnchorHMC float64         `json:"session_anchor_hmc"`
			SessionAnchorSUP float64         `json:"session_anchor_sup"`
			PayoutAddress    string          `json:"payout_address"`
			LastTxHash       string          `json:"last_tx_hash"`
			LastSettleUnix   int64           `json:"last_settle_unix"`
			PendingSettle    json.RawMessage `json:"pending_settle"`
		} `json:"workers"`
		Meta struct {
			LastForceUnix int64 `json:"last_force_unix"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().Unix()
	for wid, e := range st.Workers {
		wid = strings.TrimSpace(wid)
		if wid == "" {
			continue
		}
		pend := ""
		if len(e.PendingSettle) > 0 && string(e.PendingSettle) != "null" {
			pend = string(e.PendingSettle)
		}
		_, err := tx.ExecContext(ctx, `
INSERT INTO settle_workers (worker_id, settled_hmc, settled_sup, session_anchor_hmc, session_anchor_sup, payout_address, last_tx_hash, last_settle_unix, pending_json, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(worker_id) DO UPDATE SET
  settled_hmc=excluded.settled_hmc,
  settled_sup=excluded.settled_sup,
  session_anchor_hmc=excluded.session_anchor_hmc,
  session_anchor_sup=excluded.session_anchor_sup,
  payout_address=excluded.payout_address,
  last_tx_hash=excluded.last_tx_hash,
  last_settle_unix=excluded.last_settle_unix,
  pending_json=excluded.pending_json,
  updated_at=excluded.updated_at`,
			wid, e.SettledHMC, e.SettledSUP, e.SessionAnchorHMC, e.SessionAnchorSUP, strings.TrimSpace(e.PayoutAddress),
			strings.TrimSpace(e.LastTxHash), e.LastSettleUnix, pend, now)
		if err != nil {
			return false, err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE settle_meta SET last_force_unix=?, migrated_from_json=1 WHERE id=1`, st.Meta.LastForceUnix)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// GetWorker returns a row or zero values if missing.
func (s *Store) GetWorker(ctx context.Context, workerID string) (WorkerRow, error) {
	var r WorkerRow
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return r, fmt.Errorf("workersettle: empty worker_id")
	}
	err := s.db.QueryRowContext(ctx, `
SELECT worker_id, settled_hmc, settled_sup, session_anchor_hmc, session_anchor_sup, payout_address, last_tx_hash, last_settle_unix, pending_json
  FROM settle_workers WHERE worker_id=?`, workerID).Scan(
		&r.WorkerID, &r.SettledHMC, &r.SettledSUP, &r.SessionAnchorHMC, &r.SessionAnchorSUP, &r.PayoutAddress, &r.LastTxHash, &r.LastSettleUnix, &r.PendingJSON)
	if err == sql.ErrNoRows {
		r.WorkerID = workerID
		return r, nil
	}
	return r, err
}

// SetPending records durable pending intent (BEGIN IMMEDIATE) and enqueues outbox.
func (s *Store) SetPending(ctx context.Context, workerID, payoutAddr string, deltaHMC float64, nonce int64) (outboxID int64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	workerID = strings.TrimSpace(workerID)
	payoutAddr = strings.TrimSpace(payoutAddr)
	if workerID == "" || payoutAddr == "" || deltaHMC <= 0 {
		return 0, fmt.Errorf("workersettle: invalid pending args")
	}
	now := time.Now().Unix()
	pendObj := map[string]any{
		"delta_hmc":      deltaHMC,
		"payout_address": payoutAddr,
		"started_unix":   now,
		"nonce":          nonce,
	}
	pendRaw, _ := json.Marshal(pendObj)

	conn, err := s.db.Conn(ctx)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return 0, err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(ctx, `ROLLBACK`)
		}
	}()
	_, err = conn.ExecContext(ctx, `
INSERT INTO settle_workers (worker_id, settled_hmc, payout_address, pending_json, updated_at)
VALUES (?, 0, ?, ?, ?)
ON CONFLICT(worker_id) DO UPDATE SET
  payout_address=excluded.payout_address,
  pending_json=excluded.pending_json,
  updated_at=excluded.updated_at`, workerID, payoutAddr, string(pendRaw), now)
	if err != nil {
		return 0, err
	}
	res, err := conn.ExecContext(ctx, `
INSERT INTO settle_outbox (worker_id, payout_address, delta_hmc, status, nonce, created_at)
VALUES (?, ?, ?, 'pending', ?, ?)`, workerID, payoutAddr, deltaHMC, nonce, now)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		return 0, err
	}
	committed = true
	return id, nil
}

// AttachTxHash stamps pending + latest outbox row with tx_hash.
func (s *Store) AttachTxHash(ctx context.Context, workerID, txHash string) error {
	workerID = strings.TrimSpace(workerID)
	txHash = strings.TrimSpace(txHash)
	if workerID == "" || txHash == "" {
		return fmt.Errorf("workersettle: empty worker or tx")
	}
	row, err := s.GetWorker(ctx, workerID)
	if err != nil {
		return err
	}
	pend := map[string]any{}
	if row.PendingJSON != "" {
		_ = json.Unmarshal([]byte(row.PendingJSON), &pend)
	}
	pend["tx_hash"] = txHash
	raw, _ := json.Marshal(pend)
	_, err = s.db.ExecContext(ctx, `
UPDATE settle_workers SET pending_json=?, updated_at=? WHERE worker_id=?`,
		string(raw), time.Now().Unix(), workerID)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
UPDATE settle_outbox SET tx_hash=?
 WHERE id=(SELECT id FROM settle_outbox WHERE worker_id=? AND status='pending' ORDER BY id DESC LIMIT 1)`,
		txHash, workerID)
	return err
}

// ClearPending drops pending intent without advancing settled (failed send)
// and abandons any pending outbox rows for the worker (audit 5d).
func (s *Store) ClearPending(ctx context.Context, workerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return fmt.Errorf("workersettle: empty worker_id")
	}
	now := time.Now().Unix()
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(ctx, `ROLLBACK`)
		}
	}()
	if _, err := conn.ExecContext(ctx, `
UPDATE settle_workers SET pending_json='', updated_at=? WHERE worker_id=?`,
		now, workerID); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `
UPDATE settle_outbox SET status='abandoned', applied_at=?
 WHERE worker_id=? AND status='pending'`, now, workerID); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		return err
	}
	committed = true
	return nil
}

// ApplySettle advances settled watermark, clears pending, marks outbox applied.
func (s *Store) ApplySettle(ctx context.Context, workerID, payoutAddr, txHash string, newSettled, sessionAnchor float64, clearAnchor bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return fmt.Errorf("workersettle: empty worker_id")
	}
	now := time.Now().Unix()
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(ctx, `ROLLBACK`)
		}
	}()
	anchor := sessionAnchor
	if clearAnchor {
		anchor = 0
	}
	// Never touch settled_sup here — SUP settle owns that watermark (audit 5a).
	_, err = conn.ExecContext(ctx, `
INSERT INTO settle_workers (worker_id, settled_hmc, settled_sup, session_anchor_hmc, payout_address, last_tx_hash, last_settle_unix, pending_json, updated_at)
VALUES (?, ?, 0, ?, ?, ?, ?, '', ?)
ON CONFLICT(worker_id) DO UPDATE SET
  settled_hmc=excluded.settled_hmc,
  session_anchor_hmc=excluded.session_anchor_hmc,
  payout_address=excluded.payout_address,
  last_tx_hash=excluded.last_tx_hash,
  last_settle_unix=excluded.last_settle_unix,
  pending_json='',
  updated_at=excluded.updated_at`,
		workerID, newSettled, anchor, strings.TrimSpace(payoutAddr), strings.TrimSpace(txHash), now, now)
	if err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, `
UPDATE settle_outbox SET status='applied', tx_hash=?, applied_at=?
 WHERE worker_id=? AND status='pending'`, strings.TrimSpace(txHash), now, workerID)
	if err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		return err
	}
	committed = true
	return nil
}

// SetLastForceUnix updates meta.
func (s *Store) SetLastForceUnix(ctx context.Context, ts int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE settle_meta SET last_force_unix=? WHERE id=1`, ts)
	return err
}

// GetMeta returns ledger meta.
func (s *Store) GetMeta(ctx context.Context) (Meta, error) {
	var m Meta
	err := s.db.QueryRowContext(ctx, `SELECT last_force_unix FROM settle_meta WHERE id=1`).Scan(&m.LastForceUnix)
	return m, err
}

// ExportJSON writes a JSON document compatible with settle_worker_payouts.sh / settlement_api.
func (s *Store) ExportJSON(ctx context.Context) ([]byte, error) {
	workers, meta, err := s.exportWorkers(ctx)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(map[string]any{
		"workers": workers,
		"meta":    map[string]any{"last_force_unix": meta.LastForceUnix},
	}, "", "  ")
}

func (s *Store) exportWorkers(ctx context.Context) (map[string]any, Meta, error) {
	var meta Meta
	rows, err := s.db.QueryContext(ctx, `
SELECT worker_id, settled_hmc, settled_sup, session_anchor_hmc, session_anchor_sup, payout_address, last_tx_hash, last_settle_unix, pending_json
  FROM settle_workers`)
	if err != nil {
		return nil, meta, err
	}
	defer rows.Close()
	workers := map[string]any{}
	for rows.Next() {
		var r WorkerRow
		if err := rows.Scan(&r.WorkerID, &r.SettledHMC, &r.SettledSUP, &r.SessionAnchorHMC, &r.SessionAnchorSUP, &r.PayoutAddress, &r.LastTxHash, &r.LastSettleUnix, &r.PendingJSON); err != nil {
			return nil, meta, err
		}
		ent := map[string]any{
			"settled_hmc":      r.SettledHMC,
			"settled_sup":      r.SettledSUP,
			"payout_address":   r.PayoutAddress,
			"last_tx_hash":     r.LastTxHash,
			"last_settle_unix": r.LastSettleUnix,
		}
		if r.SessionAnchorHMC > 0 {
			ent["session_anchor_hmc"] = r.SessionAnchorHMC
		}
		if r.SessionAnchorSUP > 0 {
			ent["session_anchor_sup"] = r.SessionAnchorSUP
		}
		if strings.TrimSpace(r.PendingJSON) != "" {
			var pend any
			if json.Unmarshal([]byte(r.PendingJSON), &pend) == nil {
				ent["pending_settle"] = pend
			}
		}
		workers[r.WorkerID] = ent
	}
	if err := rows.Err(); err != nil {
		return nil, meta, err
	}
	meta, err = s.GetMeta(ctx)
	return workers, meta, err
}

// SetSettledSUP updates the SUP watermark (and optional pending_sup blob via MergeExport).
// HMC ApplySettle must never clobber this field.
func (s *Store) SetSettledSUP(ctx context.Context, workerID string, settledSUP float64, payoutAddr string) error {
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return fmt.Errorf("workersettle: empty worker_id")
	}
	if settledSUP < 0 {
		return fmt.Errorf("workersettle: negative settled_sup")
	}
	now := time.Now().Unix()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO settle_workers (worker_id, settled_hmc, settled_sup, payout_address, updated_at)
VALUES (?, 0, ?, ?, ?)
ON CONFLICT(worker_id) DO UPDATE SET
  settled_sup=excluded.settled_sup,
  payout_address=CASE WHEN excluded.payout_address != '' THEN excluded.payout_address ELSE settle_workers.payout_address END,
  updated_at=excluded.updated_at`,
		workerID, settledSUP, strings.TrimSpace(payoutAddr), now)
	return err
}

// SetSessionAnchorSUP sets or clears the SUP session floor after a coordinator counter reset.
func (s *Store) SetSessionAnchorSUP(ctx context.Context, workerID string, anchor float64, clear bool) error {
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return fmt.Errorf("workersettle: empty worker_id")
	}
	if clear {
		anchor = 0
	} else if anchor < 0 {
		return fmt.Errorf("workersettle: negative session_anchor_sup")
	}
	now := time.Now().Unix()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO settle_workers (worker_id, settled_hmc, settled_sup, session_anchor_sup, updated_at)
VALUES (?, 0, 0, ?, ?)
ON CONFLICT(worker_id) DO UPDATE SET
  session_anchor_sup=excluded.session_anchor_sup,
  updated_at=excluded.updated_at`,
		workerID, anchor, now)
	return err
}

// MigratedFromJSON reports whether MigrateFromJSON completed successfully.
func (s *Store) MigratedFromJSON(ctx context.Context) (bool, error) {
	var migrated int
	err := s.db.QueryRowContext(ctx, `SELECT migrated_from_json FROM settle_meta WHERE id=1`).Scan(&migrated)
	return migrated != 0, err
}

// ExportJSONToFile merges SQLite HMC state with any existing JSON so SUP fields
// (settled_sup, pending_sup) are never rolled back (audit 5a/5b/5c).
func (s *Store) ExportJSONToFile(ctx context.Context, path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("workersettle: empty export path")
	}
	migrated, err := s.MigratedFromJSON(ctx)
	if err != nil {
		return err
	}
	if !migrated {
		return fmt.Errorf("workersettle: refuse export before successful migrate (migrated_from_json=0)")
	}

	workers, meta, err := s.exportWorkers(ctx)
	if err != nil {
		return err
	}

	type oldDoc struct {
		Workers map[string]json.RawMessage `json:"workers"`
		Meta    struct {
			LastForceUnix int64 `json:"last_force_unix"`
		} `json:"meta"`
	}
	var old oldDoc
	oldWorkers := map[string]map[string]any{}
	if raw, err := os.ReadFile(path); err == nil && len(raw) > 0 {
		if json.Unmarshal(raw, &old) == nil {
			for wid, blob := range old.Workers {
				var ent map[string]any
				if json.Unmarshal(blob, &ent) != nil {
					continue
				}
				oldWorkers[wid] = ent
			}
			if meta.LastForceUnix == 0 && old.Meta.LastForceUnix > 0 {
				meta.LastForceUnix = old.Meta.LastForceUnix
			}
		}
	}

	if len(workers) == 0 && len(oldWorkers) > 0 {
		return fmt.Errorf("workersettle: refuse empty export over non-empty JSON (%d workers) at %s", len(oldWorkers), path)
	}

	// Merge: never lower settled_sup; preserve pending_sup from JSON; keep JSON-only workers.
	for wid, rawEnt := range workers {
		ent, _ := rawEnt.(map[string]any)
		if ent == nil {
			ent = map[string]any{}
		}
		if oldEnt, ok := oldWorkers[wid]; ok {
			oldSUP := anyFloat(oldEnt["settled_sup"])
			dbSUP := anyFloat(ent["settled_sup"])
			if oldSUP > dbSUP {
				ent["settled_sup"] = oldSUP
				// Keep SQLite in sync so the next export does not need the JSON crutch.
				if err := s.SetSettledSUP(ctx, wid, oldSUP, fmt.Sprint(ent["payout_address"])); err != nil {
					log.Printf("workersettle: SetSettledSUP(%s) during export merge: %v", wid, err)
				}
			}
			if pend, ok := oldEnt["pending_sup"]; ok && pend != nil {
				ent["pending_sup"] = pend
			}
			// Keep SUP session anchor from JSON when SQLite has none (or keep max).
			oldAnchor := anyFloat(oldEnt["session_anchor_sup"])
			dbAnchor := anyFloat(ent["session_anchor_sup"])
			if oldAnchor > dbAnchor {
				ent["session_anchor_sup"] = oldAnchor
			}
		}
		workers[wid] = ent
	}
	for wid, oldEnt := range oldWorkers {
		if _, ok := workers[wid]; ok {
			continue
		}
		// Worker only known to SUP/JSON path — keep so HMC export does not erase them.
		workers[wid] = oldEnt
	}

	out, err := json.MarshalIndent(map[string]any{
		"workers": workers,
		"meta":    map[string]any{"last_force_unix": meta.LastForceUnix},
	}, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(path, append(out, '\n'), 0o600)
}

func anyFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case json.Number:
		f, _ := t.Float64()
		return f
	case string:
		var f float64
		_, _ = fmt.Sscanf(t, "%f", &f)
		return f
	default:
		return 0
	}
}

func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("empty path")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	_ = os.Chmod(tmpName, perm)
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

// CountPendingOutbox returns pending outbox rows.
func (s *Store) CountPendingOutbox(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM settle_outbox WHERE status='pending'`).Scan(&n)
	return n, err
}
