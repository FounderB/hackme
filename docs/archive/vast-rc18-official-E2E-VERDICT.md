# Vast.ai official-source E2E — 0.1.0-rc18 (Linux CUDA)

**Host:** Vast instance `da853e262f55` · SSH `root@74.126.26.46:6521` (key `hackme_vast_ed25519`)  
**GPU:** NVIDIA GeForce RTX 5090 (32 GB, driver 580.173.02, CUDA compute_90)  
**When:** 2026-10-26 ~20:15–20:21 UTC  
**Channel:** official **apt** `hackme-node` **0.1.0~rc18** + cross-check **GitHub** `hackme_0.1.0-rc18_linux.tar.gz`  
**Commit / build:** dashboard `/api/status` → `version=0.1.0-rc18` · `commit=109793ad29c6` · `build_date=2026-10-26T19:04:55Z`  
**latest.json:** `commit=109793a` · linux tar sha `e163c12e…1e9de0` · deb sha `d9275c5e…c3c16`

Tokens / seeds / admin secrets redacted. Miner **left running**.

---

## Vast readiness: **GO**

| Gate | Result | Evidence |
|------|--------|----------|
| Signed apt install `hackme-node` 0.1.0~rc18 | **PASS** | `apt-get install` ~**10.8 s** after repo add; `ii hackme-node 0.1.0~rc18` |
| apt `Packages` SHA256 == published deb | **PASS** | `d9275c5e2f3f55bfad852dbe95ac8412b91fa2954a11051aefa5e99f3c0c3c16` |
| GitHub linux tar SHA256 vs `latest.json` | **PASS** | download ~**1.5 s** · `e163c12e…1e9de0` |
| Artifacts: `hackme`, `workerpoh-cuda`, `hackme-wasm-check`, `workerfuzz` | **PASS** | present under `/opt/hackme` + `/opt/hackme/bin` |
| Pool token available via official channel | **PASS*** | *not inside `.deb` (by design)*; fetched `https://hackme.tech/dist/pool.miner.token` (same bytes as tar `linux/pool.miner.token`); also in GH tar |
| apt binaries == GH tar binaries | **PASS** | sha256 match for `hackme`, `hackme-wasm-check`, `bin/workerpoh-cuda`, `bin/workerfuzz` |
| CUDA mine multi-GH on real NVIDIA | **PASS** | steady **~35.8 GH/s** (peak ~**36.0**); calibrate **27.17 GH/s** |
| Dig/hybrid: wasm-check sibling + markers | **PASS** | sibling next to cuda; cuda has `-hackme-wasm-validate`; check has `hackme-wasm-validate-child`; hybrid fuzz ON inline |
| Orders / pool marketplace APIs | **PASS** | campaigns/list, fuzz/pool/stats, pool/stats, work/stats all **200** |
| Dashboard `:8080` version+commit | **PASS** | `/api/status` **200** · rc18 / `109793ad29c6` |
| Leave-running healthy | **PASS** | `hackme` + `workerpoh-cuda` up; submits continuing; dash 200 |

**GO reasons:** Customer apt path installs rc18 with wasm-check + CUDA worker; official tar hashes match apt; pool-direct mining hits multi-tens of GH/s on 5090; Dig checker + hybrid path present; marketplace endpoints live. Prefer `curl …/apt/install.sh` (or token URL / tar) so pool token is installed — bare `apt install` alone does not ship the token.

---

## PASS/FAIL table + timings

### Install (official)

| Step | Result | Timing / notes |
|------|--------|----------------|
| SSH direct `:6521` | **PASS** | key OK; proxy not needed |
| apt keyring + `sources.list.d/hackme.list` | **PASS** | **~1.3 s** update; candidate `0.1.0~rc18` |
| `apt-get install -y hackme-node` | **PASS** | **~10.8 s** · fetch ~99.3 MB @ ~12.9 MB/s |
| Packages SHA vs deb | **PASS** | identical `d9275c5e…c3c16` |
| Official token URL | **PASS** | 64 B · sha256 `ef46d06d…ef8a7` (matches tar) |
| GH linux tar download + SHA256 | **PASS** | **~1.5 s** · `e163c12e…1e9de0` |
| Required bins in apt layout | **PASS** | `hackme`, `workerpoh-cuda`, `hackme-wasm-check`, `workerfuzz` (+ `bin/` copies) |
| `pool.miner.token` inside `.deb` | **N/A / expected miss** | Debian packaging omits secrets; `web/site/apt/install.sh` fetches token into `~/.config/hackme/` |

### Configure + Dig / wasm

