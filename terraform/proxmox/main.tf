terraform {
  required_version = ">= 1.0"
  required_providers {
    proxmox = {
      source  = "telmate/proxmox"
      version = "~> 2.9"
    }
  }
}

provider "proxmox" {
  pm_api_url          = var.proxmox_api_url
  pm_api_token_id     = var.proxmox_api_token_id
  pm_api_token_secret = var.proxmox_api_token_secret
  pm_tls_insecure     = true  # flip to false if you have a valid cert on Proxmox
}

# ---- Helpers ----

locals {
  ssh_pub_key = trimspace(file(pathexpand(var.ssh_public_key)))
}

# ---- Database VM ----

resource "proxmox_vm_qemu" "parley_db" {
  name        = "parley-db"
  target_node = var.proxmox_node
  clone       = var.vm_template
  full_clone  = true
  agent       = 1  # requires qemu-guest-agent in template

  cores   = var.db_cores
  memory  = var.db_memory
  os_type = "cloud-init"

  disk {
    slot    = "scsi0"
    size    = var.db_disk_size
    type    = "scsi"
    storage = var.proxmox_storage
  }

  network {
    model  = "virtio"
    bridge = var.proxmox_bridge
  }

  ipconfig0  = "ip=${var.db_ip}/${var.subnet_mask},gw=${var.gateway}"
  nameserver = var.dns_server
  sshkeys    = local.ssh_pub_key

  ciuser     = "root"
  cicustom   = ""

  # Cloud-init user data is passed via the userdata-db.sh template
  # For Proxmox, inject via a snippet — see DEPLOYMENT.md for the snippet setup.
  # Alternatively, provision via remote-exec after boot.

  provisioner "remote-exec" {
    connection {
      type        = "ssh"
      user        = "root"
      private_key = file(pathexpand(var.ssh_private_key))
      host        = var.db_ip
    }

    inline = [
      "cloud-init status --wait || true",
      templatefile("${path.module}/../userdata-db.sh", {
        db_password = var.db_password
      })
    ]
  }
}

# ---- API VMs ----

resource "proxmox_vm_qemu" "parley_api" {
  count       = var.api_count
  name        = "parley-api-${count.index + 1}"
  target_node = var.proxmox_node
  clone       = var.vm_template
  full_clone  = true
  agent       = 1

  cores   = var.api_cores
  memory  = var.api_memory
  os_type = "cloud-init"

  disk {
    slot    = "scsi0"
    size    = var.api_disk_size
    type    = "scsi"
    storage = var.proxmox_storage
  }

  network {
    model  = "virtio"
    bridge = var.proxmox_bridge
  }

  # Increment IP by index: api_ip_base is the first node's IP
  ipconfig0  = "ip=${cidrhost(format("%s/%d", var.api_ip_base, var.subnet_mask), count.index)}/${var.subnet_mask},gw=${var.gateway}"
  nameserver = var.dns_server
  sshkeys    = local.ssh_pub_key

  ciuser = "root"

  provisioner "remote-exec" {
    connection {
      type        = "ssh"
      user        = "root"
      private_key = file(pathexpand(var.ssh_private_key))
      host        = cidrhost(format("%s/%d", var.api_ip_base, var.subnet_mask), count.index)
    }

    inline = [
      "cloud-init status --wait || true",
      templatefile("${path.module}/../userdata-api.sh", {
        DB_HOST                  = var.db_ip
        DB_PORT                  = "6432"
        DB_NAME                  = "parley"
        DB_USER                  = "parley"
        DB_PASSWORD              = var.db_password
        JWT_SECRET               = var.jwt_secret
        PORT                     = "8080"
        REPO_URL                 = var.repo_url
        REDIS_HOST               = var.db_ip
        SPACES_ACCESS_KEY        = var.spaces_access_key
        SPACES_SECRET_KEY        = var.spaces_secret_key
        SPACES_BUCKET            = var.spaces_bucket
        SPACES_REGION            = "us-east-1"
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
      })
    ]
  }

  depends_on = [proxmox_vm_qemu.parley_db]
}

# ---- Admin VM ----

resource "proxmox_vm_qemu" "parley_admin" {
  name        = "parley-admin"
  target_node = var.proxmox_node
  clone       = var.vm_template
  full_clone  = true
  agent       = 1

  cores   = var.admin_cores
  memory  = var.admin_memory
  os_type = "cloud-init"

  disk {
    slot    = "scsi0"
    size    = "10G"
    type    = "scsi"
    storage = var.proxmox_storage
  }

  network {
    model  = "virtio"
    bridge = var.proxmox_bridge
  }

  ipconfig0  = "ip=${var.admin_ip}/${var.subnet_mask},gw=${var.gateway}"
  nameserver = var.dns_server
  sshkeys    = local.ssh_pub_key

  ciuser = "root"

  provisioner "remote-exec" {
    connection {
      type        = "ssh"
      user        = "root"
      private_key = file(pathexpand(var.ssh_private_key))
      host        = var.admin_ip
    }

    inline = [
      "cloud-init status --wait || true",
      templatefile("${path.module}/../userdata-admin.sh", {
        REPO_URL                 = var.repo_url
        DB_HOST                  = var.db_ip
        DB_PASSWORD              = var.db_password
        REDIS_HOST               = var.db_ip
        ADMIN_JWT_SECRET         = var.admin_jwt_secret
        PARLEY_JWT_SECRET        = var.jwt_secret
        ADMIN_IMPERSONATE_SECRET = var.admin_impersonate_secret
        ADMIN_PORT               = "8080"
      })
    ]
  }

  depends_on = [proxmox_vm_qemu.parley_db]
}
