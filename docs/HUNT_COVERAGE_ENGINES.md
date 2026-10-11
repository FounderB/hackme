# Coverage-guided Hunt engines (libFuzzer + AFL++)

Distributed **coverage-guided Hunt** — not an OSS-Fuzz replacement. Dig stays WASM + own mutator; AFL++/libFuzzer never enter Dig.

## Engines

| Engine | Role | Claim `hunt_engine` |
|--------|------|---------------------|
| **libFuzzer** | Fleet default | `libfuzzer` |
| **AFL++** | Deep / long profile (CmpLog/dict/corpus) | `aflpp` |
| **byte** | Fallback without clang / coverage runner | `byte` |

## Claim / submit contract

**Claim** (Hunt):

- `hunt_engine` / `engine`: `libfuzzer` \| `aflpp` \| `byte`
- `engine_lease_sec`: wall seconds for coverage engines (fleet ~60, deep ~180, cap 300)
- `engine_profile`: `fleet` \| `deep`
- `exec_per_unit`: `1` for coverage engines (one engine window)
- Worker advertises `hunt_engines` (CSV) + existing `hunt_harness_exec=libfuzzer_oneshot`

**Submit**:

- `hunt_engine` / `engine`, `engine_version` (report)
- `segment_exec_done=1` for coverage engines
- `edges_touched` may be sent but is **untrusted for Hunt pay**
- Corpus novelty + crashes via `POST /api/fuzz/work/corpus_delta` while lease held

## Pay model (coordinator-proven)

1. Worker runs LF/AFL for `engine_lease_sec`, uploads **session-new** corpus_delta + crash artifacts (pre-lease corpus snapshot).
2. Coordinator **ASAN-oneshots** each new seed under the campaign harness (`content_sha256`-bound) before any coverage accounting. Seeds that do not execute in the harness never unlock edge pay. Crashes are ASAN-replayed separately.
3. After a CLEAN harness exec, coordinator records campaign coverage buckets and counts **new** edges. Miner `edges_touched` is ignored.
4. Settle:
   - **Full PayRun** — harness-proven new edges and/or ASAN crash
   - **Floor** — CLEAN without proven delta (no full shard settle / pennies posture)

## Soft-skip

Workers without the claimed runner release the lease (`hunt_engine_soft_skip`). Empty corpus after a finished libFuzzer wall does **not** soft-skip — the shard submits CLEAN floor and re-seeds boot starters (avoids lease churn). Soft-skip remains for missing clang/afl/target/build preflight only. With `HACKME_HUNT_ENGINE_BYTE_FALLBACK` (default ON), legacy workers omitting `hunt_engines` fall back to byte Hunt.

## Install (no git clone required)

```bash
bash scripts/ops/install_hunt_engines.sh --desktop
# or
sudo bash scripts/ops/install_hunt_engines.sh --system --prefix /opt/hackme
```

Ships clang (apt) + AFL++ from a **release tarball** or pre-vendored bundle under `third_party/aflpp/` / `dist/toolchains/`. Sets `HACKME_AFL_FUZZ`.

## Env knobs

| Env | Default | Meaning |
|-----|---------|---------|
| `HACKME_HUNT_COVERAGE_ENGINE` | ON | Master switch |
| `HACKME_HUNT_ENGINE` | (auto) | Force `libfuzzer`/`aflpp`/`byte` |
| `HACKME_HUNT_ENGINE_DEEP_AFL` | ON | Heavy/standard → aflpp |
| `HACKME_HUNT_ENGINE_LEASE_SEC` | — | Override wall |
| `HACKME_HUNT_ENGINE_BYTE_FALLBACK` | ON | Soft-skip → byte |
| `HACKME_WORKER_HUNT_ENGINES` | detect | Claim CSV override |
| `HACKME_POOL_SEED_FROM_RESEARCH` | OFF | Dig seed feed — **do not enable** |

## Isolation

Native harness / fuzzer runs under wall-clock + optional `bwrap --unshare-net` (no net). Coordinator crash replay uses the same Hunt ASAN path (content_sha256-bound harness).

## Related

- [POOL_WORKER_RESEARCH_SLOT.md](POOL_WORKER_RESEARCH_SLOT.md) — byte Hunt supplemental LF (skipped when engine shard already ran LF)
- [HUNT_ECONOMICS.md](HUNT_ECONOMICS.md) — escrow / 50/50
- [HUNT_VS_LIBFUZZER.md](HUNT_VS_LIBFUZZER.md) — honesty vs OSS-Fuzz
