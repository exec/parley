variable "proxmox_api_url" {
  description = "Proxmox API URL (e.g. https://192.168.1.10:8006/api2/json)"
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
  description = "Proxmox node name to deploy VMs on"
  type        = string
  default     = "pve"
}

variable "proxmox_storage" {
  description = "Proxmox storage pool for VM disks"
  type        = string
  default     = "local-lvm"
}

variable "proxmox_bridge" {
  description = "Network bridge for VM NICs"
  type        = string
  default     = "vmbr0"
}

variable "vm_template" {
  description = "Name of the Ubuntu 24.04 cloud-init template VM"
  type        = string
  default     = "ubuntu-2404-template"
}

# ---- VM sizing (mirrors DO droplet sizes) ----

variable "api_cores" {
  description = "vCPUs for API VMs (DO s-2vcpu-2gb equivalent)"
  type        = number
  default     = 2
}

variable "api_memory" {
  description = "RAM in MB for API VMs"
  type        = number
  default     = 2048
}

variable "api_disk_size" {
  description = "Disk size for API VMs (GB)"
  type        = string
  default     = "20G"
}

variable "db_cores" {
  description = "vCPUs for DB VM (DO s-2vcpu-4gb equivalent)"
  type        = number
  default     = 2
}

variable "db_memory" {
  description = "RAM in MB for DB VM"
  type        = number
  default     = 4096
}

variable "db_disk_size" {
  description = "Disk size for DB VM (GB)"
  type        = string
  default     = "40G"
}

variable "admin_cores" {
  description = "vCPUs for admin VM (DO s-1vcpu-1gb equivalent)"
  type        = number
  default     = 1
}

variable "admin_memory" {
  description = "RAM in MB for admin VM"
  type        = number
  default     = 1024
}

variable "api_count" {
  description = "Number of API VMs"
  type        = number
  default     = 1
}

# ---- Networking ----

variable "api_ip_base" {
  description = "Base IP for API VMs (e.g. 192.168.1.11 — increments by count)"
  type        = string
  default     = "192.168.1.11"
}

variable "db_ip" {
  description = "Static IP for DB VM"
  type        = string
  default     = "192.168.1.10"
}

variable "admin_ip" {
  description = "Static IP for admin VM"
  type        = string
  default     = "192.168.1.15"
}

variable "gateway" {
  description = "Default gateway for VMs"
  type        = string
  default     = "192.168.1.1"
}

variable "subnet_mask" {
  description = "Subnet mask bits"
  type        = number
  default     = 24
}

variable "dns_server" {
  description = "DNS server for VMs"
  type        = string
  default     = "1.1.1.1"
}

# ---- SSH ----

variable "ssh_public_key" {
  description = "SSH public key to inject via cloud-init"
  type        = string
  default     = "~/.ssh/id_ed25519.pub"
}

variable "ssh_private_key" {
  description = "SSH private key path for provisioning"
  type        = string
  default     = "~/.ssh/id_ed25519"
}

# ---- App secrets (same as DO vars) ----

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

variable "spaces_access_key" {
  description = "S3-compatible object storage access key"
  type        = string
  sensitive   = true
  default     = ""
}

variable "spaces_secret_key" {
  description = "S3-compatible object storage secret key"
  type        = string
  sensitive   = true
  default     = ""
}

variable "spaces_bucket" {
  description = "Object storage bucket name"
  type        = string
  default     = "parley-dev"
}

variable "spaces_endpoint" {
  description = "S3-compatible endpoint URL"
  type        = string
  default     = ""
}

variable "spaces_cdn_url" {
  description = "CDN/direct URL for serving uploaded files"
  type        = string
  default     = ""
}

variable "brevo_api_key" {
  description = "Brevo API key for transactional email"
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
  description = "Public URL (used in email links)"
  type        = string
  default     = "http://192.168.1.11"
}

variable "admin_jwt_secret" {
  description = "JWT secret for admin panel"
  type        = string
  sensitive   = true
  default     = ""
}

variable "admin_impersonate_secret" {
  description = "Shared secret for admin impersonation"
  type        = string
  sensitive   = true
  default     = ""
}

variable "giphy_api_key" {
  description = "Giphy API key for GIF search"
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
  default     = "wss://parley-6jjbl5wy.livekit.cloud"
}

variable "ollama_api_url" {
  description = "Ollama API base URL"
  type        = string
  default     = "https://ollama.com"
}

variable "ollama_api_key" {
  description = "Ollama API key for AI theme generation"
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
