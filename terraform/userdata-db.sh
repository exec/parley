#!/bin/bash
set -e

# Works as a cloud-init script on DO droplets and as a re-runnable provisioner
# script inside LXC containers. All non-idempotent steps are guarded so a second
# run is a no-op. Firewall config (ufw) is skipped inside LXC because the
# Proxmox host firewall already handles inter-container isolation.

IS_LXC=$(systemd-detect-virt --container 2>/dev/null || echo none)
case "$IS_LXC" in lxc|systemd-nspawn) IS_LXC=yes ;; *) IS_LXC=no ;; esac

echo "=== Starting Parley Database setup (LXC=$IS_LXC) ==="

# Update system
echo "=== Updating system packages ==="
export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get upgrade -y

# Install PostgreSQL
echo "=== Installing PostgreSQL ==="
apt-get install -y postgresql postgresql-contrib

# Start PostgreSQL service
echo "=== Starting PostgreSQL service ==="
systemctl start postgresql
systemctl enable postgresql

# Wait for PostgreSQL to be ready
sleep 3

# Create parley user and database (idempotent)
echo "=== Creating Parley database and user ==="
sudo -u postgres psql <<EOF
DO \$\$ BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'parley') THEN
    CREATE USER parley WITH PASSWORD '${db_password}';
  ELSE
    ALTER USER parley WITH PASSWORD '${db_password}';
  END IF;
END \$\$;

SELECT 'CREATE DATABASE parley OWNER parley'
 WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'parley')\gexec

GRANT ALL PRIVILEGES ON DATABASE parley TO parley;

\c parley

GRANT ALL ON SCHEMA public TO parley;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO parley;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO parley;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON FUNCTIONS TO parley;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON PROCEDURES TO parley;
EOF

# pgbouncer auth_query setup (D5 follow-up — replaces plaintext userlist.txt).
# The pgbouncer_auth role is a low-privilege lookup role that can ONLY execute
# the user_lookup SECURITY DEFINER function. It cannot SELECT pg_authid or
# pg_shadow directly — only postgres (the function owner) has that right.
# pgbouncer holds this role's password to open its own auth-DB connection;
# every per-client lookup runs through user_lookup() and never exposes hashes
# for any other user to the pgbouncer_auth role.
echo "=== Configuring pgbouncer auth_query (D5 follow-up) ==="

# Generate a random password for pgbouncer_auth if one hasn't been stored yet.
# Persisted in /etc/pgbouncer/auth_user.pw (mode 640, owned by postgres) so
# re-runs of this script stay idempotent and don't churn the role password.
PGB_AUTH_PW_FILE=/etc/pgbouncer/auth_user.pw
install -d -m 755 /etc/pgbouncer
if [ ! -s "$PGB_AUTH_PW_FILE" ]; then
    umask 077
    openssl rand -base64 32 | tr -d '\n' > "$PGB_AUTH_PW_FILE"
    umask 022
fi
PGB_AUTH_PW=$(cat "$PGB_AUTH_PW_FILE")

sudo -u postgres psql <<EOF
DO \$\$ BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'pgbouncer_auth') THEN
    CREATE ROLE pgbouncer_auth LOGIN PASSWORD '$PGB_AUTH_PW';
  ELSE
    ALTER ROLE pgbouncer_auth LOGIN PASSWORD '$PGB_AUTH_PW';
  END IF;
END \$\$;

\c parley

-- Lookup function owned by postgres (superuser). SECURITY DEFINER lets
-- pgbouncer_auth read pg_authid indirectly via this function even though it
-- has no direct SELECT privilege on pg_authid. search_path pinned to
-- pg_catalog so a future malicious object in public can't hijack the call.
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

