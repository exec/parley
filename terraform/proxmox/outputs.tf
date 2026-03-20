output "entry_ip" {
  value       = var.api_count > 1 ? var.lb_ip : var.api_ip_base
  description = "Entry point IP — nginx LB if api_count > 1, first API node otherwise"
}

output "api_ips" {
  value       = [for i in range(var.api_count) : "${join(".", slice(split(".", var.api_ip_base), 0, 3))}.${tonumber(split(".", var.api_ip_base)[3]) + i}"]
  description = "API VM IPs"
}

output "db_ip" {
  value       = var.db_ip
  description = "Database VM IP"
}

output "admin_ip" {
  value       = var.admin_ip
  description = "Admin VM IP"
}

output "minio_ip" {
  value       = var.minio_ip
  description = "MinIO VM IP — S3 API on :9000, web console on :9001"
}
