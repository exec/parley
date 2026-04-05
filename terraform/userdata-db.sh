#!/bin/bash
set -e

# Cloud-init script for Parley PostgreSQL droplet
# This script sets up PostgreSQL and configures it for the API

echo "=== Starting Parley Database setup ==="

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

# Create parley user and database
echo "=== Creating Parley database and user ==="
sudo -u postgres psql <<EOF
-- Create the parley user
CREATE USER parley WITH PASSWORD '${db_password}';

-- Create the parley database
CREATE DATABASE parley OWNER parley;

-- Grant privileges
GRANT ALL PRIVILEGES ON DATABASE parley TO parley;

-- Connect to the database and set default schema
\c parley

-- Grant schema privileges
GRANT ALL ON SCHEMA public TO parley;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO parley;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO parley;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON FUNCTIONS TO parley;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON PROCEDURES TO parley;
EOF

# Configure PostgreSQL to accept connections from API droplets
echo "=== Configuring PostgreSQL for remote connections ==="

# Get the private IP of this database server
DB_PRIVATE_IP=$(hostname -I | awk '{print $1}')
echo "Database private IP: $DB_PRIVATE_IP"

# Update pg_hba.conf to allow connections from the local subnet
# Derive subnet from this VM's IP — works on both DO (10.x.x.x) and Proxmox (192.168.x.x)
LAN_SUBNET="$${DB_PRIVATE_IP%.*}.0/24"
PG_HBA=$(find /etc/postgresql -name pg_hba.conf | head -1)
if [ -n "$PG_HBA" ]; then
    echo "# Parley API connections - allow LAN subnet only" >> "$PG_HBA"
    echo "host    all             all             $LAN_SUBNET          scram-sha-256" >> "$PG_HBA"
fi

# Update postgresql.conf to listen on all interfaces
PG_CONF=$(find /etc/postgresql -name postgresql.conf | head -1)
if [ -n "$PG_CONF" ]; then
    # Configure PostgreSQL to listen on all addresses
    sed -i "s/^listen_addresses = 'localhost'/listen_addresses = '*'/" "$PG_CONF"

    # Ensure it's not commented
    if grep -q "^#listen_addresses" "$PG_CONF"; then
        sed -i "s/^#listen_addresses = 'localhost'/listen_addresses = '*'/" "$PG_CONF"
    fi

    echo "=== Tuning PostgreSQL max_connections ==="
    sed -i "s/^#*max_connections.*/max_connections = 150/" "$PG_CONF"
    # Shared buffers: 25% of RAM (4GB droplet → 1GB)
    sed -i "s/^#*shared_buffers.*/shared_buffers = 1GB/" "$PG_CONF"
    # Effective cache size: 75% of RAM
    sed -i "s/^#*effective_cache_size.*/effective_cache_size = 3GB/" "$PG_CONF"
    # Work memory for sort operations
    sed -i "s/^#*work_mem.*/work_mem = 4MB/" "$PG_CONF"
    # Huge pages: let Postgres use them if the kernel has them available.
    # With 1GB shared_buffers, huge pages save ~500 TLB entries and reduce
    # kernel memory overhead. "try" falls back gracefully if unavailable.
    sed -i "s/^#*huge_pages.*/huge_pages = try/" "$PG_CONF"
fi

# Enable huge pages in the kernel (2MB pages; 512 pages covers 1GB shared_buffers).
# Guard with || true: if the kernel lacks huge pages support (e.g. cloud kernels),
# sysctl -w returns non-zero and would abort this set -e script unnecessarily.
# PostgreSQL's huge_pages=try already falls back gracefully.
echo "vm.nr_hugepages=512" >> /etc/sysctl.conf
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

# Configure firewall — DB/Redis only accessible from LAN subnet, not public internet
echo "=== Configuring firewall (LAN-only DB ports) ==="
ufw default deny incoming
ufw default allow outgoing
ufw allow 22/tcp                                                   # SSH
ufw allow from "$LAN_SUBNET" to any port 5432 proto tcp           # PostgreSQL — LAN only
ufw allow from "$LAN_SUBNET" to any port 6432 proto tcp           # PgBouncer — LAN only
ufw allow from "$LAN_SUBNET" to any port 6379 proto tcp           # Redis — LAN only
ufw --force enable

# Restart PostgreSQL with new configuration
echo "=== Restarting PostgreSQL ==="
systemctl restart postgresql

# Verify PostgreSQL is running
echo "=== Verifying PostgreSQL status ==="
systemctl status postgresql --no-pager || true

# Check connections
echo "=== Testing database connection ==="
sudo -u postgres psql -c "SELECT version();"
sudo -u postgres psql -c "\\du" | grep parley

# Create database schema (if migrations exist)
echo "=== Checking for database migrations ==="
if [ -d "/parley" ] && [ -d "/parley/migrations" ]; then
    echo "Migrations directory found - migrations should be run via API"
fi

echo "=== Installing and configuring PgBouncer ==="
apt-get install -y pgbouncer

# PgBouncer configuration
cat > /etc/pgbouncer/pgbouncer.ini << 'EOF'
[databases]
parley = host=localhost port=5432 dbname=parley

[pgbouncer]
listen_addr = 0.0.0.0
listen_port = 6432
# auth_type = md5 requires PostgreSQL pg_hba.conf to also use md5.
# Ubuntu 22.04+ PostgreSQL defaults to scram-sha-256. We use scram-sha-256
# here to match. If your PostgreSQL version/config uses md5, change both.
auth_type = scram-sha-256
auth_file = /etc/pgbouncer/userlist.txt
# Session pooling mode: each client connection gets its own dedicated server
# connection. This prevents prepared statement contamination across nodes —
# lib/pq uses extended query protocol (prepared statements) which causes
# "bind message supplies N parameters but prepared statement requires M" errors
# in transaction mode when connections are reused across different clients.
# 3 API nodes × 25 Go pool connections = 75 server connections; well under
# PostgreSQL's max_connections = 150.
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

# PgBouncer auth file. With auth_type=scram-sha-256 and PgBouncer 1.17+ (Ubuntu 22.04),
# plaintext passwords in userlist.txt are supported — PgBouncer computes the SCRAM
# exchange internally. No separate SCRAM verifier extraction is needed at provisioning.
# To verify post-boot: psql -h 127.0.0.1 -p 6432 -U parley parley
echo "\"parley\" \"${db_password}\"" > /etc/pgbouncer/userlist.txt
chmod 640 /etc/pgbouncer/userlist.txt
chown postgres:postgres /etc/pgbouncer/userlist.txt

systemctl enable pgbouncer
systemctl restart pgbouncer
echo "PgBouncer listening on port 6432"

# Daily database backup to /var/backups/parley
# Retention policy: pg_dump runs at 03:00 UTC daily; backups older than 7 days
# are automatically deleted. Dumps use custom format (-Fc) for selective restore.
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