-- Tighten: revoke PUBLIC default, grant EXECUTE only to the lookup role.
REVOKE ALL ON FUNCTION user_lookup(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION user_lookup(text) TO pgbouncer_auth;
EOF

# Configure PostgreSQL to accept connections from API droplets
echo "=== Configuring PostgreSQL for remote connections ==="

# Get the VPC private IP of this database server (10.x.x.x on DO, 192.168.x.x on Proxmox)
DB_PRIVATE_IP=$(hostname -I | tr ' ' '\n' | grep -E '^10\.|^192\.168\.' | head -1)
if [ -z "$DB_PRIVATE_IP" ]; then
  DB_PRIVATE_IP=$(hostname -I | awk '{print $1}')
  echo "WARNING: Could not find VPC IP, falling back to $DB_PRIVATE_IP"
fi
echo "Database private IP: $DB_PRIVATE_IP"

# Update pg_hba.conf to allow connections from the local subnet
# Derive subnet from this VM's IP — works on both DO (10.x.x.x) and Proxmox (192.168.x.x)
LAN_SUBNET="$${DB_PRIVATE_IP%.*}.0/24"
PG_HBA=$(find /etc/postgresql -name pg_hba.conf | head -1)
if [ -n "$PG_HBA" ] && ! grep -q "Parley API connections" "$PG_HBA"; then
    echo "# Parley API connections - allow LAN subnet only" >> "$PG_HBA"
    echo "host    all             all             $LAN_SUBNET          scram-sha-256" >> "$PG_HBA"
fi

# Bind PostgreSQL to loopback + the private vmbr1 IP only. pg_hba.conf still
# gates per-subnet, but binding here avoids any accidental exposure if hba is
# ever misconfigured. Tenants on vmbr1 still reach the DB via 10.10.10.10.
# IMPORTANT: include ::1 — pgbouncer's [databases] entry uses host=localhost,
# which resolves to ::1 first on Debian. Dropping ::1 makes pgbouncer auth
# fail with "server login has been failing, cached error: connect failed".
PG_CONF=$(find /etc/postgresql -name postgresql.conf | head -1)
if [ -n "$PG_CONF" ]; then
    sed -i "s/^listen_addresses = 'localhost'/listen_addresses = '127.0.0.1,::1,10.10.10.10'/" "$PG_CONF"
    sed -i "s/^listen_addresses = '\*'/listen_addresses = '127.0.0.1,::1,10.10.10.10'/" "$PG_CONF"

    # Ensure it's not commented
    if grep -q "^#listen_addresses" "$PG_CONF"; then
        sed -i "s/^#listen_addresses = 'localhost'/listen_addresses = '127.0.0.1,::1,10.10.10.10'/" "$PG_CONF"
    fi

    echo "=== Tuning PostgreSQL max_connections ==="
    sed -i "s/^#*max_connections.*/max_connections = 100/" "$PG_CONF"
    # Shared buffers: 25% of RAM (4GB droplet → 1GB)
    sed -i "s/^#*shared_buffers.*/shared_buffers = 1GB/" "$PG_CONF"
    # Effective cache size: 75% of RAM
    sed -i "s/^#*effective_cache_size.*/effective_cache_size = 3GB/" "$PG_CONF"
    # Work memory for sort operations
    sed -i "s/^#*work_mem.*/work_mem = 4MB/" "$PG_CONF"
    # Huge pages: let Postgres use them if the kernel has them available.
    # With 1GB shared_buffers, huge pages save TLB entries and reduce
    # kernel memory overhead. "try" falls back gracefully if unavailable.
    sed -i "s/^#*huge_pages.*/huge_pages = try/" "$PG_CONF"
fi

# Enable huge pages in the kernel (2MB pages; 512 pages covers 1GB shared_buffers).
# Guard with || true: if the kernel lacks huge pages support (e.g. cloud kernels),
# sysctl -w returns non-zero and would abort this set -e script unnecessarily.
# PostgreSQL's huge_pages=try already falls back gracefully.
grep -q "^vm.nr_hugepages=" /etc/sysctl.conf || echo "vm.nr_hugepages=512" >> /etc/sysctl.conf
sysctl -w vm.nr_hugepages=512 || true

# Install Redis for cross-node WebSocket broadcasting
echo "=== Installing Redis ==="
apt-get install -y redis-server

# Configure Redis — bind to loopback + all non-loopback IPs, require password
echo "=== Configuring Redis (LAN-only, authenticated) ==="
# Collect all non-loopback IPs — works on both DO (10.x.x.x VPC) and Proxmox (192.168.x.x LAN)
ALL_PRIVATE_IPS=$(hostname -I | tr ' ' '\n' | grep -v '^127\.' | grep -v '^::' | tr '\n' ' ' | xargs)
echo "Detected private IPs: $ALL_PRIVATE_IPS"

# Bind to loopback and all LAN IPs (never bare 0.0.0.0)
sed -i "s/^bind .*/bind 127.0.0.1 $ALL_PRIVATE_IPS/" /etc/redis/redis.conf 2>/dev/null || true
# Require password authentication — handle both commented and active requirepass lines
sed -i "s/^#\? *requirepass .*/requirepass ${redis_password}/" /etc/redis/redis.conf 2>/dev/null || true
grep -q "^requirepass" /etc/redis/redis.conf || echo "requirepass ${redis_password}" >> /etc/redis/redis.conf
# Keep protected mode on
sed -i "s/^protected-mode .*/protected-mode yes/" /etc/redis/redis.conf 2>/dev/null || true

# Restart Redis with new configuration
systemctl restart redis-server
systemctl enable redis-server

# Configure firewall — DB/Redis only accessible from LAN subnet, not public internet.
# Skipped in LXC: the Proxmox host firewall handles inter-container isolation,
# and ufw/iptables in unprivileged LXC has reliability issues (sudo hangs
# after ufw reconfigures netfilter rules mid-script).
if [ "$IS_LXC" = "no" ] && command -v ufw >/dev/null 2>&1; then
  echo "=== Configuring firewall (LAN-only DB ports) ==="
  ufw default deny incoming
  ufw default allow outgoing
  ufw allow 22/tcp                                                   # SSH
  ufw allow from "$LAN_SUBNET" to any port 5432 proto tcp           # PostgreSQL — LAN only
  ufw allow from "$LAN_SUBNET" to any port 6432 proto tcp           # PgBouncer — LAN only
  ufw allow from "$LAN_SUBNET" to any port 6379 proto tcp           # Redis — LAN only
  ufw --force enable
else
  echo "=== Skipping ufw config (LXC — Proxmox firewall handles isolation) ==="
fi

# Restart PostgreSQL with new configuration
echo "=== Restarting PostgreSQL ==="
systemctl restart postgresql

# Verify PostgreSQL is running
echo "=== Verifying PostgreSQL status ==="
systemctl status postgresql --no-pager || true

# Check connections
echo "=== Testing database connection ==="
timeout 30 sudo -u postgres psql -c "SELECT version();" || echo "(could not fetch version — continuing)"
timeout 30 sudo -u postgres psql -c "\\du" 2>/dev/null | grep parley || echo "(could not list roles — continuing)"

# Create database schema (if migrations exist)
echo "=== Checking for database migrations ==="
if [ -d "/parley" ] && [ -d "/parley/migrations" ]; then
    echo "Migrations directory found - migrations should be run via API"
fi

echo "=== Installing and configuring PgBouncer ==="
apt-get install -y pgbouncer

# D5: bind pgbouncer only to the internal LAN interface (and loopback),
# not to 0.0.0.0. We resolve the LAN IP at runtime by picking the first
# non-loopback IPv4 — this works on DO (private VPC) and on Proxmox LXC
# (10.10.10.0/24). If detection fails, we fail closed to 127.0.0.1 so the
# DB only ever accepts local traffic rather than accidentally listening
# on the public interface. F2 (PVE firewall) already isolates this port
# from other tenants; binding narrowly is defense-in-depth so a future
# firewall-rule regression doesn't silently re-expose the DB.
PGB_LAN_IP=$(ip -4 -o addr show scope global | awk '{print $4}' | cut -d/ -f1 | head -n1)
if [ -z "$PGB_LAN_IP" ]; then
    echo "WARN: could not resolve LAN IP for pgbouncer listen_addr; falling back to 127.0.0.1"
    PGB_LISTEN="127.0.0.1"
else
    echo "Binding pgbouncer listen_addr to 127.0.0.1 and LAN IP $PGB_LAN_IP"
    PGB_LISTEN="127.0.0.1,$PGB_LAN_IP"
fi

# PgBouncer configuration — uses auth_query pattern so only pgbouncer_auth's
# password lives in userlist.txt. Per-client hashes are fetched on demand
# from user_lookup() in the parley DB.
cat > /etc/pgbouncer/pgbouncer.ini << EOF
[databases]
parley = host=localhost port=5432 dbname=parley

[pgbouncer]
listen_addr = $PGB_LISTEN
listen_port = 6432
# auth_type = md5 requires PostgreSQL pg_hba.conf to also use md5.
# Ubuntu 22.04+ PostgreSQL defaults to scram-sha-256. We use scram-sha-256
# here to match. If your PostgreSQL version/config uses md5, change both.
auth_type = scram-sha-256
auth_file = /etc/pgbouncer/userlist.txt
# auth_query pattern (D5 follow-up): pgbouncer connects as pgbouncer_auth to
# the target database and calls user_lookup() for each client login. Only
# pgbouncer_auth's credentials are stored on disk; individual user SCRAM
# hashes are looked up per-connection and never written to userlist.txt.
# The function is SECURITY DEFINER and EXECUTE is granted to pgbouncer_auth
# only, so compromising pgbouncer_auth yields at most the ability to call
# user_lookup() — not to read pg_authid directly.
auth_user = pgbouncer_auth
auth_query = SELECT usename, passwd FROM user_lookup(\$1)
# Session pooling mode: each client connection gets its own dedicated server
# connection. This prevents prepared statement contamination across nodes —
# lib/pq uses extended query protocol (prepared statements) which causes
# "bind message supplies N parameters but prepared statement requires M" errors
# in transaction mode when connections are reused across different clients.
# 2 API nodes × 25 Go pool connections = 50 server connections; well under
# PostgreSQL's max_connections.
pool_mode = session
# lib/pq sends extra_float_digits as a startup parameter; PgBouncer rejects
# unknown startup params unless explicitly ignored.
ignore_startup_parameters = extra_float_digits
max_client_conn = 1000
default_pool_size = 25
reserve_pool_size = 5
reserve_pool_timeout = 3
server_idle_timeout = 600
client_idle_timeout = 0
log_connections = 0
log_disconnections = 0
EOF

# PgBouncer auth file — only the pgbouncer_auth lookup role's credentials.
# No application user hashes are stored here anymore (auth_query fetches them
# per-connection). pgbouncer_auth is low-privilege: LOGIN + EXECUTE on
# user_lookup(text) in the parley DB, nothing else.
echo "\"pgbouncer_auth\" \"$PGB_AUTH_PW\"" > /etc/pgbouncer/userlist.txt
chmod 640 /etc/pgbouncer/userlist.txt
chown postgres:postgres /etc/pgbouncer/userlist.txt

systemctl enable pgbouncer
systemctl restart pgbouncer
echo "PgBouncer listening on port 6432"

# Daily database backup to /var/backups/parley
# Retention policy: pg_dump runs at 03:00 UTC daily; backups older than 7 days
# are automatically deleted. Dumps use custom format (-Fc) for selective restore.
# NOTE on offsite sync: if you add a cron/systemd unit that pushes these dumps
# to MinIO, the target MUST be the private `parley-backups` bucket (see
# terraform/userdata-minio.sh). Never target `parley/backups/*` — the `parley`
# bucket has a public-read policy on the CDN prefixes and any accidental listing
# misconfig would expose them. Historical CT-101 cron wrote to parley/backups/
# and triggered audit finding D1.
mkdir -p /var/backups/parley
cat > /etc/cron.d/parley-backup <<'CRON'
0 3 * * * postgres pg_dump -Fc parley > /var/backups/parley/parley-$(date +\%Y\%m\%d).dump && find /var/backups/parley -name "*.dump" -mtime +7 -delete
CRON
chmod 644 /etc/cron.d/parley-backup

echo "=== Database setup complete ==="
echo "Database can be reached at: $DB_PRIVATE_IP:5432"
echo "PgBouncer pooler available at: $DB_PRIVATE_IP:6432"
echo "Database: parley"
echo "User: parley"
