// Package checkpoint builds, signs and verifies external chain checkpoints.
//
// One canonical node produces every block, so nothing outside the node proves that past
// history was not rewritten. A checkpoint commits to the tip block hash and to the full
// ledger state at that tip; publishing it in a public git repo and anchoring its hash in
// Bitcoin via OpenTimestamps makes any later rewrite of history detectable by anyone.
package checkpoint

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/zeebo/blake3"

	"hackme/internal/nodecrypto"
)

// FormatVersion is bumped whenever the payload or the state encoding changes.
const FormatVersion = 1

// stateDomain prefixes the state stream so hashes never collide with other blake3 uses.
const stateDomain = "hackme-checkpoint-state-v1\n"

// metaUnitKeys are the integer economic counters committed to the state hash
// (float mirrors such as econ_total_minted_hmc are derived and excluded).
var metaUnitKeys = []string{
	"econ_order_escrow_units",
	"econ_total_burned_units",
	"econ_total_minted_units",
	"hms_total_burned_units",
	"hms_total_minted_units",
	"sup_total_minted_units",
}

// Payload is the signed content. Field order is fixed by the struct, so json.Marshal is canonical.
type Payload struct {
	Version            int               `json:"version"`
	ChainID            string            `json:"chain_id"`
	Height             uint64            `json:"height"`
	BlockHash          string            `json:"block_hash"`
	BlockTimestampUnix int64             `json:"block_timestamp_unix"`
	StateHash          string            `json:"state_hash"`
	Accounts           int               `json:"accounts"`
	Escrows            int               `json:"escrows"`
	Totals             map[string]uint64 `json:"totals"`
	PrevCheckpointHash string            `json:"prev_checkpoint_hash"`
	CreatedUnix        int64             `json:"created_unix"`
	SignerPubKey       string            `json:"signer_pubkey_ed25519"`
	SignerAddress      string            `json:"signer_address"`
}

// Checkpoint is a signed payload as published (one JSON file per checkpoint).
type Checkpoint struct {
	Payload
	Sig string `json:"sig_ed25519"`
}

// SignedBytes is the exact byte string the signature covers.
func (p Payload) SignedBytes() ([]byte, error) {
	return json.Marshal(p)
}

