terraform {
  required_version = ">= 1.0"
  required_providers {
    digitalocean = {
      source  = "digitalocean/digitalocean"
      version = "~> 2.0"
    }
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 4.0"
    }
  }

  # Remote state in DO Spaces (S3-compatible).
  # Credentials passed via -backend-config in CI or via AWS_* env vars locally.
  # First-time migration from local state: terraform init -migrate-state
  backend "s3" {
    endpoint                    = "https://nyc3.digitaloceanspaces.com"
    bucket                      = "parley-prod"
    key                         = "terraform-state/terraform.tfstate"
    region                      = "us-east-1" # required field; ignored by DO Spaces
    skip_credentials_validation = true
    skip_metadata_api_check     = true
    skip_region_validation      = true
    force_path_style            = true
  }
}

provider "digitalocean" {
  token = var.do_token
}

# AWS provider is implicitly loaded by the S3 backend (DO Spaces).
# DO Spaces keys are not real AWS credentials, so skip all AWS validation.
provider "aws" {
  region                      = "us-east-1"
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = true
}

# Get the existing project if it exists, or create a new one
resource "digitalocean_project" "parley_project" {
  name        = "parley-infrastructure"
  description = "Parley Discord clone infrastructure"
  purpose     = "Web Application"
  environment = "Production"
}

# SSH key for droplet access
resource "digitalocean_ssh_key" "parley_key" {
  name       = "parley-deploy-key"
  public_key = file(pathexpand(var.ssh_public_key))
}

# VPC for private networking
resource "digitalocean_vpc" "parley_vpc" {
  name   = "parley-vpc"
  region = var.region
}

# Database droplet
resource "digitalocean_droplet" "parley_db" {
  image    = "ubuntu-24-04-x64"
  name     = "parley-db"
  size     = var.db_droplet_size
  region   = var.region
  vpc_uuid = digitalocean_vpc.parley_vpc.id
  ssh_keys = [digitalocean_ssh_key.parley_key.fingerprint]

  user_data = templatefile("${path.module}/userdata-db.sh", {
    db_password    = var.db_password
    redis_password = var.redis_password
  })

  tags = ["parley", "database"]

  connection {
    type        = "ssh"
    user        = "root"
    private_key = var.ssh_private_key
    host        = self.ipv4_address
  }
}

# API droplets
resource "digitalocean_droplet" "parley_api" {
  count    = var.api_count
  image    = "ubuntu-24-04-x64"
  name     = "parley-api-${count.index + 1}"
  size     = var.api_droplet_size
  region   = var.region
  vpc_uuid = digitalocean_vpc.parley_vpc.id
  ssh_keys = [digitalocean_ssh_key.parley_key.fingerprint]

  user_data = templatefile("${path.module}/userdata-api.sh", {
    DB_HOST                  = digitalocean_droplet.parley_db.ipv4_address_private
    DB_PORT                  = "6432"
    DB_NAME                  = "parley"
    DB_USER                  = "parley"
    DB_PASSWORD              = var.db_password
    JWT_SECRET               = var.jwt_secret
    PORT                     = "8080"
    REPO_URL                 = var.repo_url
    REDIS_HOST               = digitalocean_droplet.parley_db.ipv4_address_private
    SPACES_ACCESS_KEY        = var.spaces_access_key
    SPACES_SECRET_KEY        = var.spaces_secret_key
    SPACES_BUCKET            = var.spaces_bucket
    SPACES_REGION            = var.region
    SPACES_ENDPOINT          = var.spaces_endpoint
    SPACES_CDN_URL           = var.spaces_cdn_url
    BREVO_API_KEY            = var.brevo_api_key
    BREVO_FROM_EMAIL         = var.brevo_from_email
    SITE_URL                 = var.site_url
    ADMIN_IMPERSONATE_SECRET = var.admin_impersonate_secret
    LIVEKIT_API_KEY          = var.livekit_api_key
    LIVEKIT_API_SECRET       = var.livekit_api_secret
    LIVEKIT_URL              = var.livekit_url
    GIPHY_API_KEY            = var.giphy_api_key
    OLLAMA_API_URL           = var.ollama_api_url
    OLLAMA_API_KEY           = var.ollama_api_key
    OLLAMA_MODEL             = var.ollama_model
    BOT_KEY_SECRET           = var.bot_key_secret
    REDIS_PASSWORD           = var.redis_password
  })

  tags = ["parley", "api"]

  connection {
    type        = "ssh"
    user        = "root"
    private_key = var.ssh_private_key
    host        = self.ipv4_address
  }

  depends_on = [digitalocean_droplet.parley_db]
}

