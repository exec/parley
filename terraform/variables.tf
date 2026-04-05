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
  default     = "s-1vcpu-1gb"
}

variable "db_droplet_size" {
  description = "Droplet size for database server"
  type        = string
  default     = "s-1vcpu-2gb"
}

variable "domain_name" {
  description = "Domain name for Parley (e.g. parley.example.com). Set via TF_VAR_domain_name or tfvars."
  type        = string
}

variable "cf_zone" {
  description = "Cloudflare zone (parent domain) that domain_name lives in (e.g. example.com). Auto-detected by infra.yml from the CF API; set manually in tfvars for local runs."
  type        = string
}

variable "api_count" {
  description = "Number of API droplets"
  type        = number
  default     = 2
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
}

variable "jwt_secret" {
  description = "JWT secret for authentication"
  type        = string
  sensitive   = true
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

variable "brevo_api_key" {
  description = "Brevo (Sendinblue) API key for transactional email"
  type        = string
  sensitive   = true
  default     = ""
}

variable "brevo_from_email" {
  description = "From email address for verification emails (e.g. noreply@<domain_name>)"
  type        = string
  default     = ""
}

variable "site_url" {
  description = "Public URL of the site (e.g. https://<domain_name>). Used in email links."
  type        = string
  default     = ""
}

variable "admin_jwt_secret" {
  description = "JWT secret for admin panel authentication"
  type        = string
  sensitive   = true
  default     = ""
}

variable "admin_impersonate_secret" {
  description = "Shared secret for admin impersonation requests"
  type        = string
  sensitive   = true
  default     = ""
}

variable "giphy_api_key" {
  description = "Giphy API key for GIF search in the frontend"
  type        = string
  sensitive   = true
  default     = ""
}

variable "livekit_api_key" {
  description = "LiveKit API key (used by API servers to issue tokens and the VC droplet to authenticate them)"
  type        = string
  sensitive   = true
  default     = ""
}

variable "livekit_api_secret" {
  description = "LiveKit API secret (HMAC key for JWT signing)"
  type        = string
  sensitive   = true
  default     = ""
}

variable "livekit_url" {
  description = "LiveKit Cloud WSS URL (e.g. wss://<project>.livekit.cloud)"
  type        = string
  default     = ""
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
  description = "Ollama model name for AI theme generation"
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
  description = "Redis authentication password (requirepass)"
  type        = string
  sensitive   = true
}

variable "admin_allowed_ip" {
  description = "CIDR allowed to reach the admin panel (e.g. your home IP /32)"
  type        = string
}
