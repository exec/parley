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
  default     = ""
}

variable "jwt_secret" {
  description = "JWT secret for authentication"
  type        = string
  sensitive   = true
  default     = ""
}

variable "repo_url" {
  description = "Git repository URL for Parley"
  type        = string
  default     = "https://github.com/yourusername/parley.git"
}