# Load Balancer
resource "digitalocean_loadbalancer" "parley_lb" {
  name   = "parley-lb"
  region = var.region

  # Keep WebSocket connections alive — default DO LB idle timeout is ~60s
  # which silently kills long-lived WS connections.
  http_idle_timeout_seconds = 1800

  # Sticky sessions so WS and HTTP requests from the same client land on the
  # same node, avoiding cross-node Redis pub/sub round-trips for message echoes.
  sticky_sessions {
    type               = "cookies"
    cookie_name        = "DO-LB"
    cookie_ttl_seconds = 300
  }

  # Forwarding rules
  forwarding_rule {
    entry_port      = 80
    entry_protocol  = "http"

    target_port     = 80
    target_protocol = "http"
  }


  # Health check
  healthcheck {
    protocol               = "http"
    port                   = 80
    path                   = "/health"
    check_interval_seconds = 10
    response_timeout_seconds = 5
    unhealthy_threshold    = 3
    healthy_threshold      = 3
  }

  # Tag-based targeting so the LB survives API droplet replacements
  droplet_tag = "api"

  vpc_uuid = digitalocean_vpc.parley_vpc.id
}

# Admin panel droplet
resource "digitalocean_droplet" "parley_admin" {
  name   = "parley-admin"
  size   = "s-1vcpu-1gb"
  image  = "ubuntu-24-04-x64"
  region = var.region

  ssh_keys = [digitalocean_ssh_key.parley_key.id]
  vpc_uuid = digitalocean_vpc.parley_vpc.id
  tags     = ["parley", "admin"]

  user_data = templatefile("${path.module}/userdata-admin.sh", {
    REPO_URL                 = var.repo_url
    DB_HOST                  = digitalocean_droplet.parley_db.ipv4_address_private
    DB_PASSWORD              = var.db_password
    REDIS_HOST               = digitalocean_droplet.parley_db.ipv4_address_private
    ADMIN_JWT_SECRET         = var.admin_jwt_secret
    PARLEY_JWT_SECRET        = var.jwt_secret
    ADMIN_IMPERSONATE_SECRET = var.admin_impersonate_secret
    ADMIN_PORT               = "8080"
  })

  depends_on = [digitalocean_droplet.parley_db]
}

output "admin_droplet_ip" {
  value = digitalocean_droplet.parley_admin.ipv4_address
}

# Note: DNS records not managed by Terraform - configure manually at your registrar
# Point your domain to the load balancer IP after creation
# LiveKit is handled via LiveKit Cloud — no self-hosted vc droplet needed

# Spaces bucket (parley-prod) is managed manually in the DO console
# with CDN already configured — not managed by Terraform.

# ────────────────────────────────────────────────────────────────────────────
# Cloud Firewalls — enforced at the hypervisor, independent of OS firewall
# ────────────────────────────────────────────────────────────────────────────

# Cloudflare published IPv4 ranges (https://www.cloudflare.com/ips-v4)
locals {
  cloudflare_ipv4 = [
    "173.245.48.0/20",
    "103.21.244.0/22",
    "103.22.200.0/22",
    "103.31.4.0/22",
    "141.101.64.0/18",
    "108.162.192.0/18",
    "190.93.240.0/20",
    "188.114.96.0/20",
    "197.234.240.0/22",
    "198.41.128.0/17",
    "162.158.0.0/15",
    "104.16.0.0/13",
    "104.24.0.0/14",
    "172.64.0.0/13",
    "131.0.72.0/22",
  ]
}

