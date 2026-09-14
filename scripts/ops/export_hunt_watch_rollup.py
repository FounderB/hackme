#!/usr/bin/env python3
"""Export Hunt 12-day watch series rollup (HTML + Markdown + JSON).

  SERIES=2026sep python3 scripts/ops/export_hunt_watch_rollup.py
  SERIES=2026sep OUT=reports/hunt-watch/2026sep/ROLLUP.html \\
    python3 scripts/ops/export_hunt_watch_rollup.py

Scans reports/hunt-watch/<series>/day*/hunt-*.json (skips hunt-report-*).
Includes a fixed known-issue appendix for the obscure pilot disclosures.
"""
from __future__ import annotations

import json
import os
import re
from datetime import datetime, timezone
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SERIES = os.environ.get("SERIES", "2026sep")
BASE = Path(os.environ.get("WATCH_BASE", ROOT / "reports" / "hunt-watch" / SERIES))
# Do not inherit soak OUT=.../dayNN-... (a directory); only accept an explicit .html path.
_out = os.environ.get("ROLLUP_HTML") or os.environ.get("OUT")
if _out and str(_out).endswith(".html") and not Path(_out).is_dir():
    OUT_HTML = Path(_out)
else:
    OUT_HTML = BASE / "ROLLUP.html"
OUT_MD = Path(os.environ.get("OUT_MD", BASE / "ROLLUP.md"))
OUT_JSON = Path(os.environ.get("OUT_JSON", BASE / "ROLLUP.json"))
# Public site ledger (soft-publish). Override with SITE_OUT= to skip or redirect.
_site = os.environ.get("SITE_OUT", str(ROOT / "web" / "site" / "reports" / f"hunt-watch-{SERIES}"))
SITE_OUT = Path(_site) if _site and _site.lower() not in ("0", "false", "no", "-") else None

# Honest disclosure appendix (obscure pilot — not part of day01–12 rotation).
KNOWN_ISSUES = [
    {
        "target": "centijson",
        "verdict": "INFORMATIONAL",
        "sanitizer": "ubsan/null-deref (memcpy)",
        "status": "fixed_upstream",
        "detail": "Empty-string input \"\" triggered UBSan at value.c:438 on an older shallow tip. "
        "Maintainer pointed to 7d4ab62 (guard memcpy when len==0). Retested on current master: CLEAN.",
        "links": [
            "https://github.com/mity/centijson/issues/16",
            "https://github.com/mity/centijson/commit/7d4ab62",
        ],
    },
    {
        "target": "jsonparser",
        "verdict": "INFORMATIONAL",
        "sanitizer": "ubsan/null-pointer-offset",
        "status": "known_open_duplicate",
        "detail": "3-byte input {\"\" hit json.c:437. Upstream closed our #186 as duplicate of open #166 (2022).",
        "links": [
            "https://github.com/json-parser/json-parser/issues/186",
            "https://github.com/json-parser/json-parser/issues/166",
        ],
    },
]


def day_num(name: str) -> int:
    m = re.match(r"day(\d+)", name)
    return int(m.group(1)) if m else 0


def load_rows() -> list[dict]:
    rows: list[dict] = []
    if not BASE.is_dir():
        return rows
    for day_dir in sorted(BASE.glob("day*"), key=lambda p: (day_num(p.name), p.name)):
        if not day_dir.is_dir():
            continue
        for path in sorted(day_dir.glob("hunt-*.json")):
            if path.name.startswith("hunt-report-"):
                continue
            try:
                d = json.loads(path.read_text())
            except Exception:
                continue
            d["_day_dir"] = day_dir.name
            d["_day"] = day_num(day_dir.name)
            d["_path"] = str(path.relative_to(ROOT)) if path.is_relative_to(ROOT) else str(path)
            rows.append(d)
    return rows


def summarize(rows: list[dict]) -> dict:
    by_verdict: dict[str, int] = {}
    total_iter = 0
    total_crashes = 0
    days = sorted({r["_day"] for r in rows})
    for r in rows:
        v = str(r.get("verdict") or "UNKNOWN")
        by_verdict[v] = by_verdict.get(v, 0) + 1
        total_iter += int(r.get("iterations") or 0)
        total_crashes += int(r.get("crashes") or 0)
    return {
        "series": SERIES,
        "generated_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "days_present": days,
        "targets_completed": len(rows),
        "total_iterations": total_iter,
        "total_crashes": total_crashes,
        "by_verdict": by_verdict,
        "engine": "hunt_standard",
        "branch": "feature/hunt-mvp",
        "known_issues": KNOWN_ISSUES,
        "rows": rows,
    }


