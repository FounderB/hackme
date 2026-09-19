package main

import (
	"database/sql"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// Issue #8 Phase 3: durable signed-payload / result-hash dedup across coordinator restarts.

func migrateWorkDedupTables(db *sql.DB) error {
	if db == nil {
		return nil
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS work_signed_payload_dedup (
			payload_hash TEXT PRIMARY KEY,
			seen_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS work_result_hash_dedup (
			result_hash TEXT PRIMARY KEY,
			seen_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_work_signed_payload_seen ON work_signed_payload_dedup(seen_at)`,
		`CREATE INDEX IF NOT EXISTS idx_work_result_hash_seen ON work_result_hash_dedup(seen_at)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

func (m *workManager) attachDedupDB(db *sql.DB) {
	if m == nil || db == nil {
		return
	}
	m.dedupDB = db
	if err := migrateWorkDedupTables(db); err != nil {
		log.Printf("work dedup migrate: %v", err)
		return
	}
	m.loadDurableDedup()
}

func workDedupTTLSec() int64 {
	v := strings.TrimSpace(os.Getenv("HACKME_WORK_DEDUP_TTL_SEC"))
	if v == "" {
		return 7 * 24 * 3600
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 3600 {
		return 7 * 24 * 3600
	}
	return n
}

func (m *workManager) loadDurableDedup() {
	if m == nil || m.dedupDB == nil {
		return
	}
	cutoff := time.Now().Unix() - workDedupTTLSec()
	_, _ = m.dedupDB.Exec(`DELETE FROM work_signed_payload_dedup WHERE seen_at < ?`, cutoff)
	_, _ = m.dedupDB.Exec(`DELETE FROM work_result_hash_dedup WHERE seen_at < ?`, cutoff)
	rows, err := m.dedupDB.Query(`SELECT payload_hash FROM work_signed_payload_dedup WHERE seen_at >= ?`, cutoff)
	if err == nil {
		for rows.Next() {
			var h string
			if rows.Scan(&h) == nil && h != "" {
				m.acceptedSignedPayloads[h] = struct{}{}
			}
		}
		rows.Close()
	}
	rows, err = m.dedupDB.Query(`SELECT result_hash FROM work_result_hash_dedup WHERE seen_at >= ?`, cutoff)
	if err == nil {
		for rows.Next() {
			var h string
			if rows.Scan(&h) == nil && h != "" {
				m.acceptedResultHashes[h] = struct{}{}
			}
		}
		rows.Close()
	}
	log.Printf("work dedup loaded: signed=%d results=%d ttl_sec=%d",
		len(m.acceptedSignedPayloads), len(m.acceptedResultHashes), workDedupTTLSec())
}

func (m *workManager) persistSignedPayload(key string) {
	if m == nil || m.dedupDB == nil || key == "" {
		return
	}
	_, _ = m.dedupDB.Exec(
		`INSERT INTO work_signed_payload_dedup(payload_hash, seen_at) VALUES(?,?)
		 ON CONFLICT(payload_hash) DO UPDATE SET seen_at=excluded.seen_at`,
		key, time.Now().Unix())
}

func (m *workManager) persistResultHash(key string) {
	if m == nil || m.dedupDB == nil || key == "" {
		return
	}
	_, _ = m.dedupDB.Exec(
		`INSERT INTO work_result_hash_dedup(result_hash, seen_at) VALUES(?,?)
		 ON CONFLICT(result_hash) DO UPDATE SET seen_at=excluded.seen_at`,
		key, time.Now().Unix())
}
