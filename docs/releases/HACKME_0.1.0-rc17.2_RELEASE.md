# HackMe 0.1.0-rc17.2

**Channel:** live · **Date:** 2026-09-25  
**Hub:** already on tip · this tag publishes **downloadable** Win / Linux / deb / fuzz / HackMe OS ISO

## Highlights

- **Hunt deep-havoc v2.8** — opt-in `havoc_deep_v28` stack (walking N-bitflip, interesting64, UTF-8 overlong, length cascade, 3-way splice); new campaigns dig harder; legacy campaigns keep byte-identical replay
- **Shard depth** — lite 48 / standard 192 / heavy 256 iterations; higher power_mut_cap
- **Fuzz pool hardening** — release lease identity, Hunt capability opt-in (dig fleets stay clean), harness publish before claim, `runs_ok` smoke gate
- **workerpoh** — `GPU_CHUNK` / `SEARCH_TIMEOUT_MS` honored for node-spawned workers (#14)
- **Partner retest3** — GPU verbose path NO freeze (124 min / ~4.3k submit-ok under unlimited hotlog)
- **CodeQL / pathsafe** — AbsRE barriers, bounded integer casts

## Artifacts

| File | Role |
|------|------|
| `HackMe-Setup-0.1.0-rc17.2.exe` | Windows installer |
| `hackme_0.1.0-rc17.2_windows.zip` | Windows portable |
| `hackme_0.1.0-rc17.2_linux.tar.gz` | Linux miner/node |
| `hackme-node_0.1.0-rc17.2_amd64.deb` | Ubuntu/Debian apt package |
| `hackme-fuzzing-0.1.0-rc17.2-*` | Dig / Hunt CLI |
| `HackMe-OS-0.1.0-rc17.2-amd64.iso` | Live USB HackMe OS |
| `SHA256SUMS.txt` / `SHA256SUMS-iso.txt` | Verify before install |
| `latest.json` | Self-update channel |

## SHA256 (canonical)

_Filled after `run_full_release_local.sh` — see `dist/release_0.1.0-rc17.2/SHA256SUMS.txt`._

## Verify

```bash
cd dist/release_0.1.0-rc17.2
sha256sum -c SHA256SUMS.txt
bash ../../scripts/release/smoke_artifacts.sh .
```

## Notes

- Supersedes published installers from **0.1.0-rc17** / runtime hotpatch **0.1.0-rc17.1**.
- Channel docs: [HACKME_RC17.md](../HACKME_RC17.md)
