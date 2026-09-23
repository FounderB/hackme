# Cloudflare vs origin CSP for exchange.hackme.tech

**Origin (VPS nginx):** sets `Content-Security-Policy` with `frame-ancestors 'self' https://hackme.tech …`
and `Cross-Origin-Resource-Policy: cross-origin` (`scripts/ops/nginx/hackme-exchange-domain.tls.conf`).

**Cloudflare (as of 2026-09-23):** public responses show `X-Frame-Options: SAMEORIGIN` and
`Cross-Origin-Resource-Policy: same-origin`, and **omit** origin CSP. That **blocks hub iframe**
from `https://hackme.tech` and undoes the frame-ancestors gate.

## Operator fix (Cloudflare dashboard)

For zone `hackme.tech` → hostname `exchange.hackme.tech`:

1. **Rules → Transform Rules → Modify Response Header** (or Configuration Rules):
   - Remove header: `X-Frame-Options`
   - Set header: `Content-Security-Policy` =
     `default-src 'self'; base-uri 'self'; frame-ancestors 'self' https://hackme.tech http://127.0.0.1:8080 http://localhost:8080; object-src 'none'`
   - Set/override: `Cross-Origin-Resource-Policy` = `cross-origin`
2. Purge cache for `https://exchange.hackme.tech/`
3. Verify:
   ```bash
   curl -sI https://exchange.hackme.tech/ | grep -iE 'content-security|x-frame|cross-origin-resource'
   # expect: CSP with frame-ancestors hackme.tech; no X-Frame-Options SAMEORIGIN
   ssh hackme-vps 'curl -skI https://127.0.0.1/ -H "Host: exchange.hackme.tech" | grep -i content-security'
   ```

Until CF is fixed, **origin is correct**; public edge framing claims remain conditional.
