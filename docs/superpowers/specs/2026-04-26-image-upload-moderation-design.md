# Image Upload Moderation Design

**Goal:** Stand up the technical controls behind Parley's policy ban on adult content (TOS §5) and the legally-mandated CSAM reporting obligations (Privacy §3 / 18 U.S.C. § 2258A). Three independent moderation layers, a defensible single-operator review queue, and an end-to-end automated path for CSAM that never asks the operator to look at the image.

**Artifacts produced (this is the deliverable):**
- This design spec — reasoning record + architecture
- Implementation plan to follow at `docs/superpowers/plans/2026-04-26-image-upload-moderation.md`
- README pre-launch checklist already gates v1 on this work being verified end-to-end

**Out of scope for this spec:** the policy-document side (already shipped in `TOS.md` §5 and `PRIVACY.md` §6, with the v5 reasoning at `2026-04-26-tos-and-privacy-design.md`). This spec is the technical-controls counterpart.

---

## Decisions made

### Pipeline scope

The pipeline runs at `/api/upload` itself — every image that lands in Spaces is hash-matched. Hash matching against NCMEC lists is essentially free and legally compulsory once we're using NCMEC hash sources, so exempting any user-uploaded surface from that creates a coverage gap.

The classifier portion (Falconsai, Hive Visual Moderation) gates **non-trivial surfaces** — `message_attachment` and `dm_message_attachment`. The **trusted-purpose surfaces** (`user_avatar`, `user_banner`, `server_icon`, `server_banner`, `dm_group_avatar`, `soundboard`) pass on low-confidence and only quarantine on (a) hash hits, or (b) clear-positive classifier signal (Hive `class_score >= 0.99` or Falconsai `nsfw_score >= 0.98` — tighter than the standard threshold). The reasoning: the false-positive cost of holding up someone's avatar change is high relative to the abuse-vector value, but hash hits are rare and a clear-positive on an avatar is its own red flag.

Soundboard sounds are audio, not images — the classifier path doesn't apply, but soundboard files still get a `moderation_records` row (state transitions straight to `approved` with a `purpose = 'soundboard'` notation) so audit/retention/quarantine plumbing is uniform.

This also means the upload pipeline must know *what surface* the upload is destined for — which drives the propagation decision below.

### Synchrony

Hybrid: **Falconsai sync at `/api/upload`**, **Hive async** after the Spaces write, **Cloudflare CSAM Scanning Tool independent** (out of band on cached content; emails operator on hits).

- Falconsai is fast (~100-300 ms on CPU) and runs locally on our infrastructure. Sync gating means obvious positives are blocked at the door — no URL is ever returned for an obviously-NSFW image.
- Hive's API call is slower (~1-2 s) and per-call cost matters; running it async lets the user see their image immediately, with retroactive quarantine if Hive flags something Falconsai missed. The retroactive-quarantine window is the abuse cost we're accepting in exchange for not paying Hive latency on every clean upload.
- Cloudflare's tool runs on cached content — there's no inline integration to make sync; their email notification is the only signal.

State machine per upload:

```
pending_local
  ├─ Falconsai positive ─→ pending_csam_check_after_reject
  │                          ├─ Hive CSAM positive ─→ quarantined_csam (terminal)
  │                          └─ Hive CSAM negative ─→ quarantined_nsfw (terminal — bytes dropped, no Spaces write)
  └─ Falconsai pass ─→ pending_remote
                        ├─ Hive CSAM positive ─→ quarantined_csam (terminal — Spaces moved to csam-quarantine/)
                        ├─ Hive Visual NSFW positive ─→ quarantined_nsfw (terminal — Spaces deleted, propagation runs)
                        └─ both negative ─→ approved (terminal)
```

`pending_csam_check_after_reject` rows have a `user_uploads` row marked `provisional = true` pointing to bytes in the `pending-csam-check/` Spaces prefix. The bytes are durable from the moment `/api/upload` returns 422; if the API process crashes mid-Hive-call, the worker's recovery loop drains the row on the next poll cycle.

### Falconsai deployment

Python FastAPI sidecar in a **separate VM on the Proxmox internal bridge**. Reasoning:

- **CPU-spike isolation.** Vision Transformer inference spikes a CPU core; keeping it off the API box's CPU protects login latency, message fan-out, and (most importantly) LiveKit voice signaling.
- **Boot independence.** Loading torch + transformers + Falconsai weights takes 5-10 seconds at process start; the API doesn't wait on it.
- **Future GPU option.** If Hive volume gets expensive, attaching a GPU to *just* the classifier VM is straightforward; doing so on a shared API box is messy.
- **Disk hygiene.** torch + transformers + the model = ~2 GB; off the API box's image.

**Sidecar contract:**
- `POST /classify` accepts image bytes + content-type
- Returns `{"nsfw_score": 0.0-1.0, "model": "Falconsai/nsfw_image_detection", "model_version": "..."}`
- Health: `GET /health` (returns 200 when model is loaded)
- Auth: `X-Parley-Auth-Timestamp: <unix_seconds>` header + `X-Parley-Auth: <hmac_sha256(timestamp || body, shared_secret)>` header. Sidecar verifies the HMAC and rejects requests with a timestamp outside a **60-second window** to prevent replay. Probably not exploitable in practice (internal bridge isolation) but a five-line fix that closes the seam.
- Transport: HTTP over Proxmox internal bridge (private 10.x.x.x); no TLS (host-internal traffic only)
- Hostname: `moderation.parley.internal` via Proxmox-managed DNS so the VM can be moved without touching API config

**Sidecar repo layout** (new directory in this monorepo, NOT a separate repo):
- `services/moderation-classifier/`
  - `app.py` — FastAPI app
  - `model.py` — Falconsai loader, classify function
  - `auth.py` — HMAC verification middleware
  - `requirements.txt` — `fastapi`, `uvicorn[standard]`, `torch`, `transformers`, `pillow`, `python-multipart`
  - `systemd/parley-moderation.service` — systemd unit for autorestart
  - `README.md` — local-dev instructions

**Confidence thresholds:**
- `nsfw_score >= 0.95` → quarantine (high confidence positive)
- `nsfw_score <= 0.05` → pass to Hive layer
- `0.05 < nsfw_score < 0.95` → pass to Hive layer (uncertain, get second opinion)

The threshold pair is configurable via env vars on the API side, so we can tune empirically without redeploying the classifier.

### Async queue + state storage

`moderation_records` table + a Postgres-polled worker goroutine inside the API binary. Reasoning: at v1 volume Hive will return well within any sensible poll interval; Postgres-as-queue keeps Redis Streams off the dependency list; durable state means an API restart mid-pipeline is safe.

