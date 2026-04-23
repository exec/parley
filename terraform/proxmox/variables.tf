variable "proxmox_endpoint" {
  description = "Proxmox API root endpoint (e.g. https://10.0.0.246:8006/)"
  type        = string
}

variable "proxmox_api_token_id" {
  description = "Proxmox API token ID (e.g. root@pam!terraform)"
  type        = string
}

variable "proxmox_api_token_secret" {
  description = "Proxmox API token secret"
  type        = string
  sensitive   = true
}

variable "proxmox_node" {
  description = "Proxmox node name to deploy containers on"
  type        = string
  default     = "pve"
}

variable "proxmox_storage" {
  description = "Proxmox storage pool for container rootfs"
  type        = string
  default     = "local-lvm"
}

variable "proxmox_bridge" {
  description = "Network bridge for container NICs"
  type        = string
  default     = "vmbr1"
}

variable "ct_template" {
  description = "LXC CT template in storage:vztmpl/<name> form"
  type        = string
  default     = "local:vztmpl/debian-13-standard_13.1-2_amd64.tar.zst"
}

# ---- Proxmox host SSH (used by provisioners to pct push/exec into containers) ----

variable "proxmox_host_address" {
  description = "IP or hostname of the Proxmox host reachable by SSH from the machine running terraform"
  type        = string
}

variable "proxmox_host_user" {
  description = "SSH user on the Proxmox host (must have passwordless sudo)"
  type        = string
  default     = "root"
}

variable "proxmox_host_ssh_key" {
  description = "Path to the SSH private key for proxmox_host_user"
  type        = string
  default     = "~/.ssh/id_ed25519"
}

# ---- Container sizing ----

variable "api_cores" {
  description = "vCPUs for API containers"
  type        = number
  default     = 2
}

variable "api_memory" {
  description = "RAM in MB for API containers"
  type        = number
  default     = 2048
}

variable "api_disk_size" {
  description = "Rootfs size for API containers (e.g. 20G)"
  type        = string
  default     = "20G"
}

variable "db_cores" {
  description = "vCPUs for DB container"
  type        = number
  default     = 2
}

variable "db_memory" {
  description = "RAM in MB for DB container"
  type        = number
  default     = 4096
}

variable "db_disk_size" {
  description = "Rootfs size for DB container (e.g. 40G)"
  type        = string
  default     = "40G"
}

variable "admin_cores" {
  description = "vCPUs for admin container"
  type        = number
  default     = 1
}

variable "admin_memory" {
  description = "RAM in MB for admin container"
  type        = number
  default     = 1024
}

variable "admin_disk_size" {
  description = "Rootfs size for admin container"
  type        = string
  default     = "10G"
}

variable "lb_cores" {
  description = "vCPUs for nginx LB container"
  type        = number
  default     = 1
}

variable "lb_memory" {
  description = "RAM in MB for nginx LB container"
  type        = number
  default     = 512
}

variable "lb_disk_size" {
  description = "Rootfs size for nginx LB container"
  type        = string
  default     = "8G"
}

variable "minio_cores" {
  description = "vCPUs for MinIO container"
  type        = number
  default     = 1
}

variable "minio_memory" {
  description = "RAM in MB for MinIO container"
  type        = number
  default     = 1024
}

variable "minio_disk_size" {
  description = "Rootfs size for MinIO container"
  type        = string
  default     = "20G"
}

variable "api_count" {
  description = "Number of API containers (max 9 with default IPs)"
  type        = number
  default     = 1

  validation {
    condition     = var.api_count >= 1 && var.api_count <= 9
    error_message = "api_count must be between 1 and 9."
  }
}

# ---- Networking (vmbr1 internal subnet by default) ----

variable "api_ip_base" {
  description = "First octet-group + first API IP (e.g. 10.10.10.11 — increments by count)"
  type        = string
  default     = "10.10.10.11"
}

variable "db_ip" {
  description = "Static IP for DB container"
  type        = string
  default     = "10.10.10.10"
}

variable "admin_ip" {
  description = "Static IP for admin container"
  type        = string
  default     = "10.10.10.15"
}

