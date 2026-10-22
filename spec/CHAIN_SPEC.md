# HackMe Chain Specification (MVP)

## Network ID

- **Chain ID (boolean):** `hackme-dev-mainnet` (constant in code, duplicated in `meta.chain_id` after genesis).

## Block

JSON field (stored in SQLite entirely in column `json` of table `blocks`).

| Field | Type | Description |
|------|-----|----------|
| `index` | uint64 | Height; genesis = 0 |
| `timestamp_unix` | int64 | Unix time seconds |
| `prev_hash` | string | hex SHA-256; genesis has 64 characters `0` |
| `hash` | string | hex SHA-256 header (see below) |
| `miner_pubkey_ed25519` | string | hex-public key of the node (optional for old unsigned blocks) |
| `miner_sig_ed25519` | string | hex signature Ed25519 on line `hash` (optional for older unsigned blocks) |
| `nonce` | uint64 | For genesis 0; reserved for PoW/PoH |
| `miner_address` | string | MVP: string like `HMC-…` (node ​​ID) |
| `task` | object | Problem (see below) |

## Task

| Field | Type | Description |
|------|-----|----------|
| `id` | string | Unique task id |
| `kind` | string | In genesis: `genesis_v1`; for PoH blocks: `poh_wasm_v1` |
| `payload` | bytes (base64 in JSON) | Arbitrary data; genesis has JSON text in bytes |
| `target_hint` | string | Human-readable tooltip for PoH |

## Hashing (normative for MVP)

1. Take the structure **without the field `hash`** (and without `miner_pubkey_ed25519` / `miner_sig_ed25519`), serialized in JSON with fixed keys (`index`, `timestamp_unix`, `prev_hash`, `nonce`, `miner_address`, `task`) - see `internal/block/hash.go`.
2. **SHA-256** is calculated from these bytes.
3. `hash` = hex string (lower case), length 64 characters.

Any change to header fields after calculation requires a recalculation of `hash`. The block signature (`miner_sig_ed25519`) is validated separately against the bytes of the string `hash`.

## Genesis reward

- **0 HMC** for block #0 to address `miner_address` (production policy: no starting emission).

## WASM lock and PoH blocks

**PoH formula versions (height-gated, no genesis wipe):**

| Height gate | Kind | Work proof |
|-------------|------|------------|
| `< PoHFormulaV3ActivationHeight` (default **485000** as of rc17.4; lab: `HACKME_POH_V3_ACTIVATION_HEIGHT`) | `synthetic_poh_v1` | `eval(n)=n*7+13`, accept iff `eval % M == 0` |
| `≥ PoHFormulaV3ActivationHeight` | `synthetic_poh_v3` | `work = uint64_le(Blake3(prev_hash_hex_ascii_lower ‖ miner_address_utf8 ‖ nonce_u64_le)[0:8])`, accept iff `work % M == 0` |

Endianness for v3 is **little-endian** for both the nonce bytes and the work uint64 (locked by tests). Block signatures remain the **node** Ed25519 signer; Blake3 binds `miner_address` in the work proof only. Order WASM `check(n)` stays a separate gate after the native PoH condition.

The miner searches for `nonce` **on the CPU** (v1 algebraic or v3 Blake3) or, when building **`-tags cuda`** / **`-tags opencl`**, batches on GPU backends (`internal/gpupoh`, `kernels/poh_search.cu` — v1 kernel + Blake3 v3 path). **CUDA** is preferred over **OpenCL** if both tags are combined (see `HACKME_FORCE_OPENCL`). For paid orders, the found nonce is also checked with one WASM `check` call.

After a successful solution, the node writes the **next block** to SQLite: in the header `nonce` = found value, in `task` - `kind: poh_wasm_v1` and JSON-payload with `nonce`, `eval`, `mod`, `formula`; when mining an **order** from table `tasks`, **`order_task_id`** (= `id` order) is added to the payload. `prev_hash` and `hash` (tip) are updated like a normal chain. The reward (**+0.01 HMC** by default or `reward_hmc` of the order/manifest) is credited in the same transaction; The order progress counter in `tasks` is updated atomically with the block.

