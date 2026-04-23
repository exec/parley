# PgBouncer `auth_query` migration (D5 follow-up)

**Finding:** `terraform/userdata-db.sh` historically wrote the `parley` user's
password in plaintext to `/etc/pgbouncer/userlist.txt` (mode 640,
`postgres:postgres`). A filesystem-level compromise of CT 101 (DB container)
yielded the full application DB password — not just a pgbouncer-scoped
credential.

**Fix.** PgBouncer's `auth_query` pattern. A dedicated low-privilege role
(`pgbouncer_auth`) holds the only credential on disk. Every per-client
authentication call runs through a `SECURITY DEFINER` function
(`user_lookup(text)`) that reads `pg_authid` on the function-owner's
behalf and returns `(usename, passwd)`. The lookup role cannot `SELECT`
`pg_authid` or `pg_shadow` directly — it can only `EXECUTE` the function.

| Asset                                         | Before          | After                                                |
| --------------------------------------------- | --------------- | ---------------------------------------------------- |
| Credential on disk at `/etc/pgbouncer/userlist.txt` | `parley`'s plaintext password | `pgbouncer_auth`'s password (low-priv lookup role)   |
| Can read `pg_authid`                          | n/a             | only `postgres` (function owner); `pgbouncer_auth` goes through `user_lookup()` |
| Blast radius of userlist.txt leak             | full app DB access | can call `user_lookup(text)` to retrieve individual SCRAM hashes (still offline-crackable, but narrower and rotatable) |

The provisioning script (`terraform/userdata-db.sh`) now implements this on
fresh boots. This runbook describes how to migrate **the existing CT 101**
without reprovisioning.

---

## Prerequisites

- SSH/pct access to CT 101 (DB container, `10.10.10.10`).
- PostgreSQL superuser access (`sudo -u postgres psql`).
- A short maintenance window for a `systemctl reload pgbouncer` — no client
  downtime (reload drains gracefully) but new connections during the reload
  will briefly pause.

## Apply

All steps run on CT 101 as `root` (or via `pct enter 101`).

### 1. Generate and persist the pgbouncer_auth password

```bash
install -d -m 755 /etc/pgbouncer
PGB_AUTH_PW_FILE=/etc/pgbouncer/auth_user.pw
umask 077
openssl rand -base64 32 | tr -d '\n' > "$PGB_AUTH_PW_FILE"
umask 022
chown root:root "$PGB_AUTH_PW_FILE"
chmod 600 "$PGB_AUTH_PW_FILE"
PGB_AUTH_PW=$(cat "$PGB_AUTH_PW_FILE")
```

### 2. Create the lookup role and function

Note the `\$func\$` — shell escape for the PostgreSQL dollar-quote delimiter.

```bash
sudo -u postgres psql <<EOF
DO \$\$ BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'pgbouncer_auth') THEN
    CREATE ROLE pgbouncer_auth LOGIN PASSWORD '$PGB_AUTH_PW';
  ELSE
    ALTER ROLE pgbouncer_auth LOGIN PASSWORD '$PGB_AUTH_PW';
  END IF;
END \$\$;

\c parley

CREATE OR REPLACE FUNCTION user_lookup(IN p_usename text, OUT usename text, OUT passwd text)
  RETURNS record
  LANGUAGE sql
  SECURITY DEFINER
  SET search_path = pg_catalog
  AS \$func\$
    SELECT rolname::text, rolpassword::text
      FROM pg_authid
     WHERE rolname = p_usename;
  \$func\$;

REVOKE ALL ON FUNCTION user_lookup(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION user_lookup(text) TO pgbouncer_auth;
EOF
```

### 3. Update `pgbouncer.ini`

Add the two lines below inside `[pgbouncer]` in `/etc/pgbouncer/pgbouncer.ini`:

```ini
auth_user = pgbouncer_auth
auth_query = SELECT usename, passwd FROM user_lookup($1)
```

Ensure `auth_type = scram-sha-256` is already set (it is in current prod).

### 4. Rewrite `userlist.txt`

Replace the parley row with the lookup role's credentials:

```bash
PGB_AUTH_PW=$(cat /etc/pgbouncer/auth_user.pw)
echo "\"pgbouncer_auth\" \"$PGB_AUTH_PW\"" > /etc/pgbouncer/userlist.txt
chmod 640 /etc/pgbouncer/userlist.txt
chown postgres:postgres /etc/pgbouncer/userlist.txt
```

### 5. Reload PgBouncer

```bash
systemctl reload pgbouncer
# If reload doesn't pick up auth_user/auth_query changes on your pgbouncer
# version, restart instead:
systemctl restart pgbouncer
systemctl is-active pgbouncer   # expect: active
```

---

## Verification

### Application path still works

From an API container (CT 102, `10.10.10.11`):

```bash
PGPASSWORD='<db_password>' psql -h 10.10.10.10 -p 6432 -U parley -d parley -c 'SELECT 1;'
# expect:  ?column?
#          ----------
#                  1
```

From CT 101 itself (loopback pgbouncer):

```bash
PGPASSWORD='<db_password>' psql -h 127.0.0.1 -p 6432 -U parley -d parley -c 'SELECT 1;'
```

### `parley` cannot read `pg_authid` directly