def md(doc: dict) -> str:
    lines = [
        f"# Hunt watch rollup — `{doc['series']}`",
        "",
        f"- generated: **{doc['generated_at']}**",
        f"- branch: [`feature/hunt-mvp`](https://github.com/jokeez/hackme/tree/feature/hunt-mvp)",
        f"- engine: **{doc['engine']}** (ASAN+UBSan, not libFuzzer)",
        f"- targets completed: **{doc['targets_completed']}**",
        f"- total iterations: **{doc['total_iterations']:,}**",
        f"- crashes (series): **{doc['total_crashes']}**",
        f"- verdicts: `{json.dumps(doc['by_verdict'])}`",
        "",
        "## Results",
        "",
        "| Day | Target | Verdict | Iterations | exec/s | Crashes |",
        "|-----|--------|---------|------------|--------|---------|",
    ]
    for r in doc["rows"]:
        eps = float(r.get("exec_per_sec") or 0)
        lines.append(
            f"| {r['_day']} | {r.get('target','?')} | {r.get('verdict','?')} | "
            f"{int(r.get('iterations') or 0):,} | {eps:.1f} | {r.get('crashes',0)} |"
        )
    lines += [
        "",
        "## Known issues (obscure pilot — disclosure)",
        "",
        "These are **not** part of the day-rotation CLEAN ledger. Honest status for public posts:",
        "",
    ]
    for ki in doc["known_issues"]:
        links = ", ".join(f"[link]({u})" for u in ki["links"])
        lines += [
            f"### {ki['target']} — {ki['status']}",
            "",
            f"- verdict at find: **{ki['verdict']}** · `{ki['sanitizer']}`",
            f"- {ki['detail']}",
            f"- {links}",
            "",
        ]
    lines += [
        "## Interpretation",
        "",
        "- **CLEAN** on mature parsers is a normal Hunt Standard outcome.",
        "- Hunt value = verified sanitizer audit + report, not exec/s vs libFuzzer.",
        "- Re-run export anytime: `python3 scripts/ops/export_hunt_watch_rollup.py`",
        "",
    ]
    return "\n".join(lines)


def _day_chips(doc: dict) -> str:
    from collections import defaultdict

    by: dict[int, list] = defaultdict(list)
    for r in doc["rows"]:
        by[int(r["_day"])].append(r)
    chips = []
    for day in sorted(by):
        rows = by[day]
        targets = sorted({str(r.get("target") or "?") for r in rows})
        flags = sorted({str(r.get("verdict") or "?") for r in rows if r.get("verdict") != "CLEAN"})
        if "CVE_CANDIDATE" in flags:
            pill, cls = "SIGNAL", "signal"
        elif flags:
            pill, cls = "INFO", "info"
        else:
            pill, cls = "CLEAN", "done"
        tip = ", ".join(targets[:4]) + ("…" if len(targets) > 4 else "")
        chips.append(
            f'<div class="daychip {cls}" title="{tip}">'
            f"<span>Day {day:02d}</span><span class=\"pill {cls}\">{pill}</span></div>"
        )
    return "".join(chips)


def _highlight_cards(doc: dict) -> str:
    """Dedupe non-CLEAN target highlights for the public hero strip."""
    seen: set[str] = set()
    cards = []
    order = ("CVE_CANDIDATE", "INFORMATIONAL")
    for want in order:
        for r in doc["rows"]:
            v = str(r.get("verdict") or "")
            t = str(r.get("target") or "?")
            key = f"{t}:{v}"
            if v != want or key in seen:
                continue
            seen.add(key)
            crashes = int(r.get("crashes") or 0)
            iters = int(r.get("iterations") or 0)
            label = "Candidate signal" if v == "CVE_CANDIDATE" else "Informational"
            cards.append(
                f'<article class="hl"><span class="hl-tag">{label}</span>'
                f"<h3><code>{t}</code></h3>"
                f"<p>{v} · {iters:,} iter · {crashes} crash artifacts · "
                f"unique sanitizer class — not a CVE ID.</p></article>"
            )
    if not cards:
        return '<p class="sub">No non-CLEAN signals in the day rotation.</p>'
    return "".join(cards)


