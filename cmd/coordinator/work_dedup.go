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
		`CREATE TABLE IF NOT EXISTS worker_payout_lock (
			worker_id TEXT PRIMARY KEY,
			payout_address TEXT NOT NULL
		)`,
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
	m.loadPayoutLocks()
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

func (m *workManager) loadPayoutLocks() {
	if m == nil || m.dedupDB == nil {
		return
	}
	rows, err := m.dedupDB.Query(`SELECT worker_id, payout_address FROM worker_payout_lock`)
	if err != nil {
		log.Printf("work payout lock load: %v", err)
		return
	}
	defer rows.Close()
	type row struct{ id, addr string }
	var loaded []row
	for rows.Next() {
		var id, addr string
		if rows.Scan(&id, &addr) != nil {
			continue
		}
		id = strings.TrimSpace(id)
		addr = strings.TrimSpace(addr)
		if id == "" || addr == "" {
			continue
		}
		loaded = append(loaded, row{id, addr})
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.worker == nil {
		m.worker = map[string]workerPayoutStat{}
	}
	for _, r := range loaded {
		st := m.worker[r.id]
		if strings.TrimSpace(st.PayoutAddress) == "" {
			st.PayoutAddress = r.addr
			m.worker[r.id] = st
		}
	}
	if len(loaded) > 0 {
		log.Printf("work payout locks loaded: %d", len(loaded))
	}
}

// lockedPayoutAddress returns the worker_id binding from memory, falling back
// to the durable lock so a coordinator restart or idle prune cannot free the name.
func (m *workManager) lockedPayoutAddress(workerID string) string {
	if m == nil {
		return ""
	}
	workerID = strings.TrimSpace(workerID)
	m.mu.Lock()
	locked := strings.TrimSpace(m.worker[workerID].PayoutAddress)
	db := m.dedupDB
	m.mu.Unlock()
	if locked != "" || db == nil || workerID == "" {
		return locked
	}
	var addr string
	if err := db.QueryRow(`SELECT payout_address FROM worker_payout_lock WHERE worker_id=?`, workerID).Scan(&addr); err != nil {
		return ""
	}
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	m.mu.Lock()
	st := m.worker[workerID]
	if strings.TrimSpace(st.PayoutAddress) == "" {
		st.PayoutAddress = addr
		if m.worker == nil {
			m.worker = map[string]workerPayoutStat{}
		}
		m.worker[workerID] = st
	}
	m.mu.Unlock()
	return addr
}

// persistPayoutLock records the first payout address for a worker_id. A later
// different address does not replace it.
func (m *workManager) persistPayoutLock(workerID, addr string) {
	if m == nil || m.dedupDB == nil {
		return
	}
	workerID = strings.TrimSpace(workerID)
	addr = strings.TrimSpace(addr)
	if workerID == "" || addr == "" {
		return
	}
	_, _ = m.dedupDB.Exec(
		`INSERT INTO worker_payout_lock(worker_id, payout_address) VALUES(?,?)
		 ON CONFLICT(worker_id) DO NOTHING`,
		workerID, addr)
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
