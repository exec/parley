output "db_ip" {
  value       = var.db_ip
  description = "Database VM IP"
}

output "api_ips" {
  value       = [for i in range(var.api_count) : cidrhost(format("%s/%d", var.api_ip_base, var.subnet_mask), i)]
  description = "API VM IPs"
}

output "admin_ip" {
  value       = var.admin_ip
  description = "Admin VM IP"
}
