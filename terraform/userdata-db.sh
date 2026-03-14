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

# Update pg_hba.conf to allow connections from the VPC
PG_HBA=$(find /etc/postgresql -name pg_hba.conf | head -1)
if [ -n "$PG_HBA" ]; then
    # Add entries for API droplets (we'll use 10.0.0.0/16 as a common VPC range)
    # DigitalOcean VPC typically uses 10.x.x.x ranges
    echo "# Parley API connections - allow all in VPC" >> "$PG_HBA"
    echo "host    all             all             10.0.0.0/8           md5" >> "$PG_HBA"
    echo "host    all             all             172.16.0.0/12        md5" >> "$PG_HBA"
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
fi

# Configure firewall
echo "=== Configuring firewall ==="
ufw allow 22/tcp    # SSH
ufw allow 5432/tcp  # PostgreSQL
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

echo "=== Database setup complete ==="
echo "Database can be reached at: $DB_PRIVATE_IP:5432"
echo "Database: parley"
echo "User: parley"