variable "do_token" {
  description = "DigitalOcean API token"
  type        = string
  sensitive   = true
}

variable "region" {
  description = "DigitalOcean region"
  type        = string
  default     = "nyc3"
}

variable "api_droplet_size" {
  description = "Droplet size for API servers"
  type        = string
  default     = "s-2vcpu-2gb"
}

variable "db_droplet_size" {
  description = "Droplet size for database server"
  type        = string
  default     = "s-2vcpu-4gb"
}

variable "domain_name" {
  description = "Domain name for Parley"
  type        = string
  default     = "parley.x86-64.com"
}

variable "api_count" {
  description = "Number of API droplets"
  type        = number
  default     = 3
}

variable "ssh_private_key" {
  description = "SSH private key path for droplet access"
  type        = string
  default     = "~/.ssh/id_ed25519"
}

variable "ssh_public_key" {
  description = "SSH public key to install on droplets"
  type        = string
  default     = "~/.ssh/id_ed25519.pub"
}

variable "db_password" {
  description = "PostgreSQL parley user password"
  type        = string
  sensitive   = true
  default     = "parley_secure_pwd_2026"
}

variable "jwt_secret" {
  description = "JWT secret for authentication"
  type        = string
  sensitive   = true
  default     = "jwt_super_secret_key_do_not_share_2026"
}

variable "repo_url" {
  description = "Git repository URL for Parley"
  type        = string
  default     = "https://github.com/yourusername/parley.git"
}

variable "spaces_access_key" {
  description = "DigitalOcean Spaces access key"
  type        = string
  sensitive   = true
}

variable "spaces_secret_key" {
  description = "DigitalOcean Spaces secret key"
  type        = string
  sensitive   = true
}

variable "spaces_bucket" {
  description = "DigitalOcean Spaces bucket name"
  type        = string
  default     = "parley-prod"
}

variable "spaces_endpoint" {
  description = "DigitalOcean Spaces S3-compatible endpoint URL"
  type        = string
  default     = "https://nyc3.digitaloceanspaces.com"
}

variable "spaces_cdn_url" {
  description = "Base URL for serving uploaded files (CDN or direct)"
  type        = string
}