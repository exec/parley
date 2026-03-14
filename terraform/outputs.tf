output "load_balancer_ip" {
  description = "Public IP address of the load balancer"
  value       = digitalocean_loadbalancer.parley_lb.ip
}

output "load_balancer_hostname" {
  description = "Hostname of the load balancer"
  value       = digitalocean_loadbalancer.parley_lb.ip
}

output "api_droplet_ips" {
  description = "IP addresses of API droplets"
  value       = [for droplet in digitalocean_droplet.parley_api : droplet.ipv4_address]
}

output "database_droplet_ip" {
  description = "Private IP address of the database droplet"
  value       = digitalocean_droplet.parley_db.ipv4_address
}

output "database_private_ip" {
  description = "Private IP address of the database droplet (for VPC)"
  value       = digitalocean_droplet.parley_db.ipv4_address_private
}

output "domain" {
  description = "Configured domain name"
  value       = var.domain_name
}

output "vpc_id" {
  description = "VPC ID for the infrastructure"
  value       = digitalocean_vpc.parley_vpc.id
}

output "project_id" {
  description = "DigitalOcean Project ID"
  value       = digitalocean_project.parley_project.id
}

output "spaces_bucket_domain" {
  description = "DigitalOcean Spaces bucket domain"
  value       = digitalocean_spaces_bucket.parley_uploads.bucket_domain_name
}