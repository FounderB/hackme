# Windows AMD maximal E2E — Vast readiness verdict (0.1.0-rc18)

**Host:** `192.168.1.118` (DESKTOP-1RGP4GE) · User: Cyrillic `Ярослав` · **GPU:** AMD Radeon RX 580 (OpenCL / Ellesmere)  
**When:** 2026-10-27 ~00:12–00:45 UTC+5  
**Artifacts tested:** local `dist/release_0.1.0-rc18/` from fresh `make_release_bundle.sh` (commit **`109793a`**, BUILD_INFO `commit=109793ad29c6`) — **not** the old pre-checker GitHub drop from `4bafcc3`.  
**Bundle:** `/tmp/hackme-rc18-bundle.log` shows `[release] done` + SHA256 verify PASS (parent `BUNDLE_EXIT=` echo may be missing; artifacts + verify are authoritative).

Tokens/seeds redacted below. Evidence JSON/log: `scripts/reports/vast-session/e2e-results*.json`, `e2e-log.txt`.

---

## Vast readiness: **GO**

| Gate | Result | Evidence |
|------|--------|----------|
| Linux tar ships `hackme-wasm-check` (CUDA Dig / RLIMIT_AS) | **PASS** | `tar -tzf hackme_0.1.0-rc18_linux.tar.gz` → `linux/hackme-wasm-check`, `linux/bin/hackme-wasm-check` (+ `workerpoh-cuda`, `workerfuzz`) |
| Windows Setup ships `hackme-wasm-check.exe` + `workerfuzz.exe` | **PASS** | Path A install contents complete |
| Wasm validate child marker / sandbox behavior | **PASS** | `hackme-wasm-validate-child` + `sandbox: compile wasm: invalid magic number` on empty stdin (flag defined; sibling check used) |
| OpenCL mine >0.5 GH/s on RX580 | **PASS** | Path A **1.33 GH/s** @ ~8s; final leave-running **~1.28–1.31 GH/s** |
| GitHub `0.1.0-rc18` SUMS vs this rebuild | **MATCH** | Setup / windows_setup.zip / linux.tar.gz SHA256 identical to local |
| apt Packages vs deb | **UNVERIFIED** | `apt.hackme.tech` …/Packages unreachable from this host; local deb sha `d9275c5e…c3c16` recorded only |

**GO reasons:** CUDA fleet path has no-cgo wasm-check in the linux tar; Windows maximal E2E (Setup) clears the AMD quality bar including previously missing `workerfuzz` + new `hackme-wasm-check`; published GH asset hashes already match this rebuild.

---

## PASS/FAIL table + timings

### Clean

| Step | Result | Timing / notes |
|------|--------|----------------|
| Stop procs + uninstall PF/AppData + leftovers | **PASS** | **3.2 s** (Inno unins ~2.2 s); AppData+PF clear; procs=0 |

### Path A — Setup.exe (newest local build)

| Step | Result | Timing / notes |
|------|--------|----------------|
| SHA256 vs SUMS | **PASS** | **0.13 s** · `ae4b1488…cd4b2` · 14 253 384 B |
| Silent `/VERYSILENT /NORESTART` | **PASS** | **4.0 s** · per-user `%LOCALAPPDATA%\HackMe` (no elevation) |
| Install contents (hackme, workerpoh*, **workerfuzz**, **wasm-check**, token, env, bats) | **PASS** | all required present |
| Markers opencl `-hackme-wasm-validate` / wasm-check `hackme-wasm-validate-child` | **PASS** | findstr |
| Wasm-check under stress (empty stdin) | **PASS** | **0.8 s** · child marker + sandbox invalid magic (OK for empty pipe) |
| Start miner / TTFB :8080 | **PASS** | dashboard up (**ttfb sample ~0.05 s** after start; wall ~3 s incl. start) |
| Pool-direct `http://132.243.112.100:18083` | **PASS** | forced in `hackme.env` for LAN reliability |
| Unique WORKER_ID (not bare `worker-desktop-1rgp4ge`) | **PASS** | `worker-desktop-1rgp4ge-a51acd` (installer suffix) |
| OpenCL hashrate >0.5 GH/s ≤60s | **PASS** | **7.8 s** · **ghs=1.331** |
| `/api/status` version/commit | **PASS** | `commit=109793ad29c6` · `build_date=2026-10-26T19:04:55Z` |
| Orders bat + pool campaigns/list + pool/stats 200; hybrid ON | **PASS** | **1.0 s** · hybrid env `HACKME_WORKER_HYBRID_FUZZ=1` `MODE=inline`; log hits |
| Dig/hybrid smoke (claim) | **PASS** | unauth POST claim **403** (empty-pool **429** also seen later: `ghs_priority_defer`); hybrid logs ON |
| Restart miner; hashrate returns | **PASS** | **9.0 s** · ghs=1.331 again |
| Watchdog/log presence | **PASS** | watchdog files=2; logs present |
| Defender quarantine | **PASS** | quarantine_hits=0; 10 exes present |