def html(doc: dict, *, public: bool = False) -> str:
    rows_html = []
    for r in doc["rows"]:
        v = str(r.get("verdict") or "?")
        cls = "ok" if v == "CLEAN" else ("signal" if v == "CVE_CANDIDATE" else "warn")
        eps = float(r.get("exec_per_sec") or 0)
        rows_html.append(
            f"<tr><td>{r['_day']}</td><td><code>{r.get('target','?')}</code></td>"
            f"<td class=\"{cls}\">{v}</td><td>{int(r.get('iterations') or 0):,}</td>"
            f"<td>{eps:.1f}</td><td>{r.get('crashes',0)}</td></tr>"
        )
    issues_html = []
    for ki in doc["known_issues"]:
        links = " · ".join(
            f'<a href="{u}" rel="noopener noreferrer">{u.rsplit("/",1)[-1]}</a>' for u in ki["links"]
        )
        issues_html.append(
            f"<div class=\"card\"><h3>{ki['target']} <span class=\"tag-inline\">{ki['status']}</span></h3>"
            f"<p><code>{ki['sanitizer']}</code> · find verdict <strong>{ki['verdict']}</strong></p>"
            f"<p>{ki['detail']}</p><p class=\"links\">{links}</p></div>"
        )
    bv = doc.get("by_verdict") or {}
    clean_n = int(bv.get("CLEAN") or 0)
    cve_n = int(bv.get("CVE_CANDIDATE") or 0)
    info_n = int(bv.get("INFORMATIONAL") or 0)
    days_n = len(doc.get("days_present") or [])
    canonical = (
        f'<link rel="canonical" href="https://hackme.tech/reports/hunt-watch-{doc["series"]}/"/>'
        if public
        else ""
    )
    nav = (
        '<p class="nav"><a href="../../research.html">Research</a> · '
        '<a href="../../orders.html">Order Hunt</a> · '
        '<a href="../oss-cve-watch/">nghttp2 libFuzzer lane</a></p>'
        if public
        else ""
    )
    return f"""<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8"/>
<meta name="viewport" content="width=device-width,initial-scale=1"/>
<title>HackMe Hunt Watch · {doc['series']} · 12-day ASAN ledger</title>
<meta name="description" content="Hunt Standard 12-day watch: {doc['targets_completed']} target runs, {doc['total_iterations']:,} iterations, {clean_n} CLEAN · {cve_n} CVE_CANDIDATE · {info_n} INFORMATIONAL. Not libFuzzer."/>
{canonical}
<meta name="robots" content="index,follow"/>
<link rel="preconnect" href="https://fonts.googleapis.com"/>
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin/>
<link href="https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;600;700&family=Syne:wght@600;700;800&display=swap" rel="stylesheet"/>
<style>
:root{{
  --bg:#0a1024;--ink:#e8eef8;--muted:#8b9bb8;--line:rgba(120,150,200,.18);
  --ok:#3dffa8;--warn:#ffb020;--signal:#ff7a59;--accent:#6ea8ff;--card:rgba(12,22,48,.72)
}}
*{{box-sizing:border-box}}
body{{margin:0;font-family:"IBM Plex Mono",ui-monospace,monospace;color:var(--ink);line-height:1.55;
  background:
    radial-gradient(1100px 520px at 12% -8%,rgba(80,130,255,.18),transparent 55%),
    radial-gradient(900px 480px at 92% 8%,rgba(61,255,168,.07),transparent 50%),
    linear-gradient(180deg,#0c1231 0%,var(--bg) 45%,#070b18 100%)}}
.wrap{{max-width:960px;margin:0 auto;padding:2.4rem 1.2rem 4rem}}
.nav{{font-size:.75rem;color:var(--muted);margin:0 0 1.4rem}}
.nav a{{color:var(--accent);text-decoration:none}}
.nav a:hover{{text-decoration:underline}}
.hero{{position:relative;overflow:hidden;padding:2rem 1.5rem 1.75rem;border:1px solid rgba(110,168,255,.28);
  border-radius:22px;background:linear-gradient(155deg,rgba(80,130,255,.12),rgba(8,12,28,.65));
  box-shadow:0 24px 60px rgba(0,0,0,.35);margin-bottom:1.5rem}}
.hero::after{{content:"";position:absolute;inset:auto -20% -40% 40%;height:180px;
  background:radial-gradient(circle,rgba(61,255,168,.12),transparent 65%);pointer-events:none}}
.tag{{font-size:.68rem;letter-spacing:.18em;text-transform:uppercase;color:var(--muted);margin:0 0 .55rem}}
h1{{font-family:Syne,sans-serif;font-size:clamp(1.45rem,4.2vw,2.15rem);margin:0;letter-spacing:-.03em;line-height:1.15}}
.lead{{margin:.75rem 0 0;color:#b7c6de;font-size:.88rem;max-width:46rem}}
.badge{{display:inline-flex;align-items:center;gap:.4rem;margin-top:1.1rem;padding:.42rem 1rem;
  border-radius:999px;border:1.5px solid var(--ok);color:var(--ok);font-weight:700;font-size:.76rem}}
.stats{{display:grid;grid-template-columns:repeat(auto-fit,minmax(132px,1fr));gap:.7rem;margin:1.35rem 0 1.6rem}}
.stat{{border:1px solid var(--line);border-radius:14px;padding:.95rem .85rem;background:var(--card);backdrop-filter:blur(8px)}}
.stat b{{display:block;font-size:.62rem;text-transform:uppercase;letter-spacing:.12em;color:var(--muted);margin-bottom:.35rem}}
.stat .v{{font-size:clamp(.95rem,2.5vw,1.28rem);font-weight:700}}
.stat .v.ok{{color:var(--ok)}} .stat .v.signal{{color:var(--signal)}} .stat .v.warn{{color:var(--warn)}}
.policy{{border:1px solid rgba(255,176,32,.32);border-radius:14px;padding:1rem 1.1rem;font-size:.8rem;
  color:#e0c9a0;background:rgba(255,176,32,.06);margin:0 0 1.6rem}}
h2{{font-family:Syne,sans-serif;font-size:1.12rem;margin:1.8rem 0 .7rem;letter-spacing:-.02em}}
.days{{display:grid;grid-template-columns:repeat(auto-fill,minmax(118px,1fr));gap:.5rem;margin:.4rem 0 1.4rem}}
.daychip{{display:flex;justify-content:space-between;align-items:center;gap:.35rem;padding:.55rem .6rem;
  border:1px solid var(--line);border-radius:10px;background:rgba(255,255,255,.02);font-size:.72rem}}
.pill{{font-size:.58rem;padding:.14rem .4rem;border-radius:999px;border:1px solid rgba(255,255,255,.18);letter-spacing:.04em}}
.pill.done{{border-color:rgba(61,255,168,.45);color:var(--ok)}}
.pill.info{{border-color:rgba(255,176,32,.45);color:var(--warn)}}
.pill.signal{{border-color:rgba(255,122,89,.55);color:var(--signal)}}
.hl-grid{{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:.75rem;margin:0 0 1.5rem}}
.hl{{border:1px solid var(--line);border-radius:14px;padding:1rem 1.05rem;background:var(--card)}}
.hl-tag{{font-size:.62rem;letter-spacing:.1em;text-transform:uppercase;color:var(--accent)}}
.hl h3{{margin:.35rem 0 .4rem;font-family:Syne,sans-serif;font-size:1rem}}
.hl p{{margin:0;font-size:.78rem;color:#a9bbd4}}
.table-wrap{{overflow-x:auto;border:1px solid var(--line);border-radius:14px;background:rgba(0,0,0,.18)}}
table{{width:100%;border-collapse:collapse;font-size:.8rem;margin:0}}
th,td{{border-bottom:1px solid var(--line);padding:.55rem .55rem;text-align:left;white-space:nowrap}}
th{{color:var(--muted);font-weight:600;font-size:.66rem;text-transform:uppercase;letter-spacing:.08em}}
tr:last-child td{{border-bottom:0}}
td.ok{{color:var(--ok)}} td.warn{{color:var(--warn)}} td.signal{{color:var(--signal);font-weight:700}}
.card{{border:1px solid var(--line);border-radius:14px;padding:1.05rem 1.15rem;margin:0 0 .85rem;background:var(--card)}}
.card h3{{margin:0 0 .45rem;font-family:Syne,sans-serif;font-size:1rem}}
.tag-inline{{font-size:.65rem;border:1px solid var(--accent);color:var(--accent);padding:.12rem .42rem;border-radius:999px;margin-left:.35rem;vertical-align:middle}}
.links a,.card a{{color:var(--accent)}}
.sub{{color:var(--muted);font-size:.82rem;margin:.15rem 0 1rem}}
footer{{margin-top:2.4rem;padding-top:1.2rem;border-top:1px solid var(--line);color:var(--muted);font-size:.74rem}}
footer a{{color:var(--accent)}}
code{{font-size:.86em}}
</style>
</head>
<body>
<div class="wrap">
  {nav}
  <section class="hero">
    <p class="tag">Hunt product lane · {doc['series']} · Sep 2–14 2026</p>
    <h1>Hunt Watch · {days_n}/{days_n} closed</h1>
    <p class="lead">
      Multi-target <strong>Hunt Standard</strong> marathon — ASAN + UBSan subprocess depth,
      not in-process libFuzzer. Honest public ledger for the Hunt product
      (separate from the nghttp2 / libheif OSS CVE Watch research lane).
    </p>
    <span class="badge">SERIES COMPLETE</span>
  </section>
  <div class="stats">
    <div class="stat"><b>Target runs</b><div class="v">{doc['targets_completed']}</div></div>
    <div class="stat"><b>Iterations</b><div class="v">{doc['total_iterations']:,}</div></div>
    <div class="stat"><b>CLEAN</b><div class="v ok">{clean_n}</div></div>
    <div class="stat"><b>CVE_CANDIDATE</b><div class="v signal">{cve_n}</div></div>
    <div class="stat"><b>INFORMATIONAL</b><div class="v warn">{info_n}</div></div>
    <div class="stat"><b>Crash artifacts</b><div class="v">{doc['total_crashes']:,}</div></div>
  </div>
  <div class="policy">
    <strong>Honest scope.</strong> Hunt ≠ “faster than libFuzzer.” Value = fleetable sanitizer audit + verified report.
    CLEAN on mature parsers is expected. <code>CVE_CANDIDATE</code> = sanitizer class worth triage — not a published CVE ID.
    Engine: <code>{doc['engine']}</code> · generated {doc['generated_at']}.
  </div>
  <h2>Day strip</h2>
  <div class="days">{_day_chips(doc)}</div>
  <h2 id="signals">Signals worth reading</h2>
  <div class="hl-grid">{_highlight_cards(doc)}</div>
  <h2>Full day rotation</h2>
  <div class="table-wrap">
  <table>
    <thead><tr><th>Day</th><th>Target</th><th>Verdict</th><th>Iter</th><th>exec/s</th><th>Crashes</th></tr></thead>
    <tbody>
      {''.join(rows_html)}
    </tbody>
  </table>
  </div>
  <h2>Known issues (obscure pilot)</h2>
  <p class="sub">Disclosure appendix — separate from the CLEAN day ledger above.</p>
  {''.join(issues_html) if issues_html else '<p class="sub">None listed.</p>'}
  <footer>
    Not a CVE lottery. Hunt = verified sanitizer audit + report.
    Re-export: <code>SERIES={doc['series']} python3 scripts/ops/export_hunt_watch_rollup.py</code>
    · <a href="https://hackme.tech/research.html">Research hub</a>
    · <a href="https://hackme.tech/orders.html">Order Hunt</a>
  </footer>
</div>
</body>
</html>
"""


