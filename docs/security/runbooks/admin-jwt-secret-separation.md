# Admin ↔ API JWT secret separation (F-admin-jwt-secret)

**Finding:** the admin container historically held `PARLEY_JWT_SECRET` — the
same key the api uses to sign normal user session JWTs (`JWT_SECRET`). An
admin-container compromise therefore let the attacker mint a valid user JWT
for any `user_id`, with any desired expiry, bypassing the
`X-Admin-Secret` handshake on `/api/auth/impersonate-token`.

**Fix.** Split the two roles across two distinct signing keys:

| Key                          | Holder(s)       | Signs                       | Verifies                    |
| ---------------------------- | --------------- | --------------------------- | --------------------------- |
| `JWT_SECRET`                 | api             | Normal user session JWTs    | Normal user session JWTs    |
| `IMPERSONATION_JWT_SECRET`   | admin, api      | Admin impersonation tokens  | Admin impersonation tokens  |

The api validator (`internal/auth/service.go::ValidateTokenFull`) reads the
`impersonation: true` claim *before* verifying the signature, then picks the
key based on that claim. A token cannot cross the boundary: an attacker
holding only `IMPERSONATION_JWT_SECRET` can mint impersonation tokens (which
`denyImpersonation` middleware blocks from sensitive routes) but cannot forge
a normal user session.

---

## Live rollout (prod, no downtime)

The api must be restarted **before** the admin so the new impersonation key
is already being verified when admin starts signing with it. This avoids a
window where minted tokens would fail validation.

1. **Generate a fresh `IMPERSONATION_JWT_SECRET`:**

   ```bash
   openssl rand -base64 32
   ```

   Store it in the secret manager (GitHub Actions secret
   `IMPERSONATION_JWT_SECRET`, `terraform.tfvars`, or equivalent).

2. **Add the key to the api container and restart:**

   On each api host:

   ```bash
   sudo sed -i '/^IMPERSONATION_JWT_SECRET=/d' /etc/parley/env
   echo "IMPERSONATION_JWT_SECRET=<value-from-step-1>" | sudo tee -a /etc/parley/env
   sudo chmod 600 /etc/parley/env
   sudo systemctl restart parley-api
   sudo systemctl is-active parley-api   # expect: active
   curl -fsS http://127.0.0.1:8080/health # expect: ok
   ```

   At this point api will verify impersonation tokens against the new key, but
   admin is still signing with the old `PARLEY_JWT_SECRET` (== `JWT_SECRET`).
   Verification against the new key will fail until step 3.

   **Acceptable outage on impersonation only:** the /users/:id/impersonate
   button in admin will error until step 3 completes. Normal user sessions
   are unaffected because they flow through the `JWT_SECRET` path.

3. **Switch the admin container to the new key:**

   ```bash
   sudo sed -i '/^PARLEY_JWT_SECRET=/d' /etc/parley/admin-env
   sudo sed -i '/^IMPERSONATION_JWT_SECRET=/d' /etc/parley/admin-env
   echo "IMPERSONATION_JWT_SECRET=<value-from-step-1>" | sudo tee -a /etc/parley/admin-env
   sudo chmod 600 /etc/parley/admin-env
   sudo systemctl restart parley-admin
   sudo systemctl is-active parley-admin # expect: active
   ```

4. **Verify end-to-end:**

   - Admin login: `POST /api/login` on the admin UI returns 200.
   - Impersonation: use the admin panel to impersonate a test user. The
     returned token should be accepted by the api (WS + REST).
   - Normal user sessions: existing user JWTs still work (they were signed
     with `JWT_SECRET`, which was not touched).
   - Admin-container compromise simulation (on a staging/dev host only):
     with `IMPERSONATION_JWT_SECRET` alone, attempt to sign a token **without**
     the `impersonation: true` claim and hit a protected api endpoint. The
     api must reject it with `invalid token`.

---

## Rollback

If step 3 breaks impersonation:

1. Restore the old admin env:

   ```bash
   sudo sed -i '/^IMPERSONATION_JWT_SECRET=/d' /etc/parley/admin-env
   echo "PARLEY_JWT_SECRET=<old-JWT_SECRET-value>" | sudo tee -a /etc/parley/admin-env
   sudo systemctl restart parley-admin
   ```

2. Revert admin binary to the previous release (which reads
   `PARLEY_JWT_SECRET` instead of `IMPERSONATION_JWT_SECRET`).

3. Remove `IMPERSONATION_JWT_SECRET` from api env and restart — the old
   admin will once again sign with `JWT_SECRET`, which api also verifies as a
   normal token. This re-opens F-admin-jwt-secret, so treat rollback as a
   short-term mitigation while the admin binary is being fixed.

---

## Secret rotation

`IMPERSONATION_JWT_SECRET` is symmetric HMAC — rotation requires both the
admin and api to agree on the current value.

Because `impersonationTTL` is 10 minutes, rotation can be done with a brief
coordinated restart instead of a dual-verify grace period:

1. Generate a new value (`openssl rand -base64 32`).
2. Update the api env first, restart api. Existing admin tokens
   (max 10 min old) become invalid. **Impersonation sessions in flight are
   revoked.** Operators with an open admin tab will see their impersonation
   cookie stop working — they should back out and re-impersonate after step 3.
3. Update the admin env, restart admin. Re-impersonating now works.
4. Total impersonation-unavailable window: ~30 seconds if steps 2-3 are
   scripted. Regular user sessions are unaffected.

No schedule for rotation is mandated by this finding; rotate on suspected
compromise or at least annually, matching `JWT_SECRET`'s cadence.

---

## Graceful degradation on api-only deploys

A deploy without an admin panel can leave `IMPERSONATION_JWT_SECRET` unset
on the api. In that mode the validator rejects any token carrying the
`impersonation: true` claim with `"impersonation unavailable"` rather than
silently accepting it. Admin is fail-start (`log.Fatal` on missing env), so
an admin container that lacks the key will refuse to boot — this is
intentional: a running-but-non-functional admin is worse than a clear start
error.

---

## Verification checklist

- [ ] `grep -R 'PARLEY_JWT_SECRET' terraform/ .github/ cmd/` returns no hits.
- [ ] `grep -R 'IMPERSONATION_JWT_SECRET' terraform/` shows it threaded into
      both `userdata-api.sh` and `userdata-admin.sh`.
- [ ] `go test ./internal/auth/... ./cmd/admin/...` passes with
      `JWT_SECRET=... IMPERSONATION_JWT_SECRET=...` set.
- [ ] Admin logs show `IMPERSONATION_JWT_SECRET is required` and exit=1 if
      the env is removed.
- [ ] Forging an impersonation token with `JWT_SECRET` on the api gets
      rejected by the validator.
