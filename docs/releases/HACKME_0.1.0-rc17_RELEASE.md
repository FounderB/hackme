# HackMe 0.1.0-rc17

**Channel:** live · **Date:** 2026-09-15  
**Hub:** already cut over · this tag publishes **downloadable** Win / Linux / deb / fuzz / HackMe OS ISO

## Highlights

- **Paper Exchange** — desk at [exchange.hackme.tech](https://exchange.hackme.tech/) · hub `#exchange` iframe
- **SUP wallet** — public `/api/sup/activity` + signed send path (nginx allowlist)
- **Hunt** — ASAN/UBSan CLI Lite / Standard / Heavy · 50/50 escrow · pool harness
- **Dig** — customer-first Scan / Audit / Deep · 20/80
- **Hunt Watch 2026sep** — 12/12 public ledger (~192.5M Hunt Standard) · [research](https://hackme.tech/reports/hunt-watch-2026sep/)

## Artifacts

| File | Role |
|------|------|
| `HackMe-Setup-0.1.0-rc17.exe` | Windows installer |
| `hackme_0.1.0-rc17_windows.zip` | Windows portable |
| `hackme_0.1.0-rc17_linux.tar.gz` | Linux miner/node |
| `hackme-node_0.1.0-rc17_amd64.deb` | Ubuntu/Debian apt package |
| `hackme-fuzzing-0.1.0-rc17-*` | Dig / Hunt CLI |
| `HackMe-OS-0.1.0-rc17-amd64.iso` | Live USB HackMe OS |
| `SHA256SUMS.txt` / `SHA256SUMS-iso.txt` | Verify before install |
| `latest.json` | Self-update channel |


## SHA256 (canonical)

| File | Hash |
|---|---|
| `hackme_0.1.0-rc17_windows.zip` | `8d4159eaafaf7293043e5f6c50fecdcc45781ea95d715703436ab9e24c7d2a4e` |
| `hackme_0.1.0-rc17_windows_setup.zip` | `21a96015c5b32808dfdd25a8cffa949c29014c8ade487c66d361dab49065d200` |
| `hackme_0.1.0-rc17_linux.tar.gz` | `ddf872d09324e638d8f93a3a0cc0f2c490faa2e0a7c6d231887fe59b016fd3f3` |
| `HackMe-Setup-0.1.0-rc17.exe` | `5bd98ae1cb18bf2072895e9cd6829b68bb350d4a7c18a7b7e9e01a10d99cc8f2` |
| `Install-HackMe.ps1` | `99cab528dbbc1ac8e30913f60893af5c55bc999f9ad1007fa611fd0e199498a7` |
| `HackMe-Install.cmd` | `0ac66574d6c2eb2bc605f3d254d912f08bb7f46befb8b66db9edc61618c8d243` |
| `hackme-fuzzing-0.1.0-rc17-linux-amd64` | `82d0c4e4ef48b4e0d2556b0d0e51bb002e62c6d87f1990c7ca0778af7eae4715` |
| `hackme-fuzzing-0.1.0-rc17-windows-amd64.exe` | `715863e38fee4c21d2c1d70f3c9e0e6620c3ca802b37d48b7375b9c475286076` |
| `hackme-fuzzing-build-0.1.0-rc17-linux-amd64` | `ce40465f561cf250c29f1e71cafe0e68785cf6940c4a815f90085f884cb1f73f` |
| `hackme-fuzzing-build-0.1.0-rc17-windows-amd64.exe` | `e0f872b44f3e4e840c34ebae7b10d35d242af102ffb8902cd3351c28674b3de0` |
| `hackme-node_0.1.0-rc17_amd64.deb` | `05aa8574277df2cf8ca8a2f883ae9e437dfb9209ad2000aef9a7fc1d14f34f67` |
| `HackMe-OS-0.1.0-rc17-amd64.iso` | `d5b19bf5caea71dfa772d836bee0b4165facad2510243714b69b31d052aa59e8` |

Also attached: `SHA256SUMS.txt`, `SHA256SUMS-iso.txt`, `latest.json`, `RELEASE_MANIFEST.json`.


## Verify

```bash
sha256sum -c SHA256SUMS.txt
sha256sum -c SHA256SUMS-iso.txt   # ISO
```

Downloads: https://hackme.tech/downloads.html  
Cutover notes: https://github.com/jokeez/hackme/blob/main/docs/RC17_CUTOVER.md  
Prior artifacts: [0.1.0-rc16](https://github.com/jokeez/hackme/releases/tag/0.1.0-rc16)

## Honest scope

Paper exchange only — no public matching API / custody. Not financial advice. Hunt CLEAN ledgers are not CVE claims.
