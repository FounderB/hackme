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

_From `dist/release_0.1.0-rc17.2/SHA256SUMS*.txt` after full cut:_

| File | Hash |
|---|---|
| `hackme_0.1.0-rc17.2_windows.zip` | `42f83744692373c8d1add908ce33be8a58ac9a67ffcedb38fc32935a421de508` |
| `hackme_0.1.0-rc17.2_windows_setup.zip` | `dc9507fd3743d2e957e6efce3beee21e62ea4cded0b3158ce53b296a50805f03` |
| `hackme_0.1.0-rc17.2_linux.tar.gz` | `4b90beeae653ee36ec1eeeecc502713b5ce435e72ea558ac15d026efdcd6cdbd` |
| `HackMe-Setup-0.1.0-rc17.2.exe` | `d8606fcce3ba8e766395a33ca8100773441413beb084143c250d956860bcbe19` |
| `Install-HackMe.ps1` | `99cab528dbbc1ac8e30913f60893af5c55bc999f9ad1007fa611fd0e199498a7` |
| `HackMe-Install.cmd` | `0ac66574d6c2eb2bc605f3d254d912f08bb7f46befb8b66db9edc61618c8d243` |
| `hackme-fuzzing-0.1.0-rc17.2-linux-amd64` | `f7dc908ece2d3b5ff0518965c95074a78eb5e07965ad547a8cb3b26a944be4aa` |
| `hackme-fuzzing-0.1.0-rc17.2-windows-amd64.exe` | `4064bf578919b1127854c8d9c00c4e188f393881a06c509f9cbc0e980e3c30d9` |
| `hackme-fuzzing-build-0.1.0-rc17.2-linux-amd64` | `3ddb226e4487210f4dc501107273cbd631b4b3adfc9149bda7def5f72ad0c361` |
| `hackme-fuzzing-build-0.1.0-rc17.2-windows-amd64.exe` | `c4bd60023a16d6436fa534b00804a42764d5eb81b23e28613ae9f4123279f2e8` |
| `hackme-node_0.1.0-rc17.2_amd64.deb` | `ff17dd6b8f9e4d24cbc15e44922bea5c21419a6c3408fce6900e4c08a965e49a` |
| `HackMe-OS-0.1.0-rc17.2-amd64.iso` | `e8d99286e7fe4b01b27ae0a7adab4f9a231ba84ed87abf72747aedeeb947dd76` |

## Verify

```bash
cd dist/release_0.1.0-rc17.2
sha256sum -c SHA256SUMS.txt
bash ../../scripts/release/smoke_artifacts.sh .
```

## Notes

- Supersedes published installers from **0.1.0-rc17** / runtime hotpatch **0.1.0-rc17.1**.
- Channel docs: [HACKME_RC17.md](../HACKME_RC17.md)