variable "lb_ip" {
  description = "Static IP for nginx LB container (only when api_count > 1)"
  type        = string
  default     = "10.10.10.20"
}

variable "minio_ip" {
  description = "Static IP for MinIO container"
  type        = string
  default     = "10.10.10.21"
}

variable "gateway" {
  description = "Default gateway (host IP on the bridge)"
  type        = string
  default     = "10.10.10.1"
}

variable "subnet_mask" {
  description = "Subnet mask bits"
  type        = number
  default     = 24
}

variable "dns_server" {
  description = "DNS server for containers"
  type        = string
  default     = "1.1.1.1"
}

# ---- SSH key injected into containers ----

variable "ssh_public_key" {
  description = "SSH public key to inject into containers (path)"
  type        = string
  default     = "~/.ssh/id_ed25519.pub"
}

# ---- App secrets ----

variable "db_password" {
  description = "PostgreSQL parley user password"
  type        = string
  sensitive   = true
}

variable "jwt_secret" {
  description = "JWT secret for authentication"
  type        = string
  sensitive   = true
}

variable "repo_url" {
  description = "Git repository URL"
  type        = string
  default     = "https://github.com/exec/parley.git"
}

variable "minio_access_key" {
  description = "MinIO root access key"
  type        = string
  default     = "parleyminio"
}

variable "minio_secret_key" {
  description = "MinIO root secret key"
  type        = string
  sensitive   = true
}

variable "minio_bucket" {
  description = "MinIO bucket name"
  type        = string
  default     = "parley"
}

variable "brevo_api_key" {
  description = "Brevo API key"
  type        = string
  sensitive   = true
  default     = ""
}

variable "brevo_from_email" {
  description = "From address for verification emails"
  type        = string
  default     = "noreply@parley.local"
}

variable "site_url" {
  description = "Public URL"
  type        = string
  default     = "http://10.10.10.11"
}

variable "admin_jwt_secret" {
  description = "JWT secret for admin panel"
  type        = string
  sensitive   = true
  default     = ""
}

# F-admin-origin-fallback: the admin Go server fails-closed when ADMIN_ORIGIN
# is unset. Must be the exact origin the admin frontend will request from
# (scheme + host + optional port, no trailing slash).
variable "admin_origin" {
  description = "Allowed CORS origin for the admin frontend (e.g. http://10.10.10.11)"
  type        = string
}

variable "admin_impersonate_secret" {
  description = "Shared secret for admin impersonation"
  type        = string
  sensitive   = true
  default     = ""
}

# Signing key for admin-minted impersonation JWTs. MUST differ from jwt_secret
# (see F-admin-jwt-secret / docs/security/runbooks/admin-jwt-secret-separation.md).
variable "impersonation_jwt_secret" {
  description = "JWT signing key for admin impersonation tokens (must differ from jwt_secret)"
  type        = string
  sensitive   = true
}

variable "giphy_api_key" {
  description = "Giphy API key"
  type        = string
  sensitive   = true
  default     = ""
}

variable "livekit_api_key" {
  description = "LiveKit API key"
  type        = string
  sensitive   = true
  default     = ""
}

variable "livekit_api_secret" {
  description = "LiveKit API secret"
  type        = string
  sensitive   = true
  default     = ""
}

variable "livekit_url" {
  description = "LiveKit WSS URL"
  type        = string
  default     = ""
}

variable "ollama_api_url" {
  description = "Ollama API base URL"
  type        = string
  default     = "https://ollama.com"
}

variable "ollama_api_key" {
  description = "Ollama API key"
  type        = string
  sensitive   = true
  default     = ""
}

variable "ollama_model" {
  description = "Ollama model name"
  type        = string
  default     = "devstral-small-2:24b-cloud"
}

variable "bot_key_secret" {
  description = "Secret for deriving the AES-256 bot API key encryption key"
  type        = string
  sensitive   = true
  default     = ""
}

variable "redis_password" {
  description = "Redis password"
  type        = string
  sensitive   = true
  default     = ""
}