// Hash identifies a checkpoint (payload + signature) for prev_checkpoint_hash links.
func (c Checkpoint) Hash() (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	sum := blake3.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// Snapshot is the ledger state read inside one read transaction.
type Snapshot struct {
	ChainID            string
	Height             uint64
	BlockHash          string
	BlockTimestampUnix int64
	StateHash          string
	Accounts           int
	Escrows            int
	Totals             map[string]uint64
}

// ReadSnapshot reads tip + full state in a single read-only transaction so the tip and the
// balances describe the same instant even while the node keeps appending blocks (WAL snapshot).
func ReadSnapshot(ctx context.Context, db *sql.DB) (Snapshot, error) {
	var snap Snapshot
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return snap, err
	}
	defer func() { _ = tx.Rollback() }()

	var tipJSON string
	if err := tx.QueryRowContext(ctx,
		`SELECT block_index, hash, json FROM blocks ORDER BY block_index DESC LIMIT 1`).
		Scan(&snap.Height, &snap.BlockHash, &tipJSON); err != nil {
		return snap, fmt.Errorf("checkpoint: read tip: %w", err)
	}
	var tip struct {
		Timestamp int64 `json:"timestamp_unix"`
	}
	if err := json.Unmarshal([]byte(tipJSON), &tip); err != nil {
		return snap, fmt.Errorf("checkpoint: decode tip block: %w", err)
	}
	snap.BlockTimestampUnix = tip.Timestamp

	snap.ChainID = "hackme-dev-mainnet"
	var chainID sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = 'chain_id'`).Scan(&chainID); err == nil && strings.TrimSpace(chainID.String) != "" {
		snap.ChainID = strings.TrimSpace(chainID.String)
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return snap, err
	}

	h := blake3.New()
	if _, err := io.WriteString(h, stateDomain); err != nil {
		return snap, err
	}
	if snap.Accounts, err = writeAccounts(ctx, tx, h); err != nil {
		return snap, err
	}
	if snap.Escrows, err = writeEscrows(ctx, tx, h); err != nil {
		return snap, err
	}
	if snap.Totals, err = writeMetaTotals(ctx, tx, h); err != nil {
		return snap, err
	}
	snap.StateHash = hex.EncodeToString(h.Sum(nil))
	return snap, nil
}

type rowQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func writeAccounts(ctx context.Context, q rowQuerier, w io.Writer) (int, error) {
	rows, err := q.QueryContext(ctx, `
SELECT address, balance_units, next_nonce,
       COALESCE(balance_sup_units,0), COALESCE(sup_next_nonce,0),
       COALESCE(balance_hms_units,0), COALESCE(hms_next_nonce,0)
FROM accounts ORDER BY address`)
	if err != nil {
		return 0, fmt.Errorf("checkpoint: read accounts: %w", err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var addr string
		var bal, nonce, sup, supNonce, hms, hmsNonce uint64
		if err := rows.Scan(&addr, &bal, &nonce, &sup, &supNonce, &hms, &hmsNonce); err != nil {
			return 0, err
		}
		if strings.ContainsAny(addr, "|\n") {
			return 0, fmt.Errorf("checkpoint: address with separator: %q", addr)
		}
		if _, err := fmt.Fprintf(w, "A|%s|%d|%d|%d|%d|%d|%d\n", addr, bal, nonce, sup, supNonce, hms, hmsNonce); err != nil {
			return 0, err
		}
		n++
	}
	return n, rows.Err()
}

func writeEscrows(ctx context.Context, q rowQuerier, w io.Writer) (int, error) {
	var exists int
	if err := q.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='fuzz_campaign_escrow'`).Scan(&exists); err != nil {
		return 0, err
	}
	if exists == 0 {
		return 0, nil
	}
	rows, err := q.QueryContext(ctx, `
SELECT campaign_id, budget_units, runs_pool_units, bounty_pool_units, runs_paid_units,
       bounty_paid_units, COALESCE(crash_bonus_paid_units,0), refunded_bounty_units, status
FROM fuzz_campaign_escrow ORDER BY campaign_id`)
	if err != nil {
		return 0, fmt.Errorf("checkpoint: read escrows: %w", err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id, status string
		var budget, runsPool, bountyPool, runsPaid, bountyPaid, crashPaid, refunded uint64
		if err := rows.Scan(&id, &budget, &runsPool, &bountyPool, &runsPaid, &bountyPaid, &crashPaid, &refunded, &status); err != nil {
			return 0, err
		}
		if strings.ContainsAny(id+status, "|\n") {
			return 0, fmt.Errorf("checkpoint: escrow field with separator: %q", id)
		}
		if _, err := fmt.Fprintf(w, "E|%s|%d|%d|%d|%d|%d|%d|%d|%s\n",
			id, budget, runsPool, bountyPool, runsPaid, bountyPaid, crashPaid, refunded, status); err != nil {
			return 0, err
		}
		n++
	}
	return n, rows.Err()
}

