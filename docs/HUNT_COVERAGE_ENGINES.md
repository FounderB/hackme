# Coverage-guided Hunt engines (libFuzzer + AFL++)

Distributed **coverage-guided Hunt** — not an OSS-Fuzz replacement, and **not** “better than OSS-Fuzz.” Dig stays WASM + own mutator; AFL++/libFuzzer never enter Dig.

**Inventory / Heavywire:** catalog + customer inventory fleet leases use **libFuzzer** + published ASAN stdin harness by default; Deep/long profile prefers **AFL++** when `afl-fuzz` is present.

## Engines

| Engine | Role | Claim `hunt_engine` |
|--------|------|---------------------|
| **libFuzzer** | Fleet default (Lite/Standard/inventory) | `libfuzzer` |
| **AFL++** | Deep / long profile when AFL present (afl-cc instrumented harness) | `aflpp` |
| **byte** | Explicit opt-in fallback only (`HACKME_HUNT_ENGINE_BYTE_FALLBACK`) | `byte` |

## Claim / submit contract

**Claim** (Hunt):

- `hunt_engine` / `engine`: `libfuzzer` \| `aflpp` \| `byte`
- `engine_lease_sec`: wall seconds for coverage engines (fleet ~60, deep ~180, cap 300)
- `engine_profile`: `fleet` \| `deep`
- `exec_per_unit`: `1` for coverage engines (one engine window)
- Worker advertises `hunt_engines` (CSV)

**Submit**:

- `hunt_engine` / `engine`, `engine_version` (report)
- `segment_exec_done=1` for coverage engines
- `edges_touched` may be sent but is **untrusted for Hunt pay**
- Corpus novelty + crashes via `POST /api/fuzz/work/corpus_delta` while lease held

## Pay model (coordinator-proven)

1. Worker runs LF/AFL for `engine_lease_sec`, uploads **session-new** corpus_delta + crash artifacts.
2. Coordinator **ASAN-oneshots** each new seed under the campaign harness (`content_sha256`-bound). Crashes are ASAN-replayed separately.
3. After a CLEAN harness exec, coordinator runs **`afl-showmap -b`** on an **afl-cc + ASAN** instrumented catalog harness when available; for **custom inventory** Hunt it materializes the published harness by `harness_hash` / `content_sha256` (non-instrumented ASAN → empty showmap is an honest floor). Structural/fingerprint buckets alone **never** unlock PayRun.
4. Settle:
   - **Full PayRun** — showmap-proven new edges and/or ASAN crash
   - **Floor** — CLEAN without proven map delta

Cap: `HACKME_HUNT_ENGINE_PROVE_MAX` default **16** seeds per delta window (extra seeds store, may not all count toward edges).

## Soft-skip / byte fallback

- Missing runner / missing **bwrap** (when `HACKME_HUNT_ISOLATE_REQUIRE=1`) / unknown catalog target without published harness → `hunt_engine_soft_skip` (release lease — **not** 1ms CLEAN success).
- Inventory fleet uses **libFuzzer subprocess + published ASAN stdin harness** (~60s wall); never silent byte dilution.
- Coordinator rejects coverage-engine submits with `duration_ms` ≪ lease wall (`hunt_engine_wall_too_short`; escape `HACKME_HUNT_ENGINE_MIN_WALL_MS=0`).
- Empty corpus after a finished LF wall → CLEAN floor (not soft-skip).
- `HACKME_HUNT_ENGINE_BYTE_FALLBACK` default **OFF**. Deep/Heavy never fall back unless `HACKME_HUNT_ENGINE_BYTE_FALLBACK_DEEP=1`.

## Install (no git clone required)

```bash
bash scripts/ops/install_hunt_engines.sh --desktop
sudo bash scripts/ops/install_hunt_engines.sh --system --prefix /opt/hackme
```

Ships clang + AFL++ (`afl-fuzz`, `afl-clang-fast`, `afl-showmap`) from release tarball / bundle. Sets `HACKME_AFL_FUZZ`.

## Env knobs

| Env | Default | Meaning |
|-----|---------|---------|
| `HACKME_HUNT_COVERAGE_ENGINE` | ON | Master switch |
| `HACKME_HUNT_ENGINE` | (auto) | Force `libfuzzer`/`aflpp`/`byte` |
| `HACKME_HUNT_ENGINE_DEEP_AFL` | ON | Heavy/standard → aflpp |
| `HACKME_HUNT_ENGINE_LEASE_SEC` | — | Override wall |
| `HACKME_HUNT_ENGINE_BYTE_FALLBACK` | **OFF** | Soft-skip → byte |
| `HACKME_HUNT_ENGINE_BYTE_FALLBACK_DEEP` | OFF | Allow byte on Deep |
| `HACKME_HUNT_ISOLATE_REQUIRE` | **ON** | Fail closed without bwrap |
| `HACKME_HUNT_ENGINE_PROVE_MAX` | 16 | Max seeds proven per delta |
| `HACKME_POOL_SEED_FROM_RESEARCH` | OFF | Dig seed feed — **do not enable** |

## Isolation

Native LF/AFL/showmap under **`bwrap --unshare-net`** + `RLIMIT_AS` when required (preferred). Bare exec only if `HACKME_HUNT_ISOLATE_REQUIRE=0` or `AllowBare` (tests). **Hub ops may set `HACKME_HUNT_ISOLATE_REQUIRE=0`** when user namespaces / bwrap are broken on the host — soft-skip otherwise would starve Hunt leases.

Miners do **not** `git clone` AFL++. Install engines via `install_hunt_engines.sh` (release tarball / bundle).

## Related

- [POOL_WORKER_RESEARCH_SLOT.md](POOL_WORKER_RESEARCH_SLOT.md)
- [HUNT_ECONOMICS.md](HUNT_ECONOMICS.md)
- [HUNT_VS_LIBFUZZER.md](HUNT_VS_LIBFUZZER.md)
- [FUZZ_PRODUCT_GUIDE.md](FUZZ_PRODUCT_GUIDE.md)
