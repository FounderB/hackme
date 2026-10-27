# Vast.ai E2E — 0.1.0-rc18 (2026-10-27)

**Host:** Vast · SSH `root@74.126.26.46:5270` (proxy `ssh5.vast.ai:36059`) · key `hackme-vast@kapa-20261024`  
**GPU:** NVIDIA GeForce RTX 5090 · driver 580.173.02  
**Channel:** official `curl …/apt/install.sh` → then tar overlay for missing bins (see FAIL below)

Tokens/seeds redacted. Miner **left running**.

---

## Verdict: **CONDITIONAL GO**

| Gate | Result | Notes |
|------|--------|-------|
| SSH direct `:5270` | **PASS** | key matches `hackme_vast_ed25519` |
| Official apt `install.sh` | **PASS*** | ~4 s · sha `adbb7440…` (22 MB **incomplete** deb) |
| Token via install.sh | **PASS** | `~/.config/hackme/pool.miner.token` 64 B · `ef46d06d…ef8a7` |
| Bare apt bins: cuda / wasm-check / workerfuzz / libnvrtc | **FAIL** | missing from 22 MB deb; **fixed** → new deb **74 MB** sha `a0a64a26…` published to apt/GH |
| GH linux tar overlay | **PASS** | sha `8165ea60…` · cuda + wasm-check + workerfuzz + `lib/` |
| CUDA mine ~GH/s | **PASS** | calibrate **27.11** · steady **~35.5–35.8 GH/s** · GPU ~79 % / ~206 W |
| Hybrid Dig path armed | **PASS** | `HYBRID_FUZZ=1` inline · claims reach coordinator |
| Dig campaign create | **PASS** | `dig-vast-20261027T083429Z` (+ gate variant) via VPS loopback admin |
| Dig shard `runs_ok` | **FAIL** | many `failed_checks` — coordinator wasm-validate reject (`invalid campaign wasm` / child marker race); **not** a Vast GPU failure |
| Hunt campaign create + complete | **PASS** | `hunt-vast-1793090094` · **runs_ok=24 / failed=0** · status **completed** |
| Hunt engines (clang/AFL++/bwrap) | **PASS*** | installed; bwrap probe fails nested userns (soft-skip STRICT) — Hunt still completed via pool |
| Orders APIs on local node | **PASS** | `/api/fuzz/campaigns` 200 |
| Fixed apt live on hackme.tech | **PASS** | Packages Size **74276138** · SHA256 `a0a64a26…` |

\* First apt install proved the **broken** package; customer path after republish should be re-smoked once (optional).

---

## Root cause: incomplete deb

`build_deb_from_dist.sh` preferred `dist/release_*/linux/` which lacked `workerpoh-cuda`, `workerfuzz`, `hackme-wasm-check`, `lib/`. Tar was complete. Deb rebuilt from completed tree → **71–74 MB**, synced to apt + GH release.

---

## Leave-running

| Item | Value |
|------|--------|
| WORKER | `worker-04353f1b8764` · CUDA 5090 |
| Coord | pool-direct `http://132.243.112.100:18083` |
| Hashrate | **~35.6 GH/s** |
| Hunt | **completed** 24/24 |
| Dig | claiming but fails replay validate (coord-side) |

```bash
ssh -i ~/.ssh/hackme_vast_ed25519 -o IdentitiesOnly=yes -p 5270 root@74.126.26.46
# dash: add -L 8080:localhost:8080
```
