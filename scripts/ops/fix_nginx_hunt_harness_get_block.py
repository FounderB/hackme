#!/usr/bin/env python3
"""Replace corrupted public Hunt harness GET location with a clean block."""
from pathlib import Path
import re
import shutil
import time

CONF = Path("/etc/nginx/sites-available/hackme-site-domain.conf")
bak = Path(f"/tmp/hackme-site-domain.conf.cleanbak.{int(time.time())}")
shutil.copy2(CONF, bak)
text = CONF.read_text()

# Cut from first harness GET location through just before the POST harness comment.
start = text.find('location ~ "^/pool/coordinator/api/fuzz/pool/hunt/harness/[a-fA-F0-9]{8,128}$"')
if start < 0:
    raise SystemExit("start not found")
# walk back to line start
start = text.rfind("\n", 0, start) + 1
post = text.find("# Hunt harness POST (admin publish from customer nodes)")
if post < 0:
    raise SystemExit("POST comment not found")
post = text.rfind("\n", 0, post) + 1

clean = '''    location ~ "^/pool/coordinator/api/fuzz/pool/hunt/harness/[a-fA-F0-9]{8,128}$" {
        if ($request_method !~ ^(GET|HEAD)$) { return 405; }
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $remote_addr;
        proxy_set_header CF-Connecting-IP "";
        proxy_set_header X-Forwarded-Proto $scheme;
        client_max_body_size 40m;
        proxy_connect_timeout 15s;
        proxy_read_timeout 120s;
        proxy_send_timeout 120s;
        # Strip /pool/coordinator so coordinator sees /api/fuzz/pool/hunt/harness/<hash>
        rewrite ^/pool/coordinator(/api/fuzz/pool/hunt/harness/.*)$ $1 break;
        proxy_pass http://127.0.0.1:18081;
    }

'''

CONF.write_text(text[:start] + clean + text[post:])
print("rewrote", CONF, "backup", bak)
