terraform {
  required_version = ">= 1.0"
  required_providers {
    digitalocean = {
      source  = "digitalocean/digitalocean"
      version = "~> 2.0"
    }
  }
}

provider "digitalocean" {
  token = var.do_token
}

# Get the existing project if it exists, or create a new one
resource "digitalocean_project" "parley_project" {
  name        = "parley-infrastructure"
  description = "Parley Discord clone infrastructure"
  purpose     = "Web Application"
  environment = "Production"
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

  user_data = file("${path.module}/userdata-db.sh")

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
  count   = var.api_count
  image   = "ubuntu-24-04-x64"
  name    = "parley-api-${count.index + 1}"
  size    = var.api_droplet_size
  region  = var.region
  vpc_uuid = digitalocean_vpc.parley_vpc.id

  user_data = templatefile("${path.module}/userdata-api.sh", {
    DB_HOST     = digitalocean_droplet.parley_db.ipv4_address
    DB_PORT     = "5432"
    DB_NAME     = "parley"
    DB_USER     = "parley"
    DB_PASSWORD = var.db_password
    JWT_SECRET  = var.jwt_secret
    PORT        = "8080"
    REPO_URL    = var.repo_url
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

  # Forwarding rules
  forwarding_rule {
    entry_port     = 80
    entry_protocol = "http"

    target_port     = 80
    target_protocol = "http"
  }

  forwarding_rule {
    entry_port     = 443
    entry_protocol = "https"

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

  # Droplet IDs to attach
  droplet_ids = [for droplet in digitalocean_droplet.parley_api : droplet.id]

  vpc_uuid = digitalocean_vpc.parley_vpc.id
}

# DNS A records for API droplets
resource "digitalocean_record" "api_records" {
  count  = var.api_count
  type   = "A"
  name   = "api-${count.index + 1}"
  domain = var.domain_name
  value  = digitalocean_droplet.parley_api[count.index].ipv4_address
  ttl    = 300
}

# DNS A record for load balancer (main domain)
resource "digitalocean_record" "lb_record" {
  type   = "A"
  name   = "@"
  domain = var.domain_name
  value  = digitalocean_loadbalancer.parley_lb.ip
  ttl    = 300
}

# CNAME for www
resource "digitalocean_record" "www_record" {
  type   = "CNAME"
  name   = "www"
  domain = var.domain_name
  value  = digitalocean_loadbalancer.parley_lb.ip
  ttl    = 300
}