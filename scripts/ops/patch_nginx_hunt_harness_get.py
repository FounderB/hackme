#!/usr/bin/env python3
"""Patch hub nginx: rewrite /pool/coordinator harness GET to /api/... path."""
from pathlib import Path
import shutil
import time

CONF = Path("/etc/nginx/sites-enabled/hackme-site-domain.conf")
bak = Path(f"/tmp/hackme-site-domain.conf.bak.{int(time.time())}")
shutil.copy2(CONF, bak)
print("backup", bak)

lines = CONF.read_text().splitlines(True)
out = []
i = 0
changed = False
needle = 'location ~ "^/pool/coordinator/api/fuzz/pool/hunt/harness/[a-fA-F0-9]{8,128}$"'
rewrite = '        rewrite ^/pool/coordinator(/api/fuzz/pool/hunt/harness/.*)$ $1 break;\n'

while i < len(lines):
    line = lines[i]
    out.append(line)
    if needle in line:
        i += 1
        while i < len(lines):
            l = lines[i]
            if "client_max_body_size 1m;" in l:
                out.append(l.replace("1m", "40m"))
                i += 1
                continue
            if "proxy_pass http://127.0.0.1:18081;" in l and not changed:
                if "rewrite ^/pool/coordinator" not in "".join(out[-8:]):
                    out.append(rewrite)
                out.append(l)
                changed = True
                i += 1
                # copy rest of block
                while i < len(lines):
                    out.append(lines[i])
                    if lines[i].strip() == "}":
                        i += 1
                        break
                    i += 1
                break
            out.append(l)
            i += 1
            if l.strip() == "}":
                break
        continue
    i += 1

if not changed:
    raise SystemExit("no change applied (already patched?)")
CONF.write_text("".join(out))
print("patched", CONF)