**New migration (#70):**

In addition to the new table below, the migration adds:
- `user_uploads.provisional BOOLEAN NOT NULL DEFAULT false` — flags rows whose bytes are still pending CSAM verdict in the `pending-csam-check/` Spaces prefix; provisional rows are excluded from quota accounting and from any user-visible listing.
- `messages.moderation_removed BOOLEAN NOT NULL DEFAULT false`
- `dm_messages.moderation_removed BOOLEAN NOT NULL DEFAULT false`
- `soundboard_sounds.removed_by_moderation BOOLEAN NOT NULL DEFAULT false`

```sql
CREATE TABLE moderation_records (
  id BIGSERIAL PRIMARY KEY,
  upload_id BIGINT NOT NULL REFERENCES user_uploads(id) ON DELETE CASCADE,
    -- Every moderation_records row points at a real user_uploads row. For
    -- Falconsai-positive sync rejections, the user_uploads row sits in the
    -- `pending-csam-check/` Spaces prefix (provisional flag set true) until
    -- the Hive CSAM verdict resolves it.
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  purpose TEXT NOT NULL DEFAULT 'unknown',
    -- 'message_attachment' | 'dm_message_attachment'
    -- | 'user_avatar' | 'user_banner'
    -- | 'server_icon' | 'server_banner'
    -- | 'dm_group_avatar' | 'soundboard'
    -- | 'orphan'  — backfill resolved no surface reference (deleted parent, etc.)
    -- | 'unknown' — fallback for client-build-without-purpose-field; warning logged at ingest
  state TEXT NOT NULL,
    -- 'pending_local' | 'pending_remote' | 'pending_csam_check_after_reject'
    -- | 'approved' | 'quarantined_nsfw' | 'quarantined_csam'
  falconsai_verdict JSONB,
  hive_visual_verdict JSONB,
  hive_csam_verdict JSONB,
  cloudflare_csam_hit_at TIMESTAMPTZ,
  operator_decision TEXT,
    -- 'approved_post_review' | 'removed' | 'escalated' | NULL while pending
  operator_decided_at TIMESTAMPTZ,
  operator_decided_by BIGINT REFERENCES admin_users(id),
  ncmec_report_id TEXT,
  ncmec_submitted_at TIMESTAMPTZ,  -- separate from closed_at because submission can be delayed (Hive outage, manual Cloudflare paste step, etc.). The § 2258A(h)(1) clock starts here, not at workflow closure.
  legal_hold BOOLEAN NOT NULL DEFAULT false,  -- set true on LE preservation directive; blocks retention sweep until cleared
  legal_hold_set_at TIMESTAMPTZ,
  retention_until TIMESTAMPTZ,  -- explicit retention deadline populated by workflow per record type; nullable while pending
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  closed_at TIMESTAMPTZ,  -- when state went terminal (informational; not used for retention sweep)

  -- Invariant: terminal-state rows must have retention_until set. Workflow code that
  -- transitions a row to a terminal state without populating retention_until trips
  -- this constraint and surfaces the bug at write time, instead of leaving a row
  -- to sit in the database indefinitely with no retention plan (which is exactly
  -- the kind of "data we shouldn't have" that creates exposure on a future audit).
  CONSTRAINT moderation_records_terminal_has_retention CHECK (
    state IN ('pending_local', 'pending_remote', 'pending_csam_check_after_reject')
    OR retention_until IS NOT NULL
  )
);

CREATE INDEX moderation_records_pending_remote_idx
  ON moderation_records(created_at)
  WHERE state = 'pending_remote';

CREATE INDEX moderation_records_review_queue_idx
  ON moderation_records(created_at)
  WHERE state = 'quarantined_nsfw' AND operator_decision IS NULL;

CREATE INDEX moderation_records_user_id_idx
  ON moderation_records(user_id);

CREATE INDEX moderation_records_retention_idx
  ON moderation_records(retention_until)
  WHERE retention_until IS NOT NULL AND legal_hold = false;
```

**Worker behavior:**
- Single goroutine launched at API boot
- Poll loop: `SELECT id, upload_id FROM moderation_records WHERE state = 'pending_remote' ORDER BY created_at LIMIT 10` every 2 seconds
- For each row: fetch image bytes from Spaces, call Hive Visual Moderation + Hive CSAM Detection in parallel, update row with verdicts, transition state, propagate quarantine if applicable
- Row-level lock to prevent double-processing (`SELECT ... FOR UPDATE SKIP LOCKED`)
- Backoff on Hive API errors: 30 s, 2 min, 10 min; after 3 failures, leave row in `pending_remote` for the next poll cycle (don't drop).

**Stuck-row monitors.** Three Prometheus-style metrics + alerts fire when the workflow state machine stalls. Each one corresponds to a real failure mode that would otherwise leave rows sitting in the database invisibly:

- `moderation_records_pending_remote_age_seconds` (max). Fires above **1 hour** — Hive is down or backlogged. Tells you the Hive-outage operational acknowledgment below is currently in effect.
- `moderation_records_stuck_csam_submission_count`. Counts rows in `quarantined_csam` with `ncmec_submitted_at IS NULL` older than **6 hours**. Fires above 0. The CSAM workflow is supposed to file the NCMEC report and populate `ncmec_submitted_at`; if that step failed, the row exists in `quarantined_csam` but has no submission record and (because retention_until depends on submission timestamp) no retention deadline. § 2258A(b) requires "as soon as reasonably possible"; 6 hours is well inside that bound but well outside any normal latency.
- `moderation_records_terminal_no_retention_count`. Counts rows in any terminal state with `retention_until IS NULL`. Should always be 0 — the CHECK constraint prevents it — but if it ever fires, the constraint was bypassed (DDL change, manual SQL) and we have data sitting outside the retention plan. Fires above 0.
- Backfill rows look identical to live rows; the worker doesn't differentiate

**Retention sweep:** A daily-tick goroutine deletes `moderation_records` where `retention_until < now() AND legal_hold = false`. The `retention_until` value is populated explicitly by the workflow per record type, so the two retention clocks don't conflate:

- **CSAM-quarantine rows:** `retention_until = ncmec_submitted_at + interval '1 year'`. Per § 2258A(h)(1), the clock starts at *submission to the CyberTipline*, not at workflow closure or detection. Submission can be delayed (Hive outage, NCMEC API down, manual Cloudflare paste step), so we track `ncmec_submitted_at` separately and base the clock on it.
- **Trust-and-safety rows (NSFW + approved-post-review):** `retention_until = closed_at + interval '1 year'`, matching Privacy §3.

**Legal-hold mechanism.** When law enforcement specifically directs retention beyond the statutory minimum (per § 2258A(h)(1) carve-out and Privacy §3 disclosure), an admin sets `legal_hold = true` and `legal_hold_set_at = now()` on the affected row(s) via an admin-only `POST /api/moderation/{id}/hold` endpoint. The retention sweep skips any row with `legal_hold = true`. Clearing the hold (when LE confirms it's lifted) is a separate `DELETE /api/moderation/{id}/hold` call that resets `legal_hold = false` and lets the sweep pick the row up on its next pass. Operator audit log records every hold set/clear with admin user ID + timestamp + free-text reason.

### Review queue UX

Wired into the **existing admin server** (`cmd/admin/`) and **admin frontend** (`admin-frontend/`). Conceptually adjacent to the existing Reports flow — same shape: queue → detail → action.

**Backend additions to `cmd/admin/server.go`:**
- `GET /api/moderation` — list `quarantined_nsfw` rows with `operator_decision IS NULL`; paginated; ordered by `created_at`
- `GET /api/moderation/{id}` — detail view including classifier verdicts + uploader handle + uploader history (prior moderation records) + the image (signed temporary URL with `?blur=1` query param the frontend respects)
- `POST /api/moderation/{id}/decide` — body: `{decision: 'approved_post_review'|'removed'|'escalated'}`; idempotent; writes `operator_decision`, `operator_decided_at`, `operator_decided_by`; on `removed`, runs the propagator; on `escalated`, also calls account-action handlers
- `GET /api/moderation/audit` — read-only view of CSAM-quarantined records (no images shown, just timestamps + NCMEC report IDs + uploader handle); admin sees that the system processed a hit but never the image

**Frontend additions to `admin-frontend/`:**
- `src/pages/Moderation.tsx` — review queue + detail; mirrors `Reports.tsx` patterns
- `src/pages/ModerationAudit.tsx` — CSAM audit log (read-only)
- Sidebar entries next to Reports

**UX requirements per reviewer (carried into the implementation plan):**
- Blur-by-default thumbnails; CSS filter blur on the `<img>` element; click-to-reveal hover state
- Keyboard shortcuts: `j`/`k` next/prev, `space` reveal, `a` approve, `r` remove, `e` escalate to account-action
- Side-by-side classifier confidences (Falconsai score, Hive Visual score, Hive CSAM score) so the operator can see why something was queued
- "Uploader history" panel: previous moderation records for this user — first offense vs repeat
- Audit log row written on every keystroke decision, including the operator admin-user ID

**The review queue NEVER shows CSAM-positive content.** Those go through the fully-automated CSAM workflow described below; they appear in the read-only `ModerationAudit` page after the fact, with metadata only.

### CSAM end-to-end

Fully automated — operator never sees the image:

1. **Detection:** Falconsai unlikely to fire CSAM-specific (it's a general NSFW classifier); Hive CSAM Detection (Safer-via-Hive) is the primary detector, with Cloudflare CSAM Scanning Tool as an independent secondary. A positive at any layer triggers the workflow.
2. **Image bytes:** Move the Spaces object from `uploads/` to `csam-quarantine/` prefix. ACL'd to admin-only IAM identity; no public URL; no API endpoint serves it. Retain **1 year per § 2258A(h)(1)** then delete.
3. **NCMEC submission:**
   - **Preferred path (A):** if Hive's Safer integration submits to NCMEC's CyberTipline directly, capture the `report_id` and persist it on the row. (Open question — see Research gates below.)
   - **Fallback path (B):** operator-registered NCMEC reporter credentials; our worker calls NCMEC's reporter API directly with the image hash + uploader metadata; capture `report_id` on the row.
   - In either case, the report is filed within 24 hours of detection per § 2258A(b).
4. **Account action:** Auto-suspend the user account, revoke all sessions, force-logout. The user receives a generic email reading: *"Your Parley account has been suspended pending review of a Terms of Service violation. If you believe this is in error, contact hello@parley.byexec.com."* That wording is deliberately *not* informative enough to compromise an investigation but *is* enough to satisfy a "you must give a reason" challenge under jurisdictions like the EU DSA Article 17 (which requires a statement of reasons on content/account decisions, even if non-specific). **The legal driver here is investigation-protection norm and DSA-style reason-giving, not § 2258A non-disclosure** — the statute imposes confidentiality on the *report itself* (we can't share the CyberTipline filing with the user) but does not bar telling the user *why* the account was suspended. We choose not to disclose the specific CSAM allegation in the auto-suspension email as a matter of practice (to avoid tipping off bad actors and to avoid making accidental claims against possibly-innocent users that turn out to be false positives), not because the statute requires it.
5. **Audit log:** A `moderation_records` row is written with `state = 'quarantined_csam'`, classifier verdicts, `ncmec_report_id`, retention countdown set. Visible to the operator in the read-only ModerationAudit page.

**No human review step.** The system reports first, and the operator sees the audit entry after the fact.

### NSFW end-to-end

1. **Detection:** Falconsai high-confidence (`nsfw_score >= 0.95`) or Hive Visual Moderation positive.
2. **Image bytes:** Immediate delete from Spaces. We don't keep adult content around, even briefly.
3. **Reference cleanup:** Run the propagator (see below) to clear references in messages/avatars/banners/soundboard.
4. **User notification:** Email + in-app notification, fired for *every* surface — not just message attachments. The email body adapts to the surface so the user understands what disappeared:
   - `message_attachment` / `dm_message_attachment` → "an image you uploaded as a message attachment was removed..."
   - `user_avatar` / `user_banner` → "your profile avatar/banner was removed..."
   - `server_icon` / `server_banner` → "the icon/banner you set on the [server name] server was removed..."
   - `dm_group_avatar` → "the group DM avatar you set was removed..."
   - `soundboard` → "a soundboard sound you uploaded was removed..."
   All include the §5 TOS reference and the first-offense warning text. Without per-surface notifications, a user whose avatar suddenly disappears with no email is left wondering what happened.
   
   First-offense gets a warning ("future violations may result in account suspension"). Repeat offenders escalate to operator review for account action; the moderation_records `user_id` index makes the count cheap.
5. **Operator review:** Row appears in the Moderation queue. Operator can approve-post-review (false positive — but the image is already gone, so this is just a counter-decrement on the user's offense count), confirm-removed (default, no-op), or escalate to account-action.

### Propagation on quarantine

Every `/api/upload` call carries a **purpose** field that the Go API records on the `moderation_records` row. On quarantine, the propagator looks up the purpose and runs a targeted UPDATE:

| Purpose | Quarantine action |
|---|---|
| `message_attachment` | `UPDATE messages SET attachment_url = '', attachment_name = '', attachment_type = '', moderation_removed = true WHERE attachment_url = $1`. UI renders a `[removed by moderation]` stub when `moderation_removed = true`. |
| `dm_message_attachment` | Same handling against `dm_messages` (separate table; same flag column + same three attachment_* fields). |
| `user_avatar` | Clear `users.avatar_url` for matching rows (NULL or `''` per column nullability — `users.avatar_url` is `NOT NULL DEFAULT ''` post-migration #11, so use `''`) |
| `user_banner` | Clear `users.banner_url` for matching rows (same shape as `user_avatar`) |
| `server_icon` | Clear `servers.icon_url` for matching rows (column is nullable — use `NULL`) |
| `server_banner` | Clear `servers.banner_url` for matching rows (verify nullability at implementation time) |
| `dm_group_avatar` | Clear `direct_messages.avatar_url` for matching rows (column is nullable — use `NULL`); group DMs only |
| `soundboard` | `UPDATE soundboard_sounds SET removed_by_moderation = true WHERE file_url = $1` and best-effort delete the Spaces object — quarantined sounds shouldn't continue to play |
| `orphan` | No database-side surface to update (backfill couldn't find a reference). The Spaces object is still handled per the standard quarantine rules: NSFW → delete the Spaces object; CSAM → move to `csam-quarantine/` and run the full CSAM workflow. The "no-op" only refers to the *DB-side propagation step*; the bytes-side handling is unchanged. Audit log entry written; row state transitions to terminal. |
| `unknown` | Best-effort: scan each surface table for the URL string. Should be rare in steady state — only old client builds without the `purpose` field land here. Warning logged at ingest so we know if a client is out of date. |

**Required schema changes** (consolidated; all part of migration #70):
- `messages.moderation_removed BOOLEAN NOT NULL DEFAULT false`
- `dm_messages.moderation_removed BOOLEAN NOT NULL DEFAULT false`
- `soundboard_sounds.removed_by_moderation BOOLEAN NOT NULL DEFAULT false`
- `user_uploads.provisional BOOLEAN NOT NULL DEFAULT false`
- `/api/upload` request body gains `purpose` field (required); existing client call sites updated

**`/api/upload` flow under the new model:**
1. Multipart parse, MIME check, quota reserve (existing behavior unchanged — verified-email + 24h-account-age gate runs before parse)
2. **NEW: Falconsai sync call** with HMAC. On positive (`nsfw_score >= 0.95`):
   - **Write bytes to Spaces under the `pending-csam-check/{generated_id}` prefix synchronously.** This prefix is admin-only ACL'd (no public URL); bytes are durable from this point on regardless of process state. The synchronous write adds ~100 ms to the rejection latency, which is acceptable for a rejection path.
   - Insert a `user_uploads` row with `provisional = true` (a new column added in this migration) referencing that Spaces key. Provisional rows are excluded from quota accounting and from any user-visible listing endpoints. Refund the quota reservation from step 1.
   - Insert `moderation_records` row with `state = 'pending_csam_check_after_reject'`, `purpose` from request body, falconsai_verdict captured, `upload_id` referencing the provisional `user_uploads` row.
   - Return `422 Unprocessable Entity` with code `IMAGE_REJECTED_NSFW` to the user.
   - Kick off a goroutine that calls Hive CSAM Detection on the bytes (re-fetching from Spaces). On Hive CSAM positive: full CSAM workflow (move object from `pending-csam-check/` to `csam-quarantine/`, mark `provisional = false`, file NCMEC report, suspend uploader, set `retention_until` from `ncmec_submitted_at`, transition state to `quarantined_csam`). On Hive CSAM negative: delete object from `pending-csam-check/`, delete the provisional `user_uploads` row, set `retention_until = closed_at + 1 year`, transition state to `quarantined_nsfw`.
3. On Falconsai pass: Spaces write to `uploads/` (existing behavior)
4. **NEW: Insert `moderation_records` row** with `state = 'pending_remote'`, `purpose` from request body, falconsai_verdict captured, `upload_id` referencing the `user_uploads` row
5. Return `{url, moderation_state: 'pending'}` to client (frontend may show a subtle "pending" indicator on the just-uploaded image, but the URL works immediately)
6. Background worker drains `pending_remote` rows asynchronously (Hive Visual + Hive CSAM in parallel)

**Worker recovery for stuck rejection-path rows.** The worker's poll loop also picks up `pending_csam_check_after_reject` rows whose `created_at` is older than 5 minutes — these are rows whose goroutine died mid-call (process crash between 422 response and Hive verdict). Recovery is identical to the in-line goroutine path: re-fetch bytes from `pending-csam-check/`, call Hive CSAM, run the negative or positive branch. Idempotency: each branch checks the row's current state before transitioning to avoid double-action if both the original goroutine and the recovery worker race.

### Verified-email + account-age gate

`/api/upload` requires:
- `users.email_verified_at IS NOT NULL`
- `users.created_at < now() - interval '24 hours'`

Both checks happen before the multipart parse so we don't accept the body wastefully. Errors:
- Email unverified: `403 Forbidden`, code `EMAIL_VERIFICATION_REQUIRED`
- Account too new: `403 Forbidden`, code `ACCOUNT_TOO_NEW_FOR_UPLOADS`, body includes the time at which uploads will become available

The threshold (24 hours) is a constant in code, not a runtime config. Tweaking it is a code change.

### Backfill

All pre-pipeline images get queued. A one-shot script at deploy time (`cmd/admin/backfill_moderation`) runs in two passes:

**Pass 1 — purpose inference.** Resolve each `user_uploads` row's actual purpose by joining against the surface tables that reference its URL. This is a one-time migration cost that prevents the propagator from ever needing to do a `LIKE %url%` scan during quarantine:

```sql
-- Stage in a temp table with explicit purpose per upload
CREATE TEMP TABLE backfill_purposes AS
SELECT u.id AS upload_id, u.user_id,
  CASE
    WHEN EXISTS (SELECT 1 FROM messages m WHERE m.attachment_url LIKE '%' || u.spaces_key || '%') THEN 'message_attachment'
    WHEN EXISTS (SELECT 1 FROM dm_messages dm WHERE dm.attachment_url LIKE '%' || u.spaces_key || '%') THEN 'dm_message_attachment'
    WHEN EXISTS (SELECT 1 FROM users usr WHERE usr.avatar_url LIKE '%' || u.spaces_key || '%') THEN 'user_avatar'
    WHEN EXISTS (SELECT 1 FROM users usr WHERE usr.banner_url LIKE '%' || u.spaces_key || '%') THEN 'user_banner'
    WHEN EXISTS (SELECT 1 FROM servers s WHERE s.icon_url LIKE '%' || u.spaces_key || '%') THEN 'server_icon'
    WHEN EXISTS (SELECT 1 FROM servers s WHERE s.banner_url LIKE '%' || u.spaces_key || '%') THEN 'server_banner'
    WHEN EXISTS (SELECT 1 FROM direct_messages d WHERE d.avatar_url LIKE '%' || u.spaces_key || '%') THEN 'dm_group_avatar'
    WHEN EXISTS (SELECT 1 FROM soundboard_sounds sb WHERE sb.file_url LIKE '%' || u.spaces_key || '%') THEN 'soundboard'
    ELSE 'orphan'  -- genuinely unreferenced; rare but possible (deleted message, orphaned upload)
  END AS purpose
FROM user_uploads u
WHERE NOT EXISTS (SELECT 1 FROM moderation_records WHERE upload_id = u.id);
```

This stage runs once at deploy time; even at 100k uploads, the surface-table joins are bounded and indexed (every URL column is referenced in at most one table). The expensive part is `LIKE` against `messages.attachment_url`; if that column lacks an index, we add one (`CREATE INDEX CONCURRENTLY ON messages(attachment_url) WHERE attachment_url <> '';`) before running the backfill.

**`spaces_key` substring-collision check.** The matching above uses `LIKE '%' || u.spaces_key || '%'` because URL formats vary across the surface tables. This is intentionally loose, but it's susceptible to false positives if one upload's `spaces_key` is a substring of another's. Spaces keys are generated via `generateID()` (UUID-style identifiers, see `cmd/api/upload_handler.go`) which makes substring collisions practically impossible — but the staging-environment backfill run includes a sanity check (`SELECT spaces_key, COUNT(*) FROM backfill_purposes GROUP BY spaces_key HAVING COUNT(*) > 1`) and aborts if any collisions are detected. If the existing key generator ever changes to short or sequential IDs, this check catches the regression before it tags uploads with the wrong purpose.

**Pass 2 — queue insert.**

```sql
INSERT INTO moderation_records (upload_id, user_id, purpose, state, created_at)
SELECT upload_id, user_id, purpose, 'pending_remote', now()
FROM backfill_purposes;
```

`'orphan'` rows are queued the same way; if quarantine fires for an orphan, the propagator does nothing (no surface to update) and just logs the decision. This is acceptable because orphans aren't visible anyway.

Worker drain rate is bounded by the config knob `MODERATION_WORKER_RATE_LIMIT` (default 100/hour). The limit applies to **outbound Hive API calls**, not row-pulls — at v1 each row triggers exactly one Hive Visual call + one Hive CSAM call (parallel), so 100/hour means 200 Hive API requests/hour at the limit, which is what determines Hive cost. If we ever batch multiple records into a single Hive call (probably not at v1 but conceivable), the metric stays Hive-API-calls/hour, not rows/hour. Once the backfill queue drains, real-time uploads get full bandwidth. **Hard time bound: if backfill hasn't drained in 8 weeks, the operator either raises the rate limit or accepts the gap and stops the backfill** — open queues that grow indefinitely become a separate operational risk.

**User-facing experience for backfill quarantines:** Backfill quarantines do **not** send the standard NSFW warning email per row. The user took no recent action they'd associate with the email, and a flood of "your year-old image was removed" notifications is more confusing than informative. Backfill quarantines write the audit log entry, run the propagator, and surface in the operator's review queue normally; if the operator wants to email a specific user (e.g., a repeat-offender threshold was crossed), they do it manually from the review queue. Documented in PRIVACY §6 if the choice persists past v1.

### Cloudflare CSAM Scanning Tool flow

Cloudflare's tool emails the configured notification address on a hash hit; there's no webhook. v1 handling is **manual operator step**:

1. Email arrives at `hello@parley.byexec.com` with the matched URL and metadata
2. Operator opens admin UI → Moderation page → "Cloudflare-flagged" tab → pastes the matched URL
3. Backend looks up the `user_uploads` row by Spaces key (parsed from URL), creates a `moderation_records` row with `state = 'quarantined_csam'` and `cloudflare_csam_hit_at = now()`, runs the standard CSAM workflow (move to `csam-quarantine/`, file NCMEC report, suspend account, audit log entry)

Building inbox-parsing automation is overkill for what should be rare events at v1; revisit if frequency increases.

### Data-handling for the Cloudflare CSAM Tool subprocessor disclosure

The PRIVACY §4 Cloudflare entry was already updated (v5 of the TOS/Privacy spec) to disclose the CSAM Scanning Tool — it operates on cached image content under Cloudflare's existing DPA, and notification routing to `hello@parley.byexec.com` is part of that flow. No additional subprocessor disclosure required for this layer.

### Hive Moderation subprocessor disclosure

Hive is a new subprocessor. PRIVACY §4 already has a stub entry (added in v5 of the TOS/Privacy spec) that explicitly says image-upload feature is gated on capturing Hive's DPA in plain text. **Image-upload moderation cannot be enabled in production until the Hive DPA is captured into PRIVACY §4 with the same direct-quote treatment we gave the other subprocessors.** This is on the README pre-launch checklist.

### NCMEC reporter registration

The operator (Dylan Hart) registers as a CyberTipline reporter at <https://report.cybertip.org/>. Registration produces credentials (NCMEC reporter ID + API token) that go into env vars `NCMEC_REPORTER_ID` and `NCMEC_API_TOKEN` on the API box. The worker uses those credentials when filing reports under fallback path B (manual via NCMEC API). On the README pre-launch checklist.

### PhotoDNA fourth layer

**Deferred to v1.1.** Falconsai + Hive Visual + Hive/Safer + Cloudflare CSAM Tool covers the legally-required floor (NCMEC hash matching) and the classifier-based novel-CSAM path. PhotoDNA's marginal coverage is real but small relative to Hive/Safer's database; the Microsoft application process is involved. Revisit when volume data justifies the integration cost.

### Video uploads

**Forbidden at v1.** The `allowedFileExt` whitelist in `cmd/api/upload_handler.go` currently includes WebM, OGG, and audio formats. The pipeline above only handles still images; video CSAM detection is a different problem (frame-by-frame extraction, temporal classification) with different vendor pricing and operational overhead. **Implementation plan must remove WebM from the allowed extensions list** as part of this work, and document the v1.1 revisit.

The audio formats (MP3, OGG, WAV) stay allowed — audio doesn't have an analogous "is this content sexual" classification problem at our scale, and audio CSAM is rare enough that hash-matching at the storage layer is sufficient if needed in v1.1.

---

## Research gates

These are open questions the implementation plan will need to resolve. They don't block design approval but they block deployment:

1. **Hive Safer's NCMEC submission product.** Does Hive's CSAM Detection API submit to NCMEC's CyberTipline directly (returning a `report_id`), or does it just flag and require us to file independently? This determines whether path A or path B is the active CSAM submission path. Resolved at Hive contract signing.

2. **Hive's DPA on input retention and training.** Required for PRIVACY §4 disclosure. Image-upload feature gated on capturing this in plain text. Reviewer's directive: same direct-quote treatment as Brevo, Cloudflare, LiveKit, etc.

3. **NCMEC reporter API documentation.** The CyberTipline reporter API (the path-B endpoint) — what's the request shape, what authentication, what rate limits? Resolved during NCMEC reporter registration.

4. **Cloudflare CSAM Scanning Tool email format.** What does the email actually contain? URL? Hash? Metadata? Does it inline a thumbnail/preview of the matched image? Needed to (a) build the admin UI's "Cloudflare-flagged" paste form and (b) configure the `hello@parley.byexec.com` mailbox to strip image rendering from emails matching Cloudflare's CSAM-tool sender — the "operator never sees CSAM" architectural goal has a seam right here if the email previews render in the inbox. Resolved at Cloudflare CSAM Scanning Tool enablement; mitigation (IMAP rules / mail-client image-blocking on that sender) lands in the README pre-launch checklist.

5. **Falconsai threshold tuning.** The 0.95/0.05 threshold pair is a starting point. Empirical tuning during early production will refine these. Threshold change is a config-only redeploy (env var on the API), not a code change.

---

## Operational acknowledgments

Things this design accepts as known limitations:

- **Retroactive-quarantine window.** Between `/api/upload` returning a URL and Hive verdict landing, the image is briefly visible. At v1 volume the window is sub-second to a few seconds; under load it could be a few minutes. The Falconsai sync gate catches obvious positives at the door so this window only affects the ambiguous middle.
- **Single-operator review queue.** The reviewer queue is a known pain point. The keyboard-shortcut UX is the mitigation; if volume grows past what one person can triage, the queue backs up and we revisit (e.g., bring in a contracted moderator with admin access scoped to the Moderation page).
- **Backfill drain rate.** All existing uploads will be sent to Hive at 100/hour — for 100k existing images that's ~6 weeks of drain time. Acceptable since the legal exposure is only on CSAM-positive cases (rare); the worst that happens is a hash-matching CSAM image stays unflagged for some weeks longer. We can raise the rate temporarily if Hive cost permits.
- **NSFW false positives are user-visible and irreversible.** Once an NSFW-classified image is deleted from Spaces, it's gone. A false-positive flagged-and-deleted image cannot be restored. The 0.95 Falconsai threshold + Hive's confidence threshold is calibrated to err on the side of false-negatives for the sync layer; the async Hive layer can be more aggressive because retroactive removal is less surprising than upload-time rejection.
- **Operator burnout risk.** Reviewing flagged content is a real psychological load. The reviewer queue's blur-by-default + CSAM-never-shown design is the mitigation; if the queue grows, the operator should disable image upload temporarily rather than work through a backlog of distressing content.
- **Hive API outage handling.** If Hive is down, the worker leaves rows in `pending_remote` indefinitely and they drain when Hive returns. New uploads still pass Falconsai sync gate, get URL returned, sit in `pending_remote` until Hive comes back. We don't block uploads on Hive availability — if Hive is down for hours, that just lengthens the retroactive-quarantine window. **The `moderation_records_pending_remote_age_seconds` metric (alert threshold 1 hour) is the signal that this acknowledgment is currently in effect** — it tells you Hive is unreachable rather than letting the silent retroactive window stretch unbounded.
- **Falconsai-positive uploads still get CSAM-checked, durably.** When Falconsai rejects an upload at the sync gate, we write the bytes to Spaces under the `pending-csam-check/` prefix *before* responding 422 to the user. The synchronous Spaces write adds ~100 ms to the rejection-path latency — acceptable for a rejection. From the moment the user gets 422, the bytes are durable; a goroutine then calls Hive CSAM Detection and resolves the row to either `quarantined_csam` (full CSAM workflow runs) or `quarantined_nsfw` (bytes deleted from `pending-csam-check/`). If the API process dies during the Hive call, the worker's recovery loop picks up `pending_csam_check_after_reject` rows older than 5 minutes and re-runs the Hive call from the durable bytes — no loss of the legally-meaningful CSAM check.

---

## File-by-file change inventory

Helps the implementation plan size the work.

### New files

- `internal/moderation/service.go` — orchestration
- `internal/moderation/falconsai.go` — Falconsai sidecar HTTP client
- `internal/moderation/hive.go` — Hive Moderation API client (Visual + CSAM Detection)
- `internal/moderation/worker.go` — Postgres-polled async worker
- `internal/moderation/propagator.go` — quarantine reference cleanup
- `internal/moderation/ncmec.go` — NCMEC submission (paths A and B)
- `internal/moderation/models.go` — data structures
- `internal/moderation/service_test.go` — unit tests
- `internal/moderation/worker_test.go` — worker poll-loop tests with fake Hive
- `internal/moderation/propagator_test.go` — propagation correctness tests per purpose
- `cmd/admin/handlers_moderation.go` — admin moderation routes
- `admin-frontend/src/pages/Moderation.tsx` — review queue page
- `admin-frontend/src/pages/ModerationAudit.tsx` — CSAM audit log
- `admin-frontend/src/api/moderation.ts` — admin API client wrapper
- `services/moderation-classifier/app.py` — FastAPI sidecar
- `services/moderation-classifier/model.py` — Falconsai model loader
- `services/moderation-classifier/auth.py` — HMAC verification
- `services/moderation-classifier/requirements.txt`
- `services/moderation-classifier/systemd/parley-moderation.service`
- `services/moderation-classifier/README.md`
- `terraform/modules/moderation-classifier-vm/main.tf` — Proxmox VM module
- `cmd/admin/backfill_moderation/main.go` — backfill seeding script

### Modified files

- `cmd/api/upload_handler.go` — add `purpose` field, Falconsai sync call, `moderation_records` row insert, account-age + email-verified gates, remove WebM from allowed extensions
- `cmd/api/helpers.go` — `UploadRequest` shape (or struct equivalent) gains `purpose`
- `cmd/admin/server.go` — register moderation routes
- `cmd/admin/main.go` — wire `backfill_moderation` subcommand
- `admin-frontend/src/App.tsx` — add Moderation + ModerationAudit routes
- `admin-frontend/src/components/Sidebar.tsx` — sidebar entries
- `internal/db/migrations.go` — new migration #70 (moderation_records + flag columns on messages, dm_messages, soundboard_sounds)
- `frontend/src/api/upload.ts` — pass `purpose` from each call site
- `frontend/src/components/chat/MessageInput.tsx` — `purpose: 'message_attachment'` (channels) or `'dm_message_attachment'` (DMs / GCs); the call site already knows which context it's in
- `frontend/src/components/settings/UserSettings.tsx` — `purpose: 'user_avatar'` and `'user_banner'`
- `frontend/src/components/settings/ServerSettings.tsx` — `purpose: 'server_icon'` and `'server_banner'`
- `frontend/src/components/settings/CustomThemeEditor.tsx` — themes don't upload images currently; only updated if it later does
- DM group avatar setter (in DM modal flow) — `purpose: 'dm_group_avatar'`
- Soundboard upload UI — `purpose: 'soundboard'`
- `README.md` pre-launch checklist — already gates on Hive DPA, Cloudflare CSAM Tool, NCMEC registration; add image-upload disable-in-production switch as a runtime feature flag pending end-to-end verification

---

## What this design does NOT cover

- **Video moderation.** Video uploads are forbidden at v1 (WebM removed from allowed extensions). v1.1 revisit.
- **Text-side classification.** §6 of the Privacy Policy explicitly says we don't run sentiment analysis, profile-building, or "predictive" classifiers on text messages. Text moderation is by reports + server-owner moderation only.
- **Audio classification.** Audio uploads (MP3, OGG, WAV) stay allowed without classification at v1.
- **Federated content moderation.** Parley is single-instance at v1; no federation = no inter-instance content propagation problem.
- **Appeals workflow.** First-offense NSFW gets a warning email; repeat triggers account-action. There's no formal appeals process — if a user thinks moderation got it wrong, they email `hello@parley.byexec.com` per the existing TOS §8 escalation path. Building a formal in-product appeals flow is a v1.1 consideration if volume justifies.
- **Multi-operator moderation.** v1 is single-operator. If Parley gains additional admins with moderation scope, the audit log already records `operator_decided_by` so the trail survives; the bigger question (what scopes does a "moderator" admin role have, vs full admin?) is out of scope.
- **Region-specific takedown obligations.** EU DSA, German NetzDG, etc. each have specific obligations on flagged content. v1 operates under US law (Illinois operator); region-specific compliance is a v1.1+ revisit if Parley grows materially in those markets.

---

## Sequencing for the implementation plan

Suggested order so that each task produces working, testable software:

1. **Migration #70.** Schema additions only — `moderation_records` table (with `retention_until`, `legal_hold`, `ncmec_submitted_at`, `upload_id NOT NULL`, terminal-state CHECK constraint), `user_uploads.provisional`, `messages.moderation_removed`, `dm_messages.moderation_removed`, `soundboard_sounds.removed_by_moderation`. No behavior change. Tests: migration applies, schema matches, CHECK constraint rejects bad terminal-state inserts.
2. **Purpose field on `/api/upload`.** Add `purpose` field to request body; plumb through all client call sites with the correct value per surface. Server-side: warn-log on `'unknown'` so we know a client is out of date. Tests: each call site passes a real purpose.
3. **Falconsai sidecar.** Standalone Python service runs locally, exposes `/classify` with HMAC-timestamp auth (60-second window), returns scores. Smoke-tested with curl + known-good and known-bad images. No Go integration yet.
4. **Falconsai HTTP client + sync gate at `/api/upload`.** Go API calls the sidecar synchronously. On Falconsai pass: write Spaces under `uploads/`, insert `user_uploads` row, insert `moderation_records` row in `pending_remote`, return URL. On Falconsai positive: write bytes to Spaces under `pending-csam-check/`, insert provisional `user_uploads` row, refund quota, insert `moderation_records` row in `pending_csam_check_after_reject`, return 422. Goroutine and Hive call are stubbed in this step (goroutine just transitions straight to `quarantined_nsfw` and deletes the bytes); real Hive CSAM call wires in at step 8. Tests: both paths transition states correctly; bytes durability verified by killing the API mid-test.
5. **Worker scaffolding.** Worker goroutine drains `pending_remote` rows AND recovers stuck `pending_csam_check_after_reject` rows older than 5 minutes (re-fetches bytes from `pending-csam-check/`, re-runs the rejection-path goroutine logic). Stubbed Hive client always returns `clean`. Stuck-row monitors emit metrics. Tests: worker drains pending_remote rows; recovery loop picks up stuck pending_csam_check_after_reject rows after a simulated crash; retention sweep uses `retention_until` correctly per record type and skips rows with `legal_hold = true`.
6. **Hive Moderation API client.** Real integration with both Visual Moderation and CSAM Detection endpoints. Wire into worker (replaces stub from step 5). Wire into the Falconsai-rejection goroutine (replaces stub from step 4). Both code paths now call real Hive CSAM. Gated behind `MODERATION_HIVE_ENABLED` feature flag for staging rollout. Tests: client, retry logic, verdict parsing.
7. **Propagator on quarantine.** Per-purpose UPDATE queries for all 8 surfaces (`message_attachment`, `dm_message_attachment`, `user_avatar`, `user_banner`, `server_icon`, `server_banner`, `dm_group_avatar`, `soundboard`); plus per-surface user notification email + in-app notification. Tested per purpose with seeded data. NSFW quarantine workflow works end-to-end including the "user's avatar disappeared" notification.
8. **CSAM workflow.** Move-to-quarantine-prefix on Spaces (`csam-quarantine/`), account-suspend with the reviewer-blessed wording ("suspended pending review of a Terms of Service violation; contact hello@..."), audit log row, NCMEC submission (path B implementation against NCMEC reporter API), `ncmec_submitted_at` populated when submission succeeds, `retention_until` computed from that. Path A wired in once Hive contract confirms it. Workflow handles both `pending_remote → quarantined_csam` (async path) and `pending_csam_check_after_reject → quarantined_csam` (sync-rejection path).
9. **Admin moderation UI (backend routes).** `GET /api/moderation`, `GET /api/moderation/{id}`, `POST /api/moderation/{id}/decide`, `GET /api/moderation/audit`, plus the LE-hold endpoints `POST /api/moderation/{id}/hold` and `DELETE /api/moderation/{id}/hold`. Tested with admin auth.
10. **Admin moderation UI (frontend pages).** `Moderation.tsx`, `ModerationAudit.tsx`, sidebar entries. Manual UX verification in browser including blur-on-load + keyboard shortcuts.
11. **Verified-email + account-age gate.** Pre-multipart-parse check in `/api/upload`. Tests: unverified email rejected, account-too-new rejected, both clear the gate as expected.
12. **Backfill seeding script (two-pass).** `cmd/admin/backfill_moderation` runs Pass 1 (purpose inference: join `user_uploads` against the surface tables; populate `backfill_purposes` temp table; verify `messages.attachment_url` index exists, create concurrently if not) then Pass 2 (insert `moderation_records` rows in `pending_remote` with the resolved purpose). Worker rate limit `MODERATION_WORKER_RATE_LIMIT=100/h` during backfill. Hard time bound: 8 weeks; raise rate or stop backfill if not drained by then. Backfill quarantines do NOT email users per row.
13. **Cloudflare CSAM Tool admin UI.** "Paste flagged URL" form in admin Moderation page; triggers full CSAM workflow on submit.
14. **Remove WebM from allowed extensions.** One-line change in `cmd/api/upload_handler.go::allowedFileExt`. Existing WebM uploads remain accessible; new uploads of WebM are blocked.
15. **Disable-image-upload feature flag.** Runtime env var (`IMAGE_UPLOAD_ENABLED`); when `false` or unset-in-dev, `/api/upload` returns 503 for image MIME types but allows audio. **In production, the API process refuses to boot if `IMAGE_UPLOAD_ENABLED` is not explicitly set** — there is no code default fallback. The asymmetry of the failure mode (image upload accidentally enabled with broken moderation) is severe enough to justify boot-time refusal: the operator must consciously set the value either way. The production deploy pipeline additionally checks for the env var being explicitly present in the systemd unit file before allowing a release. README pre-launch checklist is satisfied when this is flipped to `true` after end-to-end verification.

---

## v1 changes (2026-04-27 — external review applied)

The v1 reviewer returned with "approved with the note that issues 1 and 2 should land before image-upload moderation goes live in production, and 3 should be verified during backfill testing in staging." All five substantive items were applied to the spec; smaller hygiene items folded into the relevant sections rather than separate changelog entries.

**Substantive fixes:**

1. **CSAM retention clock split from trust-and-safety clock.** Original spec used a single `closed_at + 1 year` index for both retention paths. Reviewer correctly flagged that § 2258A(h)(1)'s clock starts at *NCMEC submission*, not closure, and that LE-hold preservation directives need a way to extend retention without manually editing rows. Schema now has explicit `retention_until` populated by the workflow per record type, plus `ncmec_submitted_at` so CSAM rows compute their `retention_until` from submission, plus `legal_hold` boolean + admin-only set/clear endpoints that block the sweep.

2. **Hive CSAM check on Falconsai-positive uploads.** Original spec rejected Falconsai-positive uploads at the sync gate without running CSAM detection on the bytes; reviewer flagged that as a legal seam ("a 95%-confidence-NSFW result on an image whose age the model did not assess is plausibly 'facts or circumstances' that warrant scrutiny"). Updated spec hands the bytes to a background goroutine on Falconsai-positive that calls Hive CSAM Detection before dropping; if Hive CSAM positive, full CSAM workflow runs (move bytes to `csam-quarantine/`, NCMEC report, account suspend). User-perceived latency is unchanged (instant 422). New state `pending_csam_check_after_reject` covers the audit trail for this branch; `upload_id` made nullable since the bytes never reach Spaces.

3. **Backfill propagator pre-populated with purpose.** Original spec used `'unknown'` purpose for all backfilled rows, which would force the propagator into a `LIKE %url%` scan against potentially millions of rows on each quarantine. Reviewer flagged the performance risk. Updated backfill is two-pass: pass 1 resolves each upload's purpose by joining `user_uploads` against the surface tables (with an index on `messages.attachment_url` if not present); pass 2 inserts the rows with explicit purpose. `'orphan'` is now the genuine fallback for uploads with no surface reference; `'unknown'` is preserved for old client builds without the `purpose` field. Backfill quarantines do not email users per row, since the user took no recent action they'd associate with the email.

4. **CSAM auto-suspension wording.** Original spec said the user got "no specific reason given." Reviewer correctly noted that (a) § 2258A doesn't actually impose non-disclosure on the platform regarding the user (it covers the report itself), and (b) "no reason at all" creates DSA Article 17 exposure once Parley grows. Updated wording: "Your Parley account has been suspended pending review of a Terms of Service violation. If you believe this is in error, contact hello@parley.byexec.com." Rationale block now correctly identifies investigation-protection norm and DSA reason-giving as the legal driver, not § 2258A.

5. **Production deploy enforcement of `IMAGE_UPLOAD_ENABLED`.** Original spec said the env var defaults to false. Reviewer's belt-and-suspenders point: production deploy should fail if the env var is unset, not rely on a code default. Updated spec: API process refuses to boot in production if env var unset; deploy pipeline checks for the env var being explicitly present in the systemd unit file.

**Smaller hardening items (folded inline):**

- **Sidecar HMAC includes timestamp + 60-second replay window.** Five-line fix; closes the captured-and-replayed `/classify` seam even though internal-bridge isolation makes it unlikely to matter in practice.
- **Worker exposes `moderation_records_pending_remote_age_seconds` metric** with a 1-hour alert threshold so silent Hive outages don't extend the retroactive-quarantine window indefinitely.
- **Per-surface user notifications on quarantine.** Original spec only described the message-attachment notification path; reviewer noted that an avatar/banner suddenly disappearing without explanation is its own confusing UX. Propagator now fires the email + in-app notification for every surface, with the body adapting to which surface was cleared.
- **Cloudflare CSAM Tool email format research-gate** expanded to include "verify the email doesn't inline-render the matched image" and added an IMAP-rule mitigation to the README pre-launch checklist. The "operator never sees CSAM" goal had a seam right at the inbox.

**Reviewer items deferred to v1.1 (not applied to spec):**

- **`LISTEN`/`NOTIFY` instead of 2-second polling.** Polling at v1 volume is fine; LISTEN/NOTIFY is a meaningful efficiency gain at scale that can be retrofitted without redesigning the worker interface. Defer.
- **Splitting `'unknown'` further into `'backfill'` / `'legacy_client'` / etc.** I added `'orphan'` (which closes the largest concern) but kept `'unknown'` for old clients. Splitting further is tidy but not load-bearing.
- **Cloudflare paste-form SLA + alerting beyond email.** Operator-on-vacation single-point-of-failure is real but unlikely at v1 single-operator volume. v1.1: add a Slack/PagerDuty alert from a mailbox-watcher on Cloudflare CSAM-tool sender, separate from inbox-parsing automation.
- **Audio classification / soundboard length limits / audio NCII.** TOS §5 covers audio harassment but the technical controls don't. v1.1 revisit if audio abuse becomes a real vector.
- **Verified-email gate exposing precise account-creation timestamp via the "available in N hours" hint.** Tiny information leak; reviewer flagged as paranoid. Not changing for v1; could round to nearest hour if it ever becomes a concern.

**Reviewer items requiring staging verification, not spec changes:**

- **Backfill propagator performance** verified during staging-environment backfill testing before production rollout. The two-pass design (per item 3 above) is the spec-side fix; staging verification confirms it works at production scale.
- **Cloudflare CSAM Tool email format** captured at first hit during staging Cloudflare CSAM Scanning Tool enablement, fed back into the admin UI's paste form design and the IMAP rule.

---

## v2 changes (2026-04-27 — second external review applied)

The v2 reviewer returned with "approve with the two items above as code-freeze-blockers; everything else can be addressed in the implementation plan." Both code-freeze-blockers landed in the spec; smaller tightenings folded inline.

**Substantive fixes:**

1. **CHECK constraint + stuck-state monitors.** v1 spec relied on workflow code to populate `retention_until` correctly on every terminal-state transition; if the code broke, rows would sit indefinitely with `retention_until = NULL` and the retention sweep would silently skip them. Reviewer correctly flagged this as a real silent-failure mode. v2 adds a CHECK constraint at the schema level (`state IN (...pending states) OR retention_until IS NOT NULL`) so the database refuses to accept terminal rows without retention. Plus three Prometheus-style monitors: `pending_remote_age_seconds` (existing, kept), `stuck_csam_submission_count` (CSAM rows with `ncmec_submitted_at IS NULL` older than 6h), `terminal_no_retention_count` (which should always be 0 because of the CHECK constraint, but alerts above 0 if a constraint bypass ever occurs). § 2258A's "as soon as reasonably possible" submission requirement is now monitored in addition to being targeted by the workflow.

2. **Durable byte storage for the Falconsai-positive Hive CSAM check.** v1 design held the bytes in process memory and ran Hive CSAM in a goroutine post-422; if the API process crashed during the goroutine, the bytes were lost and the row sat in `pending_csam_check_after_reject` with no path forward. v2 reviewer correctly noted this was the rare case where the loss matters most (a determined bad actor whose crash-window upload was never reported to NCMEC).

   v2 spec writes bytes to Spaces under a new `pending-csam-check/` admin-only-ACL'd prefix *synchronously* before the 422 response — adds ~100 ms to rejection latency, acceptable for a rejection path. The bytes are durable from the moment the user gets 422. The goroutine then runs Hive CSAM; on negative, it deletes the bytes; on positive, it moves them to `csam-quarantine/` and runs the full CSAM workflow. If the API crashes mid-Hive-call, the worker's recovery loop drains `pending_csam_check_after_reject` rows older than 5 minutes and re-runs the Hive call from durable storage.

   Schema-wise, this restores `upload_id NOT NULL` (the v1 nullable workaround is gone) and adds `user_uploads.provisional` to mark rows whose bytes are in `pending-csam-check/` and aren't user-visible / quota-counted. Spec is now internally consistent: every `moderation_records` row references a real `user_uploads` row.

**Smaller tightenings folded inline:**

- **Orphan rows still handle the underlying Spaces object on quarantine.** v1 wording said "no-op" which only covered the DB-side propagation. v2 clarifies that bytes-side handling (delete on NSFW, move on CSAM) runs the same as for any other purpose; only the DB-side UPDATE is skipped because there's no surface to update.
- **Backfill `LIKE` substring-collision check.** Spaces keys are UUID-style and substring-collisions are practically impossible, but the staging backfill run includes an explicit collision-detection query and aborts if any collisions are found. This catches the regression-class where the key generator changes to short or sequential IDs.
- **`MODERATION_WORKER_RATE_LIMIT` clarified as Hive-API-calls/hour, not rows/hour.** At v1 each row is one Visual + one CSAM call; the metric is what determines Hive cost regardless of any future batching changes.
- **Op-acks Hive-outage paragraph linked to the worker-metric alert.** The 1-hour `pending_remote_age_seconds` alert is the signal that this op-ack is currently in effect — the connection is now explicit.

**Reviewer items deferred to implementation plan (not spec-level):**

- **State enum as Go const block in `internal/moderation/models.go`** with constants used everywhere — implementation hygiene, captured as a TDD discipline note in the implementation plan.