def main() -> int:
    rows = load_rows()
    doc = summarize(rows)
    # strip private keys for JSON export
    export = {k: v for k, v in doc.items() if k != "rows"}
    export["rows"] = [
        {
            "day": r["_day"],
            "day_dir": r["_day_dir"],
            "target": r.get("target"),
            "verdict": r.get("verdict"),
            "iterations": r.get("iterations"),
            "exec_per_sec": r.get("exec_per_sec"),
            "crashes": r.get("crashes"),
            "elapsed_sec": r.get("elapsed_sec"),
        }
        for r in rows
    ]
    OUT_HTML.parent.mkdir(parents=True, exist_ok=True)
    OUT_HTML.write_text(html(doc, public=False))
    OUT_MD.write_text(md(doc))
    OUT_JSON.write_text(json.dumps(export, indent=2) + "\n")
    print(f"[hunt-rollup] targets={doc['targets_completed']} iter={doc['total_iterations']}")
    print(f"[hunt-rollup] HTML → {OUT_HTML}")
    print(f"[hunt-rollup] MD   → {OUT_MD}")
    print(f"[hunt-rollup] JSON → {OUT_JSON}")
    if SITE_OUT is not None:
        SITE_OUT.mkdir(parents=True, exist_ok=True)
        site_html = SITE_OUT / "index.html"
        site_json = SITE_OUT / "rollup.json"
        site_html.write_text(html(doc, public=True))
        site_json.write_text(json.dumps(export, indent=2) + "\n")
        print(f"[hunt-rollup] SITE → {site_html}")
        print(f"[hunt-rollup] SITE → {site_json}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
