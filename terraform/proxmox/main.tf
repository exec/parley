terraform {
  required_version = ">= 1.0"
  required_providers {
    proxmox = {
      source  = "bpg/proxmox"
      version = "~> 0.66"
    }
  }
}

provider "proxmox" {
  endpoint  = var.proxmox_endpoint
  api_token = "${var.proxmox_api_token_id}=${var.proxmox_api_token_secret}"
  insecure  = true

  ssh {
    agent       = false
    username    = var.proxmox_host_user
    private_key = file(pathexpand(var.proxmox_host_ssh_key))
  }
}

locals {
  ssh_pub_key = trimspace(file(pathexpand(var.ssh_public_key)))

  _api_parts    = split(".", var.api_ip_base)
  api_ip_prefix = "${local._api_parts[0]}.${local._api_parts[1]}.${local._api_parts[2]}"
  api_ip_last   = tonumber(local._api_parts[3])

  # Strip trailing G/g from disk size strings; bpg wants integer GB
  db_disk_gb    = tonumber(replace(lower(var.db_disk_size), "g", ""))
  api_disk_gb   = tonumber(replace(lower(var.api_disk_size), "g", ""))
  admin_disk_gb = tonumber(replace(lower(var.admin_disk_size), "g", ""))
  lb_disk_gb    = tonumber(replace(lower(var.lb_disk_size), "g", ""))
  minio_disk_gb = tonumber(replace(lower(var.minio_disk_size), "g", ""))

  db_userdata = templatefile("${path.module}/../userdata-db.sh", {
    db_password    = var.db_password
    redis_password = var.redis_password
  })

  api_userdatas = [
    for i in range(var.api_count) :
    templatefile("${path.module}/../userdata-api.sh", {
      DB_HOST                  = var.db_ip
      DB_PORT                  = "6432"
      DB_NAME                  = "parley"
      DB_USER                  = "parley"
      DB_PASSWORD              = var.db_password
      JWT_SECRET               = var.jwt_secret
      IMPERSONATION_JWT_SECRET = var.impersonation_jwt_secret
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

  admin_userdata = templatefile("${path.module}/../userdata-admin.sh", {
    REPO_URL                 = var.repo_url
    DB_HOST                  = var.db_ip
    DB_PASSWORD              = var.db_password
    REDIS_HOST               = var.db_ip
    ADMIN_JWT_SECRET         = var.admin_jwt_secret
    # F-admin-jwt-secret: admin no longer holds JWT_SECRET. See
    # docs/security/runbooks/admin-jwt-secret-separation.md.
    IMPERSONATION_JWT_SECRET = var.impersonation_jwt_secret
    ADMIN_PORT               = "8080"
  })

  lb_userdata = templatefile("${path.module}/../userdata-lb.sh", {
    UPSTREAM_SERVERS = join("\n", [
      for i in range(var.api_count) :
      "    server ${local.api_ip_prefix}.${local.api_ip_last + i}:80;"
    ])
  })

  minio_userdata = templatefile("${path.module}/../userdata-minio.sh", {
    minio_access_key = var.minio_access_key
    minio_secret_key = var.minio_secret_key
    minio_bucket     = var.minio_bucket
  })
}

# ---- Database container ----

resource "proxmox_virtual_environment_container" "parley_db" {
  node_name     = var.proxmox_node
  unprivileged  = true
  start_on_boot = true
  started       = true

  cpu {
    cores = var.db_cores
  }

  memory {
    dedicated = var.db_memory
  }

  disk {
    datastore_id = var.proxmox_storage
    size         = local.db_disk_gb
  }

  network_interface {
    name   = "eth0"
    bridge = var.proxmox_bridge
  }

  operating_system {
    template_file_id = var.ct_template
    type             = "debian"
  }

  features {
    nesting = true
  }

  initialization {
    hostname = "parley-db"
    ip_config {
      ipv4 {
        address = "${var.db_ip}/${var.subnet_mask}"
        gateway = var.gateway
      }
    }
    dns {
      servers = [var.dns_server]
    }
    user_account {
      keys = [local.ssh_pub_key]
    }
  }
}

resource "null_resource" "db_provision" {
  depends_on = [proxmox_virtual_environment_container.parley_db]

  triggers = {
    vmid   = proxmox_virtual_environment_container.parley_db.vm_id
    script = sha256(local.db_userdata)
  }

  connection {
    type        = "ssh"
    host        = var.proxmox_host_address
    user        = var.proxmox_host_user
    private_key = file(pathexpand(var.proxmox_host_ssh_key))
  }

  provisioner "file" {
    content     = local.db_userdata
    destination = "/tmp/parley-db-userdata.sh"
  }

  provisioner "remote-exec" {
    inline = [
      "until sudo pct exec ${proxmox_virtual_environment_container.parley_db.vm_id} -- getent hosts deb.debian.org >/dev/null 2>&1; do sleep 2; done",
      "sudo pct exec ${proxmox_virtual_environment_container.parley_db.vm_id} -- bash -c 'apt-get update -qq && apt-get install -y -qq sudo locales ca-certificates curl wget'",
      "sudo pct push ${proxmox_virtual_environment_container.parley_db.vm_id} /tmp/parley-db-userdata.sh /root/userdata.sh --perms 0700",
      "sudo pct exec ${proxmox_virtual_environment_container.parley_db.vm_id} -- env PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin bash /root/userdata.sh",
    ]
  }
}

# ---- API containers ----

resource "proxmox_virtual_environment_container" "parley_api" {
  count = var.api_count

  node_name     = var.proxmox_node
  unprivileged  = true
  start_on_boot = true
  started       = true

  cpu {
    cores = var.api_cores
  }

  memory {
    dedicated = var.api_memory
  }

  disk {
    datastore_id = var.proxmox_storage
    size         = local.api_disk_gb
  }

  network_interface {
    name   = "eth0"
    bridge = var.proxmox_bridge
  }

  operating_system {
    template_file_id = var.ct_template
    type             = "debian"
  }

  features {
    nesting = true
  }

  initialization {
    hostname = "parley-api-${count.index + 1}"
    ip_config {
      ipv4 {
        address = "${local.api_ip_prefix}.${local.api_ip_last + count.index}/${var.subnet_mask}"
        gateway = var.gateway
      }
    }
    dns {
      servers = [var.dns_server]
    }
    user_account {
      keys = [local.ssh_pub_key]
    }
  }
}

resource "null_resource" "api_provision" {
  count      = var.api_count
  depends_on = [proxmox_virtual_environment_container.parley_api, null_resource.db_provision, null_resource.minio_provision]

  triggers = {
    vmid   = proxmox_virtual_environment_container.parley_api[count.index].vm_id
    script = sha256(local.api_userdatas[count.index])
  }

  connection {
    type        = "ssh"
    host        = var.proxmox_host_address
    user        = var.proxmox_host_user
    private_key = file(pathexpand(var.proxmox_host_ssh_key))
  }

  provisioner "file" {
    content     = local.api_userdatas[count.index]
    destination = "/tmp/parley-api-${count.index + 1}-userdata.sh"
  }

  provisioner "remote-exec" {
    inline = [
      "until sudo pct exec ${proxmox_virtual_environment_container.parley_api[count.index].vm_id} -- getent hosts deb.debian.org >/dev/null 2>&1; do sleep 2; done",
      "sudo pct exec ${proxmox_virtual_environment_container.parley_api[count.index].vm_id} -- bash -c 'apt-get update -qq && apt-get install -y -qq sudo locales ca-certificates curl wget'",
      "sudo pct push ${proxmox_virtual_environment_container.parley_api[count.index].vm_id} /tmp/parley-api-${count.index + 1}-userdata.sh /root/userdata.sh --perms 0700",
      "sudo pct exec ${proxmox_virtual_environment_container.parley_api[count.index].vm_id} -- env PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin bash /root/userdata.sh",
    ]
  }
}

# ---- Admin container ----

resource "proxmox_virtual_environment_container" "parley_admin" {
  node_name     = var.proxmox_node
  unprivileged  = true
  start_on_boot = true
  started       = true

  cpu {
    cores = var.admin_cores
  }

  memory {
    dedicated = var.admin_memory
  }

  disk {
    datastore_id = var.proxmox_storage
    size         = local.admin_disk_gb
  }

  network_interface {
    name   = "eth0"
    bridge = var.proxmox_bridge
  }

  operating_system {
    template_file_id = var.ct_template
    type             = "debian"
  }

  features {
    nesting = true
  }

  initialization {
    hostname = "parley-admin"
    ip_config {
      ipv4 {
        address = "${var.admin_ip}/${var.subnet_mask}"
        gateway = var.gateway
      }
    }
    dns {
      servers = [var.dns_server]
    }
    user_account {
      keys = [local.ssh_pub_key]
    }
  }
}

resource "null_resource" "admin_provision" {
  depends_on = [proxmox_virtual_environment_container.parley_admin, null_resource.db_provision]

  triggers = {
    vmid   = proxmox_virtual_environment_container.parley_admin.vm_id
    script = sha256(local.admin_userdata)
  }

  connection {
    type        = "ssh"
    host        = var.proxmox_host_address
    user        = var.proxmox_host_user
    private_key = file(pathexpand(var.proxmox_host_ssh_key))
  }

  provisioner "file" {
    content     = local.admin_userdata
    destination = "/tmp/parley-admin-userdata.sh"
  }

  provisioner "remote-exec" {
    inline = [
      "until sudo pct exec ${proxmox_virtual_environment_container.parley_admin.vm_id} -- getent hosts deb.debian.org >/dev/null 2>&1; do sleep 2; done",
      "sudo pct exec ${proxmox_virtual_environment_container.parley_admin.vm_id} -- bash -c 'apt-get update -qq && apt-get install -y -qq sudo locales ca-certificates curl wget'",
      "sudo pct push ${proxmox_virtual_environment_container.parley_admin.vm_id} /tmp/parley-admin-userdata.sh /root/userdata.sh --perms 0700",
      "sudo pct exec ${proxmox_virtual_environment_container.parley_admin.vm_id} -- env PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin bash /root/userdata.sh",
    ]
  }
}

# ---- nginx LB container (only when api_count > 1) ----

resource "proxmox_virtual_environment_container" "parley_lb" {
  count = var.api_count > 1 ? 1 : 0

  node_name     = var.proxmox_node
  unprivileged  = true
  start_on_boot = true
  started       = true

  cpu {
    cores = var.lb_cores
  }

  memory {
    dedicated = var.lb_memory
  }

  disk {
    datastore_id = var.proxmox_storage
    size         = local.lb_disk_gb
  }

  network_interface {
    name   = "eth0"
    bridge = var.proxmox_bridge
  }

  operating_system {
    template_file_id = var.ct_template
    type             = "debian"
  }

  features {
    nesting = true
  }

  initialization {
    hostname = "parley-lb"
    ip_config {
      ipv4 {
        address = "${var.lb_ip}/${var.subnet_mask}"
        gateway = var.gateway
      }
    }
    dns {
      servers = [var.dns_server]
    }
    user_account {
      keys = [local.ssh_pub_key]
    }
  }
}

resource "null_resource" "lb_provision" {
  count      = var.api_count > 1 ? 1 : 0
  depends_on = [proxmox_virtual_environment_container.parley_lb, null_resource.api_provision]

  triggers = {
    vmid   = proxmox_virtual_environment_container.parley_lb[0].vm_id
    script = sha256(local.lb_userdata)
  }

  connection {
    type        = "ssh"
    host        = var.proxmox_host_address
    user        = var.proxmox_host_user
    private_key = file(pathexpand(var.proxmox_host_ssh_key))
  }

  provisioner "file" {
    content     = local.lb_userdata
    destination = "/tmp/parley-lb-userdata.sh"
  }

  provisioner "remote-exec" {
    inline = [
      "until sudo pct exec ${proxmox_virtual_environment_container.parley_lb[0].vm_id} -- getent hosts deb.debian.org >/dev/null 2>&1; do sleep 2; done",
      "sudo pct exec ${proxmox_virtual_environment_container.parley_lb[0].vm_id} -- bash -c 'apt-get update -qq && apt-get install -y -qq sudo locales ca-certificates curl wget'",
      "sudo pct push ${proxmox_virtual_environment_container.parley_lb[0].vm_id} /tmp/parley-lb-userdata.sh /root/userdata.sh --perms 0700",
      "sudo pct exec ${proxmox_virtual_environment_container.parley_lb[0].vm_id} -- env PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin bash /root/userdata.sh",
    ]
  }
}

# ---- MinIO container ----

resource "proxmox_virtual_environment_container" "parley_minio" {
  node_name     = var.proxmox_node
  unprivileged  = true
  start_on_boot = true
  started       = true

  cpu {
    cores = var.minio_cores
  }

  memory {
    dedicated = var.minio_memory
  }

  disk {
    datastore_id = var.proxmox_storage
    size         = local.minio_disk_gb
  }

  network_interface {
    name   = "eth0"
    bridge = var.proxmox_bridge
  }

  operating_system {
    template_file_id = var.ct_template
    type             = "debian"
  }

  features {
    nesting = true
  }

  initialization {
    hostname = "parley-minio"
    ip_config {
      ipv4 {
        address = "${var.minio_ip}/${var.subnet_mask}"
        gateway = var.gateway
      }
    }
    dns {
      servers = [var.dns_server]
    }
    user_account {
      keys = [local.ssh_pub_key]
    }
  }
}

resource "null_resource" "minio_provision" {
  depends_on = [proxmox_virtual_environment_container.parley_minio]

  triggers = {
    vmid   = proxmox_virtual_environment_container.parley_minio.vm_id
    script = sha256(local.minio_userdata)
  }

  connection {
    type        = "ssh"
    host        = var.proxmox_host_address
    user        = var.proxmox_host_user
    private_key = file(pathexpand(var.proxmox_host_ssh_key))
  }

  provisioner "file" {
    content     = local.minio_userdata
    destination = "/tmp/parley-minio-userdata.sh"
  }

  provisioner "remote-exec" {
    inline = [
      "until sudo pct exec ${proxmox_virtual_environment_container.parley_minio.vm_id} -- getent hosts deb.debian.org >/dev/null 2>&1; do sleep 2; done",
      "sudo pct exec ${proxmox_virtual_environment_container.parley_minio.vm_id} -- bash -c 'apt-get update -qq && apt-get install -y -qq sudo locales ca-certificates curl wget'",
      "sudo pct push ${proxmox_virtual_environment_container.parley_minio.vm_id} /tmp/parley-minio-userdata.sh /root/userdata.sh --perms 0700",
      "sudo pct exec ${proxmox_virtual_environment_container.parley_minio.vm_id} -- env PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin bash /root/userdata.sh",
    ]
  }
}
