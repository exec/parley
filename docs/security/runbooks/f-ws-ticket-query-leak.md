# F-ws-ticket-query-leak — suppress WS ticket in DMZ nginx access log

**Audit finding.** `docs/security/2026-04-23-adversarial-audit.md` — F-ws-ticket-query-leak (LOW).

WebSocket clients authenticate the upgrade with a short-lived single-use
ticket passed as a query-string parameter (`/ws?ticket=<opaque>`). The JWT
never touches the URL. The ticket is:

- minted with a 60-second TTL,
- stored in Redis under a single key,
- consumed with `GETDEL` so a second use is impossible.

Those two properties cap the real-world impact of a logged ticket to the
intersection of "attacker reads the log within 60 seconds AND ticket has
not already been consumed by the legitimate client." In practice this
window closes in milliseconds because the upgrade request itself consumes
the ticket. The residual concern is cosmetic: the DMZ nginx default
`access_log` format records the full request line including the query
string, so the ticket sits in `/var/log/nginx/dmz-access.log` for the
retention window and in any log-shipping pipeline that ingests it.

This runbook removes the cosmetic leak.

## Applies to

- CT 106 (DMZ nginx, `/etc/nginx/sites-enabled/dmz.conf`, parley server block).
- No api / admin / frontend code change.

## Fix — recommended

Add a `location = /ws` block to the parley server block with
`access_log off;`. The WS upgrade is a long-lived connection; per-request
access-log entries carry no operationally useful signal that justifies
logging them, and proxy error / connection state is still captured by
`error_log`.

```nginx
# /etc/nginx/sites-enabled/dmz.conf — parley server block
# ... existing directives (listen, server_name, ssl_*, include snippets/...) ...

include snippets/cloudflare-realip.conf;
if ($is_cloudflare = 0) { return 403; }

# New: don't access-log the WS upgrade so the single-use ticket
# query param never lands in dmz-access.log.
location = /ws {
    access_log off;

    proxy_pass http://10.20.0.106:8080;   # existing api upstream
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_read_timeout 3600s;
    proxy_send_timeout 3600s;
}

location / {
    # ... existing proxy_pass / proxy_set_header block unchanged ...
}
```

**Constraints that still apply.** The `snippets/cloudflare-realip.conf`
include and the `$is_cloudflare = 0 → return 403` check must remain in
effect for the `/ws` location. If they are defined in the server-block
scope (above `location /`), they already cover the new block; confirm
during apply. If they live inside `location /`, copy them into `location
= /ws` too — the rule that only Cloudflare-origin traffic reaches the
backend is not negotiable.

## Fix — alternative (if per-request logging must be kept)

Define a log_format that records the path only (`$uri`) rather than the
full request line (`$request`), and use it for the `/ws` location.

```nginx
# /etc/nginx/conf.d/01-dmz-format.conf
log_format dmz_no_query
    '$remote_addr - $remote_user [$time_local] '
    '"$request_method $uri $server_protocol" $status $body_bytes_sent '
    '"$http_referer" "$http_user_agent"';
```

```nginx
# inside the parley server block
location = /ws {
    access_log /var/log/nginx/dmz-access.log dmz_no_query;
    # ... proxy_pass as above ...
}
```

`$uri` is the normalized path without the query string; `$request` and
`$request_uri` both retain it, so neither is safe here. Prefer the
`access_log off` variant unless there is an audit or SIEM requirement
to retain a per-upgrade log line.

## Rollout

1. SSH to CT 106. Back up `/etc/nginx/sites-enabled/dmz.conf`.
2. Apply the diff above to the parley server block.
3. `nginx -t` to validate.
4. `systemctl reload nginx` (no restart needed; reload preserves existing
   upgraded WS sockets).
5. Run the verification step below.

## Verification

From a workstation that can reach the public endpoint (traffic traverses
Cloudflare → DMZ nginx):

```bash
curl -sv "https://<host>/ws?ticket=VERIFY-$(date +%s)-$(openssl rand -hex 16)" \
  -H "Upgrade: websocket" -H "Connection: Upgrade" \
  -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
  -H "Sec-WebSocket-Version: 13" || true
```

Then on CT 106:

```bash
grep 'VERIFY-' /var/log/nginx/dmz-access.log || echo "no log line — access_log off is working"
```

With the recommended fix the grep returns nothing. With the alternative
format, the log line is present but the ticket substring is not.

If you see the ticket in the log, the `location = /ws` block either did
not match (most common cause: a `location /` block with a more specific
prefix match was evaluated first) or the reload did not take effect;
re-check `nginx -T` output to confirm which block owns `/ws`.

## Why this is runbook-only

The nginx config (`/etc/nginx/sites-enabled/dmz.conf`) is not in the
repo — it lives on CT 106 and is edited in place. The application-layer
mitigations (single-use + 60s TTL) already cap the vulnerability; this
runbook closes the residual logging leak so a stolen log file cannot
replay a ticket even in the sub-second window.
