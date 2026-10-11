# Coverage Hunt launch — social paste drafts (2026-10-11)

Strong product story. Facts only: engines live, auto-install, prove/pay, Dig hybrid.  
**Do not** claim a new public ASAN CVE in these posts until scorecard shows ≥1 on a public pack.

Links: https://hackme.tech · https://hackme.tech/downloads.html · https://t.me/hackme_tech · Bitcointalk https://bitcointalk.org/index.php?topic=5583373.0

---

## Twitter / X (thread)

**1/**
HackMe Hunt just crossed a line most “fuzz pools” never do.

We didn’t keep shipping a toy mutator and calling it deep.

Coverage-guided Hunt is LIVE: **libFuzzer on the fleet · AFL++ on Deep** — paid on **coordinator-proven edges**, not vibes.

**2/**
Old chart people quote (~100 vs ~700 exec/s) was **byte-Hunt vs local libFuzzer**.

That race is over.

Product Hunt now runs the **same engine generation** the research world trusts — wrapped as **pool leases + ASAN/showmap prove + escrow**.

**3/**
Fresh 90s bench (cjson): standalone LF prints huge exec/s (as it should).

Hunt libFuzzer lease grew corpus **+85** in one window.

Different KPI. Same seriousness.

Report: docs/HUNT_VS_LIBFUZZER.md

**4/**
Miners: download / update Linux binaries → **clang + AFL++ + bubblewrap auto-install**.

Dig keeps mining. Hunt takes coverage leases. Soft-skip if engines missing — never break PoH.

**5/**
What we sell hard:

• Distributed coverage-guided Hunt  
• Prove-or-floor pay  
• Hybrid Dig + Hunt workers  
• Customer deliverable + escrow  

Not: “we replaced OSS-Fuzz.”

We built the **network around the engines**.

hackme.tech/downloads.html

---

## Telegram (channel)

**HackMe — Coverage Hunt is live**

Мы подняли Hunt на уровень, на котором его и надо продавать.

**libFuzzer** — флот по умолчанию. **AFL++** — Deep.  
Оплата за **доказанные edges / ASAN** на координаторе — не за скриншот от воркера.

Майнеры на Linux: при установке/апдейте бинарников сами ставятся clang + AFL++ + bubblewrap. Dig не ломается.

Старый мем «Hunt 100 exec/s vs libFuzzer 700» — про **byte-путь**. Его больше нет в дефолте продукта.

Свежий замер 90s: standalone LF всё ещё король raw exec/s; Hunt LF lease реально растит corpus (+85 на cjson). Другая метрика — та же серьёзность движка.

Не OSS-Fuzz. **Распределённый coverage Hunt + prove + escrow + hybrid Dig.**

https://hackme.tech/downloads.html  
https://hackme.tech

---

## Discord (#announcements)

**Coverage-guided Hunt is LIVE on HackMe**

Ship note for miners & customers:

- Fleet Hunt engine = **libFuzzer**
- Deep = **AFL++** when present
- Pay = **proven edges / ASAN** (showmap on coordinator) — floor otherwise
- Linux miner install/update **auto-installs** clang + AFL++ + bubblewrap
- Dig stays WASM; hybrid workers keep earning while Hunt leases run

Bench (90s, equal wall): standalone libFuzzer still wins raw exec/s; Hunt LF lease delivered **+85 corpus** on cjson. We stopped competing on the wrong chart.

Docs: `HUNT_VS_LIBFUZZER.md` · Engines: `HUNT_COVERAGE_ENGINES.md`  
Downloads: https://hackme.tech/downloads.html

---

## Bitcointalk (ANN reply / update post)

**Subject:** HackMe Hunt update — coverage-guided libFuzzer + AFL++ on the public pool

---

Update for anyone still judging Hunt by the old byte-mutator exec/s charts.

**What changed**

HackMe Hunt on the public pool is now **coverage-guided**:

- Default fleet engine: **libFuzzer**
- Deep / long profile: **AFL++** when `afl-fuzz` is present
- Workers claim an **engine lease wall**, upload session-new corpus, and the coordinator **proves** with ASAN oneshots + **afl-showmap**
- Pay: **proven new edges and/or ASAN crash** → full PayRun; CLEAN without map delta → floor
- Linux miner install / self-update best-effort installs **clang + AFL++ + bubblewrap** automatically (opt-out available)
- **Dig** (WASM PoH) is unchanged and stays separate — hybrid miners can Dig + Hunt

**Speed / honesty**

The often-cited “~113 vs ~700 exec/s” comparison was **legacy byte-Hunt vs standalone libFuzzer**. That is **not** the product default anymore.

We re-ran an equal 90s wall bench (2026-10-11):

- standalone libFuzzer on cjson: **~15.9k exec/s** (raw local king — expected)
- Hunt **libFuzzer engine** lease on cjson: corpus **335 → 420 (+85)** in ~93s
- legacy byte-Hunt on the same wall: ~29 exec/s (archive lane only)

We are **not** claiming “faster than libFuzzer” or “OSS-Fuzz killer.”  
We are shipping **the real engines on a miner network** with prove, escrow, and hybrid Dig.

**Links**

- Site: https://hackme.tech  
- Downloads: https://hackme.tech/downloads.html  
- Pool: https://hackme.tech/pool/coordinator/api/pool/stats  
- Telegram: https://t.me/hackme_tech  
- ANN thread: https://bitcointalk.org/index.php?topic=5583373.0  

Not financial advice. Verify SHA256SUMS before install.

---

## One-liner variants

- **X / TG short:** “HackMe Hunt is coverage-guided now — libFuzzer + AFL++ on the miner network, paid on proven edges. Dig still mines. https://hackme.tech”
- **Mining boards:** “Not a toy mutator pool — fleet Hunt runs libFuzzer/AFL++ with coordinator prove. Auto-install on Linux update.”