| Step | Result | Timing / notes |
|------|--------|----------------|
| `setup_hackme_miner.sh` | **PASS** | detected **cuda**; wrote `/opt/hackme/.env` |
| Pool-direct `http://132.243.112.100:18083` | **PASS** | health **200** ttfb ~**0.18 s**; `/api/status` effective URL = direct |
| Unique `WORKER_ID` | **PASS** | `vast-3e262f55-6ea499` |
| Hybrid fuzz env OOTB | **PASS** | `HACKME_WORKER_HYBRID_FUZZ=1` `MODE=inline` |
| Marker `-hackme-wasm-validate` (cuda) | **PASS** | present in binary |
| Marker `hackme-wasm-validate-child` | **PASS** | present in `hackme-wasm-check` |
| `hackme-wasm-check -hackme-wasm-validate` (empty stdin) | **PASS** | **~8 ms** · child marker + `sandbox: wasm size` (expected empty) |
| Checker under `RLIMIT_AS` | **PASS*** | **2 GiB** AS: same OK behavior; **512 MiB** AS: Go runtime fails to reserve pages (too tight for process start — Dig child uses its own budget) |

### Mine + marketplace

| Step | Result | Timing / notes |
|------|--------|----------------|
| `bash start_hackme_miner.sh` | **PASS** | node listen **~0 s** after spawn; dash `/api/status` **200** in **~2.7 s** |
| Worker CUDA up | **PASS** | log birth ~**+7 s** after node pid (`…T201909.log`); `GPU #0 [CUDA compute_90] RTX 5090` |
| Time-to-first-submit | **PASS** | first `submit ok` shortly after calibrate; **≲10–12 s** from `start_hackme_miner` (node 20:19:02 → worker 20:19:09 → immediate submits) |
| Hashrate | **PASS** | calibrate **27.17 GH/s**; steady **~35.8 GH/s** (samples ~35.2–36.0); GPU util ~24–72%, ~159 W, ~38–41 °C |
| Hybrid Dig claim path | **PASS*** | worker log: hybrid ON dig=true; fuzz claim **429** `ghs_priority_defer` (empty/deferred OK). Manual curl Bearer → **403** `claim_pubkey_required` (needs miner pubkey; worker signs) |
| `/api/fuzz/pool/campaigns/list` | **PASS** | **200** · `ok=true` (no active campaign list payload) |
| `/api/fuzz/pool/stats` | **PASS** | **200** · `work_pending=62`, `ghs_priority_enabled=true` |
| `/api/pool/stats` | **PASS** | **200** · pool hashrate ~**3.7e10** H/s class |
| `/api/work/stats` | **PASS** | **200** · large live stats blob |
| Dashboard version/commit | **PASS** | `0.1.0-rc18` / `109793ad29c6` |

**Operator wall clock (SSH → leave-running):** ~**6–8 min**.

---

## Notes / non-blockers

1. **Bare `apt install` vs customer install.sh:** Package postinst points at `~/.config/hackme/pool.miner.token` but does not download it. Official `https://hackme.tech/apt/install.sh` (or GH/tar / `https://hackme.tech/dist/pool.miner.token`) is required for mining OOTB.
2. **Dashboard `mining: false`:** Desktop/node flag; CUDA worker still submits (do not treat as failure).
3. **Fuzz marketplace empty:** `429 ghs_priority_defer` / no running campaigns — expected with canary CPU farm disabled; hybrid path still armed.
4. **Stock CF URL** left unused; this run forced pool-direct for reliability.
5. **Root on Vast:** node warns about root-owned seeds; fine for ephemeral GPU rental.

---

## Machine final state (leave-running)

| Item | Value |
|------|--------|
| Install | apt `/opt/hackme` · `hackme-node 0.1.0~rc18` |
| Processes | `hackme` + `workerpoh-cuda` (GPU 0) |
| WORKER_ID | `vast-3e262f55-6ea499` |
| Coordinator | pool-direct `http://132.243.112.100:18083` |
| Hybrid fuzz | ON / inline |
| Hashrate | **~35.8 GH/s** CUDA RTX 5090 |
| Dashboard | `http://127.0.0.1:8080/` · `/api/status` 200 |
| Key bins | `hackme-wasm-check`, `workerfuzz`, `workerpoh-cuda` present (apt ≡ GH tar) |

### Restart

```bash
bash /opt/hackme/start_hackme_miner.sh
```

SSH: `ssh -i ~/.ssh/hackme_vast_ed25519 -o IdentitiesOnly=yes -p 6521 root@74.126.26.46`  
Optional local dash: add `-L 8080:localhost:8080`.
