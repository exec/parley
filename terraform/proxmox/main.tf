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

  # Split api_ip_base into prefix (first 3 octets) and last octet so we can
  # increment correctly.  cidrhost(base/mask, N) returns the Nth address of
  # the *network* (index 0 = network address 192.168.1.0, not .11), so we
  # can't use it here — simple string arithmetic is safer and clearer.
  _api_parts      = split(".", var.api_ip_base)
  api_ip_prefix   = "${local._api_parts[0]}.${local._api_parts[1]}.${local._api_parts[2]}"
  api_ip_last     = tonumber(local._api_parts[3])
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

  ciuser = "root"

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
      "until [ -f /var/lib/cloud/instance/boot-finished ]; do sleep 2; done",
      templatefile("${path.module}/../userdata-db.sh", {
        db_password    = var.db_password
        redis_password = var.redis_password
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

  # Increment IP by index: api_ip_base is the first node's IP (.11 → .11, .12, .13 …)
  ipconfig0  = "ip=${local.api_ip_prefix}.${local.api_ip_last + count.index}/${var.subnet_mask},gw=${var.gateway}"
  nameserver = var.dns_server
  sshkeys    = local.ssh_pub_key

  ciuser = "root"

  provisioner "remote-exec" {
    connection {
      type        = "ssh"
      user        = "root"
      private_key = file(pathexpand(var.ssh_private_key))
      host        = "${local.api_ip_prefix}.${local.api_ip_last + count.index}"
    }

    inline = [
      "until [ -f /var/lib/cloud/instance/boot-finished ]; do sleep 2; done",
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
        SPACES_ACCESS_KEY        = var.minio_access_key
        SPACES_SECRET_KEY        = var.minio_secret_key
        SPACES_BUCKET            = var.minio_bucket
        SPACES_REGION            = "us-east-1"
        SPACES_ENDPOINT          = "http://${var.minio_ip}:9000"
        SPACES_CDN_URL           = "http://${var.minio_ip}:9000/${var.minio_bucket}"
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
        REDIS_PASSWORD           = var.redis_password
      })
    ]
  }

  depends_on = [proxmox_vm_qemu.parley_db, proxmox_vm_qemu.parley_minio]
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
      "until [ -f /var/lib/cloud/instance/boot-finished ]; do sleep 2; done",
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

# ---- Load Balancer VM (nginx, only when api_count > 1) ----
#
# A single API node (the default) is accessed directly — no LB needed.
# When api_count > 1, this VM runs nginx with ip_hash sticky sessions and
# a 1800s WebSocket idle timeout, matching the DO managed load balancer.

resource "proxmox_vm_qemu" "parley_lb" {
  count       = var.api_count > 1 ? 1 : 0
  name        = "parley-lb"
  target_node = var.proxmox_node
  clone       = var.vm_template
  full_clone  = true
  agent       = 1

  cores   = 1
  memory  = 512
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

  ipconfig0  = "ip=${var.lb_ip}/${var.subnet_mask},gw=${var.gateway}"
  nameserver = var.dns_server
  sshkeys    = local.ssh_pub_key

  ciuser = "root"

  provisioner "remote-exec" {
    connection {
      type        = "ssh"
      user        = "root"
      private_key = file(pathexpand(var.ssh_private_key))
      host        = var.lb_ip
    }

    inline = [
      "until [ -f /var/lib/cloud/instance/boot-finished ]; do sleep 2; done",
      templatefile("${path.module}/../userdata-lb.sh", {
        UPSTREAM_SERVERS = join("\n", [
          for i in range(var.api_count) :
          "    server ${local.api_ip_prefix}.${local.api_ip_last + i}:80;"
        ])
      })
    ]
  }

  depends_on = [proxmox_vm_qemu.parley_api]
}

# ---- MinIO VM (S3-compatible object storage) ----
#
# Provides the same API surface as DigitalOcean Spaces so the app code is
# identical across providers. Always provisioned — no DO-style managed bucket
# exists on Proxmox. API VMs depend on this so SPACES_* env vars resolve.

resource "proxmox_vm_qemu" "parley_minio" {
  name        = "parley-minio"
  target_node = var.proxmox_node
  clone       = var.vm_template
  full_clone  = true
  agent       = 1

  cores   = 1
  memory  = 1024
  os_type = "cloud-init"

  disk {
    slot    = "scsi0"
    size    = "20G"
    type    = "scsi"
    storage = var.proxmox_storage
  }

  network {
    model  = "virtio"
    bridge = var.proxmox_bridge
  }

  ipconfig0  = "ip=${var.minio_ip}/${var.subnet_mask},gw=${var.gateway}"
  nameserver = var.dns_server
  sshkeys    = local.ssh_pub_key

  ciuser = "root"

  provisioner "remote-exec" {
    connection {
      type        = "ssh"
      user        = "root"
      private_key = file(pathexpand(var.ssh_private_key))
      host        = var.minio_ip
    }

    inline = [
      "until [ -f /var/lib/cloud/instance/boot-finished ]; do sleep 2; done",
      templatefile("${path.module}/../userdata-minio.sh", {
        minio_access_key = var.minio_access_key
        minio_secret_key = var.minio_secret_key
        minio_bucket     = var.minio_bucket
      })
    ]
  }
}
