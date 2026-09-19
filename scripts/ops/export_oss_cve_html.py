#!/usr/bin/env python3
"""Export OSS CVE hunt rollup to public site (HOLD banner when CVE_CANDIDATE)."""
import json
import sys
from pathlib import Path

def main() -> int:
    if len(sys.argv) < 2:
        print("usage: export_oss_cve_html.py REPORT_DIR", file=sys.stderr)
        return 2
    report = Path(sys.argv[1]).resolve()
    rollup_path = report / "ROLLUP.json"
    if not rollup_path.is_file():
        print(f"missing {rollup_path}", file=sys.stderr)
        return 2
    root = report
    for _ in range(8):
        if (root / "web" / "site").is_dir():
            break
        root = root.parent
    else:
        print("web/site not found", file=sys.stderr)
        return 2
    out = root / "web" / "site" / "reports" / "oss-cve"
    out.mkdir(parents=True, exist_ok=True)
    r = json.loads(rollup_path.read_text())
    hold = r.get("verdict") == "CVE_CANDIDATE"
    # Public site never gets crash PoC / candidate IDs while under disclosure HOLD.
    public_summary = (
        "Responsible disclosure HOLD — CVE candidate(s) under maintainer triage. "
        "No public crash details."
        if hold
        else (r.get("summary") or "")
    )
    public_verdict = "HOLD" if hold else r.get("verdict")
    rows = ""
    for t in r.get("targets", []):
        tid = t.get("target_id")
        tv = t.get("verdict")
        if hold and tv == "CVE_CANDIDATE":
            tv = "HOLD"
        nc = len(t.get("crashes", []))
        crash_cell = "—" if (nc > 0 or tv == "HOLD") else "0"
        rows += f"<tr><td>{tid}</td><td>{t.get('iterations')}</td><td>{crash_cell}</td><td>{tv}</td></tr>"
    banner = (
        '<p class="hold"><strong>Responsible disclosure HOLD</strong> — '
        "CVE candidate(s) under maintainer triage. Do not weaponize inputs.</p>"
        if hold
        else '<p class="clean">No ASAN crash in budget — methodology case study.</p>'
    )
    html = f"""<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"><title>OSS CVE Hunt</title>
<style>body{{font-family:system-ui;max-width:920px;margin:2rem auto;padding:0 1rem;line-height:1.5}}
.hold{{background:#fff3cd;border:1px solid #ffc107;padding:1rem;border-radius:6px}}
.clean{{color:#0a0}} table{{border-collapse:collapse;width:100%}}td,th{{border:1px solid #ccc;padding:8px}}</style>
</head><body>
<h1>OSS CVE Hunt — real upstream ASAN</h1>
{banner}
<p>{public_summary}</p>
<p><code>verdict={public_verdict}</code> · started {r.get('started_at')}</p>
<h2>Targets</h2>
<table><tr><th>ID</th><th>Iterations</th><th>Signals</th><th>Verdict</th></tr>
{rows}
</table>
<p><a href="https://github.com/jokeez/hackme/blob/main/docs/OSS_CVE_HUNT.md">Methodology</a></p>
</body></html>"""
    run_html = html.replace("</body></html>", "").replace(
        "<h1>OSS CVE Hunt — real upstream ASAN</h1>",
        "<h1>OSS CVE Hunt — latest run</h1><p><a href=\"./index.html\">← Case studies</a></p>",
    ) + "</body></html>"
    meta = {
        "verdict": "CLEAN" if hold else r.get("verdict"),
        "summary": public_summary if hold else r.get("summary"),
        "cve_candidates": [],
        "informational_targets": r.get("informational_targets", []),
        "clean_targets": r.get("clean_targets", []),
        "hold_disclosure": ["centijson", "csonh"],
        "disclosed_closed": ["libucl", "cfgpack", "lua", "quickjs"],
        "informational": ["mxml", "nghttp2", "duktape"],
        "wave": report.name,
        "started_at": r.get("started_at"),
        "finished_at": r.get("finished_at"),
        "publish_allowed": True,
    }
    existing_meta = out / "meta.json"
    if existing_meta.is_file():
        try:
            prev = json.loads(existing_meta.read_text())
            for k in ("hold_disclosure", "disclosed_closed", "informational"):
                if k in prev:
                    meta[k] = prev[k]
            # Always keep known disclosure holds.
            holds = list(dict.fromkeys(list(meta.get("hold_disclosure") or []) + ["centijson", "csonh"]))
            meta["hold_disclosure"] = holds
            if r.get("informational_targets"):
                info = list(dict.fromkeys(prev.get("informational", []) + r.get("informational_targets", [])))
                meta["informational"] = info
        except json.JSONDecodeError:
            pass
    (out / "meta.json").write_text(json.dumps(meta, indent=2) + "\n")
    (out / "latest-run.html").write_text(run_html)
    cases_script = root / "scripts" / "ops" / "export_oss_cve_cases.py"
    if cases_script.is_file():
        import subprocess
        subprocess.run([sys.executable, str(cases_script), str(root)], check=False)
    print(f"exported → {out}")
    return 0

if __name__ == "__main__":
    raise SystemExit(main())
