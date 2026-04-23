# D1 — MinIO bucket hardening runbook

Fixes the CRITICAL D1 finding from `docs/security/2026-04-23-adversarial-audit.md`: anonymous listing of the `parley` MinIO bucket exposes Postgres dumps under `backups/*`, readable without authentication.

**Scope of this runbook:** remediation against the already-running lab MinIO at `http://10.10.10.21:9000`. For new deployments, `terraform/userdata-minio.sh` now provisions the correct two-bucket layout and scoped policy; this runbook is only for the existing prod.

**Who runs this:** infra lead with `mc` already aliased to the MinIO admin credentials (see `/etc/default/minio` or `/etc/minio/minio.env` on CT 103 for `MINIO_ROOT_USER` / `MINIO_ROOT_PASSWORD`).

Assume every command below is run on CT 103 with:
```
mc alias set local http://localhost:9000 "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD"
```

---

## Pre-checks (before any change)

Confirm the finding is still live — a leak can be assumed closed only after the `403` result in Step 4.

```
# Should currently succeed and list backup dumps.
curl -s 'http://10.10.10.21:9000/parley/?list-type=2&max-keys=5' | head -40

# Should currently download a dump without auth.
curl -sI 'http://10.10.10.21:9000/parley/backups/parley-YYYYMMDD-HHMMSS.dump' | head -5
```

Record the filenames you see — you'll move them in Step 3.

---

## Step 1 — Apply the scoped policy to `parley`

Writes a bucket policy that allows anonymous `s3:GetObject` only under the four CDN prefixes and denies everything else (including `ListBucket`).

```
cat > /tmp/parley-public-policy.json <<'EOF'
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {"AWS": ["*"]},
      "Action": ["s3:GetObject"],
      "Resource": [
        "arn:aws:s3:::parley/avatars/*",
        "arn:aws:s3:::parley/uploads/*",
        "arn:aws:s3:::parley/soundboard/*",
        "arn:aws:s3:::parley/audio/*"
      ]
    }
  ]
}
EOF
mc anonymous set-json /tmp/parley-public-policy.json local/parley
rm -f /tmp/parley-public-policy.json
```

**Verify:**
```
mc anonymous get-json local/parley
# Prints the JSON above.
```

**Rollback (only if legitimate avatar/asset fetches fail):** `mc anonymous set download local/parley` restores the pre-fix (overly permissive) state. Do not leave this in place — see Step 4 for the correct test.

---

## Step 2 — Create the private backups bucket

```
mc mb --ignore-existing local/parley-backups
mc anonymous set none local/parley-backups
```

**Verify:**
```
mc ls local/ | grep parley-backups
mc anonymous get local/parley-backups
# Access permission for `local/parley-backups` is `none`
```

---

## Step 3 — Move existing backup dumps off `parley`

```
# Preview what will move.
mc ls local/parley/backups/

# Move every dump. `mc mv` is copy-then-delete so each file shows up in
# parley-backups before disappearing from parley.
mc mv --recursive local/parley/backups/ local/parley-backups/
```

**Verify:**
```
mc ls --recursive local/parley-backups/   # expect all dumps present
mc ls local/parley/backups/ 2>&1          # expect empty or "Object does not exist"
```

**Rollback:** `mc mv --recursive local/parley-backups/ local/parley/backups/` puts them back (under a bucket that now correctly refuses anonymous GET for non-CDN prefixes, so the door is already shut even if rollback is partial).

---

## Step 4 — Verify anonymous listing of `parley` is denied

