#!/usr/bin/env bash
# Fix public Hunt harness GET: strip /pool/coordinator before proxy_pass.
# Run on hub: sudo bash scripts/ops/patch_nginx_hunt_harness_get.sh
set -euo pipefail
CONF="${NGINX_CONF:-/etc/nginx/sites-enabled/hackme-site-domain.conf}"
[[ -f "$CONF" ]] || { echo "missing $CONF" >&2; exit 1; }
cp -a "$CONF" "/tmp/hackme-site-domain.conf.bak.$(date +%s)"
python3 - "$CONF" <<'PY'
import sys
from pathlib import Path
p = Path(sys.argv[1])
text = p.read_text()
needle = 'location ~ "^/pool/coordinator/api/fuzz/pool/hunt/harness/[a-fA-F0-9]{8,128}$"'
idx = text.find(needle)
if idx < 0:
    raise SystemExit("harness GET location not found")
brace = text.find("{", idx)
depth = 0
end = None
for i, ch in enumerate(text[brace:], brace):
    if ch == "{":
        depth += 1
    elif ch == "}":
        depth -= 1
        if depth == 0:
            end = i + 1
            break
if end is None:
    raise SystemExit("unterminated location block")
j = idx
while j > 0 and text[j - 1] in " \t":
    j -= 1
indent = text[j:idx]
new_block = indent + """location ~ "^/pool/coordinator/api/fuzz/pool/hunt/harness/[a-fA-F0-9]{8,128}$" {
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
    }"""
p.write_text(text[:j] + new_block + text[end:])
print("patched", p)
PY
nginx -t
systemctl reload nginx
echo "nginx reloaded"