Confirms the application role still has no way to read stored hashes, even
for itself — only `postgres` (function owner) can, and `pgbouncer_auth` only
via the `EXECUTE` grant.

```bash
sudo -u postgres psql -d parley <<'EOF'
SET ROLE parley;
SELECT COUNT(*) FROM pg_authid;   -- expect: ERROR: permission denied for table pg_authid
RESET ROLE;
EOF
```

### `parley` cannot execute or inspect `user_lookup`

```bash
sudo -u postgres psql -d parley <<'EOF'
SET ROLE parley;
SELECT * FROM user_lookup('parley');  -- expect: ERROR: permission denied for function user_lookup
RESET ROLE;
EOF
```

If `parley` ever shows up in `pg_proc` ACLs for `user_lookup`, revoke:

```sql
REVOKE ALL ON FUNCTION user_lookup(text) FROM parley;
```

### `pgbouncer_auth` can execute `user_lookup` but nothing else

```bash
sudo -u postgres psql -d parley <<'EOF'
SET ROLE pgbouncer_auth;
SELECT * FROM user_lookup('parley');  -- expect: one row with usename='parley', passwd='SCRAM-SHA-256$...'
SELECT * FROM pg_authid LIMIT 1;      -- expect: ERROR: permission denied
RESET ROLE;
EOF
```

### `userlist.txt` no longer contains the `parley` password

```bash
grep -c '^"parley"' /etc/pgbouncer/userlist.txt   # expect: 0
grep -c '^"pgbouncer_auth"' /etc/pgbouncer/userlist.txt   # expect: 1
```

---

## Rollback

If `auth_query` breaks logins for any reason (e.g. unexpected pgbouncer
version quirk, bad function definition), fall back to the previous
userlist.txt-only setup without destroying the new role/function.

1. Restore the parley row in `userlist.txt`:

   ```bash
   echo "\"parley\" \"<db_password>\"" > /etc/pgbouncer/userlist.txt
   chmod 640 /etc/pgbouncer/userlist.txt
   chown postgres:postgres /etc/pgbouncer/userlist.txt
   ```

2. Comment out the two new directives in `/etc/pgbouncer/pgbouncer.ini`:

   ```ini
   # auth_user = pgbouncer_auth
   # auth_query = SELECT usename, passwd FROM user_lookup($1)
   ```

3. Reload:

   ```bash
   systemctl reload pgbouncer
   ```

The `pgbouncer_auth` role and `user_lookup()` function can stay in place
during rollback — they're inert without `auth_user`/`auth_query` wiring.
Drop them only once you're confident you won't need to roll back again:

```sql
DROP FUNCTION IF EXISTS user_lookup(text);
DROP ROLE IF EXISTS pgbouncer_auth;
```

---

## Security reasoning

### Why `SECURITY DEFINER` is safe here

`SECURITY DEFINER` means the function runs with the privileges of its owner
(`postgres`, the DB superuser), not the caller (`pgbouncer_auth`). The usual
risks with `SECURITY DEFINER`:

1. **Arbitrary arbitrary-input to a privileged function.** Mitigated here:
   the function takes a single `text` parameter used only as an equality
   predicate in one query. No dynamic SQL, no `EXECUTE`. A caller can only
   ask "give me the row for rolname = $1" — there's no injection surface.
2. **`search_path` hijacking.** Mitigated by `SET search_path = pg_catalog`.
   Without this, a caller could create a malicious `pg_authid` view in
   their own schema and get the function to read that instead. Pinning
   `search_path` inside the function forces it to resolve `pg_authid` to
   the real catalog table.
3. **Broader-than-needed grants.** Mitigated by
   `REVOKE ALL ... FROM PUBLIC; GRANT EXECUTE ... TO pgbouncer_auth;` —
   only the pooler's lookup role can invoke it. Application users
   (`parley`) cannot call it, so even if `parley` is compromised, the
   attacker cannot pivot through this function.

### What an attacker gains from `pgbouncer_auth` leak

Versus the old posture:

- **Old:** stealing `userlist.txt` yielded the parley user's plaintext
  password. Immediate full application DB access.
- **New:** stealing `userlist.txt` yields `pgbouncer_auth`'s password.
  The attacker can log in as `pgbouncer_auth` and call `user_lookup()`
  to retrieve any role's SCRAM-SHA-256 hash. They still need to crack
  the hash (SCRAM-SHA-256 with a random password → infeasible) OR use
  the hash via a PostgreSQL client that can authenticate with a stored
  hash (uncommon but possible). Either way, the attacker cannot `SELECT`
  from any application table as `pgbouncer_auth` — that role has no
  grants on application schemas.

### Why `pg_authid` vs `pg_shadow`

`pg_shadow` is a view over `pg_authid` filtered to rows the caller can
already see. A `SECURITY DEFINER` function reading `pg_authid` directly is
simpler (no view indirection) and well-precedented in the
[pgbouncer docs](https://www.pgbouncer.org/config.html#authentication-settings).

### Why the `pgbouncer_auth` password rotates independently

Stored in `/etc/pgbouncer/auth_user.pw` on CT 101. Rotating it is a matter
of generating a new value, `ALTER ROLE pgbouncer_auth PASSWORD ...`,
rewriting `userlist.txt`, and reloading pgbouncer — no application config
touched. The parley user's password is unrelated and lives only in
the API container's env (consumed by `DATABASE_URL`).