### Path B — windows_setup.zip from scratch

| Step | Result | Timing / notes |
|------|--------|----------------|
| Uninstall Setup path | **PASS** | **3.2 s** |
| SHA256 zip | **PASS** | **0.17 s** · `c1181263…5c6a` |
| Extract | **PASS** | **1.1 s** |
| `HACKME_SETUP_NONINTERACTIVE` + setup | **PASS*** | copy-to-AppData + setup bat; *`write_hackme_env.ps1 -NonInteractive` needs `-InstallDir`* |
| Contents parity (wasm-check, workerfuzz, …) | **FAIL*** | *missing `hackme.env` at assert time* (bins OK: wasm-check + workerfuzz present) |
| Markers + wasm-check | **PASS** | same as A |
| WORKER_ID | **PASS*** | assert notes `no env` before Patch-PoolDirect wrote coord URL; zip automation gap |
| Mine + TTFB + hashrate | **PASS** | ttfb ~0.5 s; **ghs≈0.63** within 9 s (calibrating; above 0.5) |
| Orders APIs | **PASS** | campaigns/list + fuzz/pool/stats **200** |
| Zip vs Setup key-file parity | **PASS** | `workerfuzz`, `hackme-wasm-check`, `workerpoh-opencl`, `start_hackme_orders.bat` on **both** |

### Extra / final

| Step | Result | Timing / notes |
|------|--------|----------------|
| Final Setup reinstall leave running | **PASS** | WID `worker-desktop-1rgp4ge-c4b670`; initial sample 0.62 GH/s then climbed |
| Live leave-running check (post-E2E) | **PASS** | `hackme.exe` + `workerpoh-opencl.exe`; dash `/api/status` **200**; log **ghs≈1.28–1.31** |

**Wall clock automated E2E:** ~70 s scripted path (hashrate waits short because GPU warm); overall operator window including bundle wait + SCP ~35+ min.

---

## Linux / publish side checks (from this host)

| Check | Result |
|-------|--------|
| Local linux tar contains `hackme-wasm-check` | **YES** |
| Local windows dir / SMOKE list contains `hackme-wasm-check.exe` + `workerfuzz.exe` | **YES** |
| GitHub release `0.1.0-rc18` SHA256SUMS vs local | **IDENTICAL** for Setup, windows_setup.zip, linux.tar.gz |
| apt `Packages` hash vs local deb | **Could not fetch** apt Packages (connection fail); local `hackme-node_0.1.0-rc18_amd64.deb` sha256 `d9275c5e2f3f55bfad852dbe95ac8412b91fa2954a11051aefa5e99f3c0c3c16` |

---

## Blockers remaining (non-blocking for Vast GO, fix soon)

1. **Zip/noninteractive env:** `write_hackme_env.ps1` requires `-InstallDir`; Path B assert saw missing `hackme.env` until later patch. Setup path is clean — prefer Setup for fleet Windows; fix zip docs/automation.
2. **apt mirror unreachable** here — confirm published deb/Packages hash before apt-based Vast images if you rely on apt rather than the linux tar.
3. **LAN pool-direct still recommended** on this box (`HACKME_COORDINATOR_URL=http://132.243.112.100:18083`); CF URL remains in env as alternate.
4. **OpenSSH job semantics:** miner children can die when the SSH session that spawned short `start` exits; long-lived start or scheduled task needed for unattended SSH bring-up (not a product defect for interactive/desktop install).
5. **Unauth fuzz claim 403** in smoke (expected without miner token); worker logs show hybrid claim **429** `ghs_priority_defer` when empty/deferred — not a ship blocker.

---

## Machine final state

| Item | Value |
|------|--------|
| Install | `%LOCALAPPDATA%\HackMe` (Setup 0.1.0-rc18, commit **109793a**) |
| Processes | `hackme.exe` + `workerpoh-opencl.exe` |
| WORKER_ID | `worker-desktop-1rgp4ge-c4b670` (unique suffix) |
| Coordinator | pool-direct `http://132.243.112.100:18083` (plus stock CF key still present) |
| Hybrid fuzz | ON / inline |
| Hashrate | **~1.28–1.33 GH/s** OpenCL RX580 |
| Dashboard | `http://127.0.0.1:8080/` · `/api/status` 200 |
| Key bins | `hackme-wasm-check.exe`, `workerfuzz.exe`, `workerpoh-opencl.exe` present; Defender clean |
| Staging dir | `%USERPROFILE%\HackMe-E2E-rc18\` (Setup + zip + e2e logs) |

### Restart

```text
%LOCALAPPDATA%\HackMe\start_hackme_miner.bat
```

---

## Verdict one-liner

**Vast: GO** — fresh rc18 bundle (`109793a`) includes linux+windows `hackme-wasm-check`; Windows AMD maximal E2E Setup path is green at ~1.3 GH/s with workerfuzz parity; GitHub asset hashes already match this build. Remaining: apt verify + zip env InstallDir polish.