func writeMetaTotals(ctx context.Context, q rowQuerier, w io.Writer) (map[string]uint64, error) {
	keys := append([]string(nil), metaUnitKeys...)
	sort.Strings(keys)
	out := make(map[string]uint64, len(keys))
	for _, k := range keys {
		var raw sql.NullString
		err := q.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, k).Scan(&raw)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		var v uint64
		if s := strings.TrimSpace(raw.String); s != "" {
			if v, err = strconv.ParseUint(s, 10, 64); err != nil {
				return nil, fmt.Errorf("checkpoint: meta %s not an integer: %q", k, s)
			}
		}
		out[k] = v
		if _, err := fmt.Fprintf(w, "M|%s|%d\n", k, v); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Signer is the subset of nodecrypto.Signer used here.
type Signer interface {
	PublicKeyHex() string
	Address() string
	SignHex(message []byte) string
}

// New builds and signs a checkpoint for snap, linked to prev (nil for the first checkpoint).
func New(snap Snapshot, prev *Checkpoint, createdUnix int64, s Signer) (Checkpoint, error) {
	var c Checkpoint
	if s == nil {
		return c, errors.New("checkpoint: signer required")
	}
	prevHash := ""
	if prev != nil {
		if snap.Height <= prev.Height {
			return c, fmt.Errorf("checkpoint: tip %d not above previous checkpoint %d", snap.Height, prev.Height)
		}
		if snap.ChainID != prev.ChainID {
			return c, fmt.Errorf("checkpoint: chain_id %q differs from previous %q", snap.ChainID, prev.ChainID)
		}
		h, err := prev.Hash()
		if err != nil {
			return c, err
		}
		prevHash = h
	}
	c.Payload = Payload{
		Version:            FormatVersion,
		ChainID:            snap.ChainID,
		Height:             snap.Height,
		BlockHash:          snap.BlockHash,
		BlockTimestampUnix: snap.BlockTimestampUnix,
		StateHash:          snap.StateHash,
		Accounts:           snap.Accounts,
		Escrows:            snap.Escrows,
		Totals:             snap.Totals,
		PrevCheckpointHash: prevHash,
		CreatedUnix:        createdUnix,
		SignerPubKey:       s.PublicKeyHex(),
		SignerAddress:      s.Address(),
	}
	msg, err := c.Payload.SignedBytes()
	if err != nil {
		return c, err
	}
	c.Sig = s.SignHex(msg)
	return c, nil
}

// VerifySignature checks the Ed25519 signature and that signer_address matches the key.
func (c Checkpoint) VerifySignature() error {
	pub, err := hex.DecodeString(c.SignerPubKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return errors.New("checkpoint: bad signer public key")
	}
	if got := nodecrypto.AddressForPublicKey(pub); got != c.SignerAddress {
		return fmt.Errorf("checkpoint: signer_address %s does not match key (%s)", c.SignerAddress, got)
	}
	sig, err := hex.DecodeString(c.Sig)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return errors.New("checkpoint: bad signature encoding")
	}
	msg, err := c.Payload.SignedBytes()
	if err != nil {
		return err
	}
	if !ed25519.Verify(pub, msg, sig) {
		return errors.New("checkpoint: signature does not verify")
	}
	if c.Version != FormatVersion {
		return fmt.Errorf("checkpoint: unsupported version %d", c.Version)
	}
	return nil
}

// VerifyChain checks signatures, a single signer, strictly increasing heights and
// prev_checkpoint_hash links for checkpoints sorted by height.
func VerifyChain(cps []Checkpoint) error {
	for i, c := range cps {
		if err := c.VerifySignature(); err != nil {
			return fmt.Errorf("checkpoint at height %d: %w", c.Height, err)
		}
		if i == 0 {
			continue
		}
		prev := cps[i-1]
		if c.Height <= prev.Height {
			return fmt.Errorf("checkpoint heights not increasing: %d after %d", c.Height, prev.Height)
		}
		if c.ChainID != prev.ChainID {
			return fmt.Errorf("checkpoint at height %d: chain_id changed", c.Height)
		}
		if c.SignerPubKey != prev.SignerPubKey {
			return fmt.Errorf("checkpoint at height %d: signer key changed", c.Height)
		}
		want, err := prev.Hash()
		if err != nil {
			return err
		}
		if c.PrevCheckpointHash != want {
			return fmt.Errorf("checkpoint at height %d: prev_checkpoint_hash %s, want %s", c.Height, c.PrevCheckpointHash, want)
		}
	}
	return nil
}