# API nodes: port 80 from Cloudflare + LB only; SSH from admin IP only
resource "digitalocean_firewall" "parley_api" {
  name = "parley-api-fw"
  tags = ["api"]

  # HTTP — Cloudflare IPs and the DO load balancer health-checker
  # Using load_balancer_uids ensures health-check probes are allowed regardless of
  # which internal IP the LB uses for probes (not necessarily the public IP).
  inbound_rule {
    protocol                  = "tcp"
    port_range                = "80"
    source_addresses          = local.cloudflare_ipv4
    source_load_balancer_uids = [digitalocean_loadbalancer.parley_lb.id]
  }

  # SSH — admin IP only
  inbound_rule {
    protocol         = "tcp"
    port_range       = "22"
    source_addresses = [var.admin_allowed_ip]
  }

  outbound_rule {
    protocol              = "tcp"
    port_range            = "1-65535"
    destination_addresses = ["0.0.0.0/0", "::/0"]
  }
  outbound_rule {
    protocol              = "udp"
    port_range            = "1-65535"
    destination_addresses = ["0.0.0.0/0", "::/0"]
  }
}

# DB server: DB/Redis ports from VPC only; SSH from admin IP only
resource "digitalocean_firewall" "parley_db" {
  name        = "parley-db-fw"
  droplet_ids = [digitalocean_droplet.parley_db.id]

  # SSH — admin IP only
  inbound_rule {
    protocol         = "tcp"
    port_range       = "22"
    source_addresses = [var.admin_allowed_ip]
  }

  # PostgreSQL — VPC only (10.0.0.0/8 covers all DO VPC ranges)
  inbound_rule {
    protocol         = "tcp"
    port_range       = "5432"
    source_addresses = ["10.0.0.0/8"]
  }

  # PgBouncer — VPC only
  inbound_rule {
    protocol         = "tcp"
    port_range       = "6432"
    source_addresses = ["10.0.0.0/8"]
  }

  # Redis — VPC only
  inbound_rule {
    protocol         = "tcp"
    port_range       = "6379"
    source_addresses = ["10.0.0.0/8"]
  }

  outbound_rule {
    protocol              = "tcp"
    port_range            = "1-65535"
    destination_addresses = ["0.0.0.0/0", "::/0"]
  }
  outbound_rule {
    protocol              = "udp"
    port_range            = "1-65535"
    destination_addresses = ["0.0.0.0/0", "::/0"]
  }
}

# Admin panel: port 80 and SSH from admin IP only
resource "digitalocean_firewall" "parley_admin" {
  name        = "parley-admin-fw"
  droplet_ids = [digitalocean_droplet.parley_admin.id]

  inbound_rule {
    protocol         = "tcp"
    port_range       = "80"
    source_addresses = [var.admin_allowed_ip]
  }

  inbound_rule {
    protocol         = "tcp"
    port_range       = "22"
    source_addresses = [var.admin_allowed_ip]
  }

  outbound_rule {
    protocol              = "tcp"
    port_range            = "1-65535"
    destination_addresses = ["0.0.0.0/0", "::/0"]
  }
  outbound_rule {
    protocol              = "udp"
    port_range            = "1-65535"
    destination_addresses = ["0.0.0.0/0", "::/0"]
  }
}

# ────────────────────────────────────────────────────────────────────────────
# Cloudflare DNS — var.domain_name → load balancer IP
# Credentials: CLOUDFLARE_API_TOKEN env var (set in CI from CLOUDFLARE_API_KEY secret)
# ────────────────────────────────────────────────────────────────────────────

provider "cloudflare" {
  # Reads CLOUDFLARE_API_TOKEN from the environment automatically.
}

data "cloudflare_zone" "parley" {
  name = var.cf_zone
}

resource "cloudflare_record" "parley_a" {
  zone_id = data.cloudflare_zone.parley.id
  # Subdomain label: "parley" from "parley.x86-64.com" / "@" if domain == zone
  name    = var.domain_name == var.cf_zone ? "@" : trimsuffix(trimsuffix(var.domain_name, var.cf_zone), ".")
  content = digitalocean_loadbalancer.parley_lb.ip
  type    = "A"
  proxied = true
  ttl     = 1 # 1 = automatic (required when proxied = true)
}

output "dns_record" {
  description = "Cloudflare DNS record for the configured domain"
  value       = "${cloudflare_record.parley_a.name}.${data.cloudflare_zone.parley.name} → ${digitalocean_loadbalancer.parley_lb.ip}"
}