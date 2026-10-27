# HackMe 0.1.0-rc17.2

**Channel:** live · **Date:** 2026-09-25 (installers refreshed 2026-09-28)  
**Hub:** already on tip · this tag publishes **downloadable** Win / Linux / deb / fuzz / HackMe OS ISO

## Highlights

- **Hunt deep-havoc v2.8** — opt-in `havoc_deep_v28` stack (walking N-bitflip, interesting64, UTF-8 overlong, length cascade, 3-way splice); new campaigns dig harder; legacy campaigns keep byte-identical replay
- **Shard depth** — lite 48 / standard 192 / heavy 256 iterations; higher power_mut_cap
- **Fuzz pool hardening** — release lease identity, Hunt capability opt-in (dig fleets stay clean), harness publish before claim, `runs_ok` smoke gate
- **workerpoh** — `GPU_CHUNK` / `SEARCH_TIMEOUT_MS` honored for node-spawned workers (#14)
- **Partner retest3** — GPU verbose path NO freeze (124 min / ~4.3k submit-ok under unlimited hotlog)
- **CodeQL / pathsafe** — AbsRE barriers, bounded integer casts
- **2026-09-28 refresh** — Linux tar/deb + Windows installer/zip rebuilt (pool token path, `/opt/hackme/logs` perms, apt `install.sh` drops `pool.miner.token`)

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

_From published `SHA256SUMS.txt` / `SHA256SUMS-iso.txt` on GitHub + `hackme.tech/dist` (2026-09-28 installer refresh):_

| File | Hash |
|---|---|
| `hackme_0.1.0-rc17.2_windows.zip` | `c21bcfe19c6d5d6ecafc5ce6d50de07ffd2a9dc2d846ae2005707380e2ae1d8d` |
| `hackme_0.1.0-rc17.2_windows_setup.zip` | `45656c130e02fd83a65880e80f33e80847729e4a1fcef4f77fdf4c536e69a80e` |
| `hackme_0.1.0-rc17.2_linux.tar.gz` | `7aea1d7acd6831818a496277fcde477ae08b01c129880913c3d83172ecb8f6ac` |
| `HackMe-Setup-0.1.0-rc17.2.exe` | `413537a2862204e62be2ac9f6881e5b03a5c464c0f548582d670f2d605892966` |
| `Install-HackMe.ps1` | `99cab528dbbc1ac8e30913f60893af5c55bc999f9ad1007fa611fd0e199498a7` |
| `HackMe-Install.cmd` | `0ac66574d6c2eb2bc605f3d254d912f08bb7f46befb8b66db9edc61618c8d243` |
| `hackme-fuzzing-0.1.0-rc17.2-linux-amd64` | `f7dc908ece2d3b5ff0518965c95074a78eb5e07965ad547a8cb3b26a944be4aa` |
| `hackme-fuzzing-0.1.0-rc17.2-windows-amd64.exe` | `4064bf578919b1127854c8d9c00c4e188f393881a06c509f9cbc0e980e3c30d9` |
| `hackme-fuzzing-build-0.1.0-rc17.2-linux-amd64` | `3ddb226e4487210f4dc501107273cbd631b4b3adfc9149bda7def5f72ad0c361` |
| `hackme-fuzzing-build-0.1.0-rc17.2-windows-amd64.exe` | `c4bd60023a16d6436fa534b00804a42764d5eb81b23e28613ae9f4123279f2e8` |
| `hackme-node_0.1.0-rc17.2_amd64.deb` | `c61f18b2103532228a2750c1e3d3a0022f86ea616722db42e4c6507f88d7cc76` |
| `HackMe-OS-0.1.0-rc17.2-amd64.iso` | `e8d99286e7fe4b01b27ae0a7adab4f9a231ba84ed87abf72747aedeeb947dd76` |

## Verify

```bash
cd dist/release_0.1.0-rc17.2
sha256sum -c SHA256SUMS.txt
bash ../../scripts/release/smoke_artifacts.sh .
```

Official mirrors:

- `https://github.com/jokeez/hackme/releases/tag/0.1.0-rc17.2`
- `https://hackme.tech/dist/release_0.1.0-rc17.2/SHA256SUMS.txt`
- `https://hackme.tech/dist/latest.json`

## Notes

- Windows installer license page is **AGPL-3.0** (not Apache-2.0).
- Windows auto GPU: VirtualBox/Hyper-V displays no longer force OpenCL when OpenCL.dll is missing.

- Supersedes published installers from **0.1.0-rc17** / runtime hotpatch **0.1.0-rc17.1**.
- Apt `.deb` does **not** ship `pool.miner.token` — use `curl -fsSL https://hackme.tech/apt/install.sh | sudo bash` or [downloads `#pool-token`](https://hackme.tech/downloads.html#pool-token).
- Channel docs: [HACKME_RC17.md](../archive/rc/HACKME_RC17.md)