From the **dmz-proxy / any unauthenticated position**, not CT 103 (on CT 103 you're effectively the admin — this must be tested from outside):

```
curl -sSI 'http://10.10.10.21:9000/parley/?list-type=2' | head -5
# expect: HTTP/1.1 403 Forbidden  (Access Denied / <Code>AccessDenied</Code>)

curl -s 'http://10.10.10.21:9000/parley/?list-type=2' | head -20
# expect: <?xml ...><Error><Code>AccessDenied</Code>...
```

Positive-path test — anonymous GET on a real CDN object must still work:

```
# Pick a real key: mc ls local/parley/avatars/ | head -1
curl -sSI "http://10.10.10.21:9000/parley/avatars/<someid>.jpg" | head -3
# expect: HTTP/1.1 200 OK
```

Negative-path test — anonymous GET on a moved backup must now fail:

```
curl -sSI "http://10.10.10.21:9000/parley/backups/parley-YYYYMMDD-HHMMSS.dump" | head -3
# expect: HTTP/1.1 404 Not Found  (object moved in Step 3)

curl -sSI "http://10.10.10.21:9000/parley-backups/parley-YYYYMMDD-HHMMSS.dump" | head -3
# expect: HTTP/1.1 403 Forbidden  (bucket is private)
```

If any of the expected/unexpected behaviours don't match, **do not mark the finding closed**. Re-check Step 1 policy and Step 2 permission.

---

## Step 5 — Point the backup sync at the new bucket

The live backup sync is **not** in this repo — it's an ad-hoc cron/script somewhere on CT 101 (DB container) or a GitHub Actions run. Hunt for it and retarget it. Search candidates on CT 101:

```
grep -Rn 'parley/backups\|s3/parley/backups\|mc cp.*parley' /etc/cron.d /etc/cron.daily /etc/cron.hourly /root /home 2>/dev/null
crontab -l -u root 2>/dev/null | grep -i 'mc\|aws s3\|parley'
systemctl list-timers | grep -i backup
```

Wherever the `mc cp` / `aws s3 cp` target is `s3/parley/backups/...` or `s3://parley/backups/...`, change the destination to `s3/parley-backups/...` (or `s3://parley-backups/...`). The script in the repo (`terraform/userdata-db.sh:216-228`) now documents this requirement for future deployments.

After updating, trigger one manual run and confirm the dump lands in `parley-backups`, not `parley`:
```
mc ls --recursive local/parley-backups/ | tail -5
```

---

## Step 6 — Rotate everything that was in the leaked dumps

The dumps were anonymously downloadable for an unknown window. Assume they exfiltrated. Per audit section D1 Impact:

1. **Argon2 user password hashes** — not crackable at scale, but treat as exposed. Force a password reset for every account:
   - Easiest: invalidate all sessions (rotate `JWT_SECRET` — logs everyone out) AND run a one-shot SQL to null the password column for every user, forcing `/reset-password` on next login. Coordinate a maintenance window.
   - Alternative: send a "suspicious activity" email, force reset on next login via a `password_reset_required` flag column.
2. **Bot API keys** — `internal/auth/middleware.go:121` authenticates bots on stored SHA-256 match. Rotate every bot's key (new key issued via admin, old key revoked). SQL: set `api_key_hash = NULL` on all bots, message operators to re-mint.
3. **`BOT_KEY_SECRET`** — used to encrypt stored LLM provider keys. Because it's deployed to every API container env, exfil of the dump + exfil of `BOT_KEY_SECRET` (separate compromise) would decrypt stored provider keys. Rotate the secret AND re-encrypt every `bot_ai_configs.provider_key_ct` row with the new secret (one-off migration script: decrypt with old, encrypt with new, update row).
4. **`JWT_SECRET`** — rotating this invalidates every outstanding token; users log back in fresh. Low-cost, do it.
5. **`ADMIN_JWT_SECRET`** — same treatment.
6. **Invite codes, channel/DM message content, emails, phone numbers** — cannot be "rotated". Inform affected users per your disclosure policy if required.

Document rotation timestamps in the security log so the audit trail is auditable.

---

## Post-checks (leaving the fix in place)

Run daily for a week, then weekly:

```
# Must stay 403.
curl -sSI 'http://10.10.10.21:9000/parley/?list-type=2' | head -1

# Must return empty.
mc ls local/parley/backups/ 2>&1
```

Set an alert if either ever changes — both are one-liner checks you can wrap in a cron + webhook.

---

## Hand-off notes

- Go-side: `internal/spaces.Client.Upload` still writes public-read (correct for avatar/upload/soundboard/audio). A new `UploadPrivate` method is available for anything that must not be anonymously readable — call it if you ever add a feature that writes non-asset objects via the `spaces` client.
- CORS (`cmd/api/main.go:350`) is unchanged. CORS is a browser-level origin check and is orthogonal to bucket ACLs / anonymous-policy — the audit's recommendation to harden bucket policy does not require CORS changes, and the existing `ConfigureCORS(..., siteURL)` call is still correct.