## PoH Difficulty (`meta.poh_target_mod`)

- Changing complexity parameters in the code **does not rewrite** the existing SQLite database. To test new rules you need a **clean start**: remove `data/` or `data/hackme.db` and run genesis again (see root **README.md**).
- After **genesis**, the table `meta` stores the pair `key = poh_target_mod`, `value` - the decimal string `uint64`: the active module `M`, for which the solution `eval(nonce) % M == 0` is sought.
- If there is no key (old databases before this version), the node treats `M` as **1000000** until the first successful write of the PoH block.
- In the JSON of the PoH block, the field `task.payload.mod` is **equal** to the `M` at which this decision was made (for audit and revalidation).
- After each accepted PoH block, `poh_target_mod` is written **in the same transaction** as the block insertion. **Difficulty recalculation** is performed only when the height of the new block is a multiple of **5** (blocks 5, 10, 15...): the Unix time difference between the new block and the block with index `height−5` is taken and compared with the target calendar window **5×30 s = 150 s**. Formula: `M_next = M_prev * T_ideal / T_actual` (then clamp): faster than the target → **more** `M` (harder), slower → **smaller** `M`. Between such eras `M` **does not change**. The range `M` is set in `internal/chain/retarget.go`.
- Pre-v3 algebraic `eval(n)=7n+13`: **M must not be divisible by 7** or the equation is unsolvable. Chain `ClampPoHTargetMod` still nudges `M` by ±1 when needed (report #29). After Blake3 v3 activation any in-bounds `M` is solvable; the pool coordinator clamps **bounds only** (no 7-nudge).

## Orders in the database (`tasks`) and priority

Through **`POST /api/tasks`** the manifest is loaded into SQLite. As long as there is an order with status **`open`** and **`reward_hmc` > 0**, the miner takes it **before** synthetics and files `./tasks` (FIFO by `created_at`). After the required number of PoH blocks from `order_task_id`, the order moves to **`completed`**.

- **Escrow:** when creating an order from the node wallet (line `wallet`, `id=1`) **`reward_hmc × target_solves`** HMC is written off; the amount is stored in column **`prepaid_hmc`** (audit). You need **genesis** already completed and sufficient balance; otherwise the order is rejected (**HTTP 402**).
- **`payer_ref`:** optional line in the manifest (up to 256 Unicode runes) - external customer identifier / link to account / hash for reporting; saved in **`tasks.payer_ref`**.
- **Response signature:** the body of the request (JSON bytes as sent by the client) is signed with the node key **Ed25519**; the response returns **`signature_ed25519`** and **`signing_public_key_ed25519`** (see `docs/API.md`). The key is created in **`data/node_ed25519.seed`** upon first launch.
- **WASM artifact on disk:** optional **`wasm_artifact_path`** - path **relative to the artifacts root** (by default the directory **`tasks/artifacts/`** from the working directory of the process; override: **`HACKME_TASK_ARTIFACT_DIR`**). Required **`artifact_hash`**: **64 hex characters SHA-256** of the contents of the `.wasm` file (lower case). The node reads the file, checks the hash, validates the module (`check(i64)->i32`), then the miner uses the same bytes as with **`wasm_check_hex`**. You cannot specify **`wasm_artifact_path`** and **`wasm_check_hex`** at the same time.
- **Cancellation and refund escrow:** in MVP **no** - no HTTP endpoint, no return **`prepaid_hmc`** to the wallet after debiting. The order goes up to **`completed`** (or remains **`open`** until progress is made). The string status **`cancelled`** in types is reserved for a future protocol (partial payments, timeouts, coordination between nodes - a separate phase).

## Local task manifest (`./tasks/*.json`)

Knot Raises `TaskProvider`: By default the chain uses **built-in** synthetics (`InternalTaskProvider`) unless there is an open paid order in `tasks`. The **`tasks/`** folder (next to the process's working directory) may contain one or more `*.json` files; the **newest file by modification time** is selected, the name with the prefix **`_`** is ignored.

Minimum JSON fields:

| Field | Type | Description |
|------|-----|----------|
| `id` | string | Mandatory task identifier (for metrics and logs). |
| `kind` | string | By default, `synthetic_poh_v1` is the same PoH as the built-in provider (other values ​​are ignored for now when selecting a file). |
| `artifact_hash` | string | If **`wasm_artifact_path`** is specified - **required**: SHA-256 of file `.wasm` (64 hex). Otherwise optional (audit); if empty when ordering without a file path, the hash of the JSON manifest can be saved in the database. |
| `wasm_artifact_path` | string | Optional: relative path to `.wasm` under the artifacts root (`tasks/artifacts/` or `HACKME_TASK_ARTIFACT_DIR`). Mutually exclusive with **`wasm_check_hex`**. |
| `timeout_ms` | int | Optional: timeout hint for future performers. |
| `reward_hmc` | number | Optional: if `> 0`, the node uses this value as a reward for a successful PoH block instead of a miner default. |
| `wasm_check_hex` | string | Optional: hex-coded WASM module with export **`check(i64)->i32`**; after the native condition `eval(n)%M==0` `check(n)` is called and the block is accepted only if the result ≠0. Validation when uploading a manifest (order or file). |

The `task` field **within the block** still describes the specific PoH solution (`poh_wasm_v1`); The manifest specifies the **order context** on the node side (table **`tasks`** in SQLite).

## HMC transfers (transfer tx v1)

Transfer transactions in v1 are **account-based** (not UTXO) and use a separate user signature.

### Transfer transaction format

The minimum unit of HMC is called **Kapa**: `1 HMC = 100,000,000 Kapa`.

| Field | Type | Description |
|------|-----|----------|
| `tx_type` | string | Required `transfer_v1` |
| `from` | string | Sender Address (`HMC-...`) |
| `to` | string | Recipient address (`HMC-...`), must be different from `from` |
| `amount_units` | uint64 | Amount in minimum units **Kapa** (not float) |
| `fee_units` | uint64 | Commission in minimum units **Kapa** |
| `nonce` | uint64 | Sender's outgoing transaction number |
| `timestamp_unix` | int64 | tx creation time (unix sec) |
| `memo` | string | Optional, up to 256 bytes UTF-8 |
| `pubkey_ed25519` | string | Hex public key (32 bytes) |
| `sig_ed25519` | string | Hex signature (64 bytes) by canonical bytes tx |

### Canonical bytes for signature and tx hash

1. For the signature, a JSON structure is taken **without** `sig_ed25519`, with the keys strictly in order:
   `tx_type`, `from`, `to`, `amount_units`, `fee_units`, `nonce`, `timestamp_unix`, `memo`, `pubkey_ed25519`.
2. `tx_hash = sha256(canonical_json_bytes)` (hex lowercase) is calculated.
3. Signature `sig_ed25519` is considered according to the same `canonical_json_bytes`.

### Validation rules

- `tx_type` should be `transfer_v1`.
- `amount_units > 0`.
- `fee_units >= min_fee_units` (from policy network/node).
- `from != to`.
- `pubkey_ed25519` and `sig_ed25519` must be the correct length and format.
- `from` must match the address deterministically derived from `pubkey_ed25519`.
- `ed25519.Verify(pubkey, canonical_json_bytes, sig) == true`.
- `nonce` must match the sender's expected `next_nonce`.
- The sender's balance must cover `amount_units + fee_units`.

### Rejected reason codes

- `invalid_tx_type`
- `invalid_amount`
- `invalid_fee`
- `invalid_nonce`
- `invalid_signature`
- `address_pubkey_mismatch`
- `insufficient_balance`
- `duplicate_or_replay`
- `tx_too_old`
- `tx_too_far_in_future`
