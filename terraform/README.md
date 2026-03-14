# Parley Infrastructure - Terraform

Terraform configuration for deploying Parley (Discord clone) to DigitalOcean.

## Architecture

- **3 API Droplets**: Go backend service running behind Nginx reverse proxy
- **1 Database Droplet**: PostgreSQL for persistent storage
- **1 Load Balancer**: Distributes traffic across API droplets
- **DNS Records**: A records for each API droplet and the load balancer

## Prerequisites

1. **DigitalOcean Account**: You need a DigitalOcean account
2. **API Token**: Create a Personal Access Token with read/write scope
3. **SSH Key**: Configure SSH key access for droplet connections
4. **Domain**: Domain name configured in DigitalOcean (parley.x86-64.com)

## Quick Start

### 1. Configure Variables

Copy the example environment file and fill in your values:

```bash
cp .env.example .env
```

Edit `.env` with your specific values, then create a `terraform.tfvars` file:

```hcl
do_token        = "your_digitalocean_token"
db_password     = "your_secure_db_password"
jwt_secret      = "your_jwt_secret_run_openssl_rand_base64_32"
repo_url        = "https://github.com/yourusername/parley.git"
```

### 2. Initialize Terraform

```bash
terraform init
```

### 3. Plan and Apply

Preview the changes:

```bash
terraform plan
```

Apply the infrastructure:

```bash
terraform apply
```

## Usage

### Connecting to Droplets

SSH into API droplets:

```bash
# API Droplet 1
ssh root@<api-droplet-ip>

# API Droplet 2
ssh root@<api-droplet-ip>
```

SSH into database:

```bash
ssh root@<db-droplet-ip>
```

### Viewing Logs

API logs:

```bash
# All API droplets
for i in {1..3}; do
  echo "=== API $i ==="
  ssh root@api$i "journalctl -u parley-api -n 20 --no-pager"
done
```

Database logs:

```bash
ssh root@<db-ip> "tail -f /var/log/postgresql/postgresql-*.log"
```

### Deploying Updates

#### Option 1: Pull latest code on each droplet

```bash
# On each API droplet
ssh root@<api-ip>
cd /parley
git pull
go build -o /usr/local/bin/parley-api ./cmd/api
systemctl restart parley-api
```

#### Option 2: Use Terraform to rebuild (recommended for major changes)

```bash
terraform apply -var="api_count=3" -refresh=true
```

#### Option 3: Use the API's built-in migration (if migrations exist)

```bash
curl -X POST http://<load-balancer-ip>/migrate
```

### Health Checks

The load balancer performs health checks on `/health` endpoint. Verify:

```bash
curl http://<load-balancer-ip>/health
```

Should return a 200 OK response.

### Database Management

Connect to PostgreSQL:

```bash
ssh root@<db-ip>
sudo -u postgres psql -d parley
```

Common commands:

```sql
-- List tables
\dt

-- Create user (if needed)
CREATE USER newuser WITH PASSWORD 'password';
GRANT ALL PRIVILEGES ON DATABASE parley TO newuser;

-- Run migrations manually
-- (Only if API doesn't auto-migrate)
```

### Adding SSL/HTTPS

The load balancer is configured for HTTP. To add HTTPS:

```bash
# On each API droplet
ssh root@<api-ip>

# Install certbot
apt-get install certbot python3-certbot-nginx

# Generate certificate
certbot --nginx -d parley.x86-64.com -d www.parley.x86-64.com

# Follow the prompts
```

Note: For production, consider using DigitalOcean's managed load balancer with SSL termination instead.

## Troubleshooting

### API not responding

1. Check if service is running:
   ```bash
   systemctl status parley-api
   ```

2. Check logs:
   ```bash
   journalctl -u parley-api -f
   ```

3. Check nginx:
   ```bash
   nginx -t
   systemctl status nginx
   ```

### Database connection issues

1. Verify PostgreSQL is running:
   ```bash
   systemctl status postgresql
   ```

2. Check if ports are open:
   ```bash
   ufw status
   ```

3. Test connection from API droplet:
   ```bash
   psql -h <db-ip> -U parley -d parley
   ```

### Load balancer not routing

1. Check load balancer health:
   - Go to DigitalOcean Dashboard > Networking > Load Balancers
   - Check "Health Check" status for each droplet

2. Verify droplet tags match:
   ```bash
   # Droplets should have tag "parley"
   doctl compute droplet list --format "ID,Name,Tags"
   ```

## Cleaning Up

To destroy all resources:

```bash
terraform destroy
```

**Warning**: This will delete all data in the database. Make sure to backup first if needed.

## Files

- `main.tf` - Main Terraform configuration
- `variables.tf` - Variable definitions
- `outputs.tf` - Output values
- `userdata-api.sh` - Cloud-init script for API droplets
- `userdata-db.sh` - Cloud-init script for database
- `.env.example` - Environment template

## Security Considerations

1. **Secrets**: Never commit `terraform.tfvars` or `.env` files to version control
2. **Firewall**: Only port 22 (SSH), 80 (HTTP), and 443 (HTTPS) are open externally
3. **Database**: Only accessible from within the VPC
4. **JWT Secret**: Use a strong, random secret (32+ bytes)

## Cost Estimation

- 3x s-2vcpu-2gb API droplets: ~$30/month
- 1x s-2vcpu-4gb DB droplet: ~$20/month
- 1x Load Balancer: ~$12/month
- Total: ~$62/month

(Plus bandwidth and storage costs)