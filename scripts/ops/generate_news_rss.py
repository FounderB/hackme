#!/usr/bin/env python3
"""Generate web/site/news.xml from web/site/assets/news-display.json (same source as news page)."""
from __future__ import annotations

import html
import json
import sys
from datetime import datetime, timezone
from pathlib import Path
from email.utils import format_datetime

ROOT = Path(__file__).resolve().parents[2]
SRC = ROOT / "web/site/assets/news-display.json"
OUT = ROOT / "web/site/news.xml"
SITE = "https://hackme.tech"


def parse_day(s: str) -> datetime:
    s = (s or "").strip()
    for fmt in ("%Y-%m-%d", "%Y-%m-%dT%H:%M:%SZ", "%Y-%m-%dT%H:%M:%S%z"):
        try:
            dt = datetime.strptime(s[:19] if "T" in s and fmt.startswith("%Y-%m-%dT%H:%M:%S%") is False and "Z" in fmt else s, fmt)
            if dt.tzinfo is None:
                dt = dt.replace(tzinfo=timezone.utc)
            return dt
        except ValueError:
            continue
    try:
        return datetime.strptime(s[:10], "%Y-%m-%d").replace(tzinfo=timezone.utc)
    except ValueError:
        return datetime.now(timezone.utc)


def main() -> int:
    data = json.loads(SRC.read_text(encoding="utf-8"))
    items = data.get("items") or data.get("news") or []
    if isinstance(data, list):
        items = data
    # news-display may use { "featured": ..., "items": [...] } or flat list under "entries"
    if not items and isinstance(data.get("entries"), list):
        items = data["entries"]
    now = datetime.now(timezone.utc)
    parts = [
        '<?xml version="1.0" encoding="UTF-8"?>',
        '<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom">',
        "<channel>",
        "<title>HackMe Network News</title>",
        f"<link>{SITE}/news.html</link>",
        "<description>Official HackMe pool, release, and research notices.</description>",
        f"<lastBuildDate>{format_datetime(now)}</lastBuildDate>",
        f'<atom:link href="{SITE}/news.xml" rel="self" type="application/rss+xml"/>',
        f"<language>en-us</language>",
    ]
    for it in items[:40]:
        if not isinstance(it, dict):
            continue
        title = str(it.get("title") or "HackMe update").strip()
        summary = str(it.get("summary") or it.get("impact") or "").strip()
        day = str(it.get("date") or it.get("day") or it.get("published") or "")
        slug = str(it.get("id") or it.get("slug") or day or title)[:80]
        link = str(it.get("href") or it.get("url") or f"{SITE}/news.html#{html.escape(slug, quote=True)}")
        if link.startswith("/"):
            link = SITE + link
        dt = parse_day(day)
        parts.append("<item>")
        parts.append(f"<title>{html.escape(title)}</title>")
        parts.append(f"<link>{html.escape(link, quote=False)}</link>")
        parts.append(f"<guid isPermaLink=\"false\">{html.escape(slug)}</guid>")
        parts.append(f"<pubDate>{format_datetime(dt)}</pubDate>")
        if summary:
            parts.append(f"<description>{html.escape(summary)}</description>")
        parts.append("</item>")
    parts.extend(["</channel>", "</rss>", ""])
    OUT.write_text("\n".join(parts), encoding="utf-8")
    print(f"wrote {OUT} ({len(items)} source items)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
