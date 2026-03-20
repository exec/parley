# Getting Started

This guide walks you through self-hosting Parley. You'll need accounts with a few services; most have generous free tiers or cost a few dollars per month.

---

## Table of Contents

1. [Prerequisites](#1-prerequisites)
2. [Cloudflare — DNS and caching](#2-cloudflare--dns-and-caching)
3. [Fork and configure the repository](#3-fork-and-configure-the-repository)
4. [Choose a hosting provider](#4-choose-a-hosting-provider)
5. [DigitalOcean setup](#5-digitalocean-setup)
6. [Object storage (file uploads)](#6-object-storage-file-uploads)
7. [Supporting services](#7-supporting-services)
8. [Terraform state backend](#8-terraform-state-backend)
9. [First infrastructure deploy](#9-first-infrastructure-deploy)
10. [First application deploy](#10-first-application-deploy)
11. [Local development](#11-local-development)

---

## 1. Prerequisites

Before you begin, install the following tools:

| Tool | Purpose | Install |
|---|---|---|
| [Terraform ≥ 1.7](https://developer.hashicorp.com/terraform/install) | Provision infrastructure | `brew install terraform` |
| [GitHub CLI](https://cli.github.com/) | Set secrets from the command line | `brew install gh` |
| [Go ≥ 1.22](https://go.dev/dl/) | Build the API | `brew install go` |
| [Node.js ≥ 20](https://nodejs.org/) | Build the frontend | `brew install node` |
| [jq](https://jqlang.github.io/jq/) | Used by CI scripts | `brew install jq` |

You also need:
- A domain name pointed to Cloudflare nameservers (see next section)
- A GitHub account with Actions enabled
- SSH key pair (`ssh-keygen -t ed25519`)

---

## 2. Cloudflare — DNS and caching

Cloudflare sits in front of your infrastructure. It handles DNS, terminates TLS, caches static assets, conceals your server IPs from the public internet, and protects against DDoS. It also makes domain migration trivial — you update one DNS record and the change propagates globally in seconds.

### 2.1 Create an account and add your domain

1. Sign up at [cloudflare.com](https://cloudflare.com).
2. Click **Add a site** and enter your domain name.
3. Select the **Free** plan (it covers everything you need).
4. Cloudflare will show you two nameserver hostnames. Go to your domain registrar and replace the existing nameservers with these two. Propagation typically takes a few minutes to a few hours.

### 2.2 Create a scoped API token

Parley's infrastructure automation needs a Cloudflare API token to manage DNS records. Use a **scoped API token** — not your Global API Key (the Global API Key has unrestricted access to your entire Cloudflare account; treat it like a root password and never put it in automation).

To create a scoped token:

1. Go to **My Profile → API Tokens → Create Token**
2. Click **Create Custom Token**
3. Give it a name like `parley-infra`
4. Under **Permissions**, add:
   - **Zone → Zone → Read**
   - **Zone → DNS → Edit**
5. Under **Zone Resources**, select **Include → All zones** (or restrict to your specific zone if you prefer)
6. Click **Continue to summary → Create Token**
7. Copy the token — you won't see it again

Add it to your GitHub repository secrets:

```bash
gh secret set CLOUDFLARE_API_KEY --body "your-token-here"
```

The infra workflow will automatically query the Cloudflare API to determine which zone your domain lives in, then create or update the DNS A record pointing your domain to the load balancer. You never need to set a DNS record manually.

---

## 3. Fork and configure the repository

### 3.1 Fork

```bash
gh repo fork exec/parley --clone
cd parley
```

### 3.2 Set the domain variable

```bash
gh variable set DOMAIN --body "parley.yourdomain.com"
```

This is the single source of truth for your domain. The deploy workflow injects it into frontend builds, Terraform uses it for DNS and TLS, and the backend uses it for email verification links. To migrate to a new domain later, update this variable and run the infra workflow.

### 3.3 Set GitHub secrets

All sensitive configuration lives in GitHub secrets and is never committed to the repository. Set them all at once:

```bash
# DigitalOcean
gh secret set DO_TOKEN --body "your-do-token"

# Database
gh secret set DB_PASSWORD --body "$(openssl rand -base64 20)"

# Authentication
gh secret set JWT_SECRET --body "$(openssl rand -base64 32)"
gh secret set ADMIN_JWT_SECRET --body "$(openssl rand -base64 32)"
gh secret set ADMIN_IMPERSONATE_SECRET --body "$(openssl rand -base64 32)"

# Object storage (see section 6)
gh secret set SPACES_ACCESS_KEY --body "your-spaces-key"
gh secret set SPACES_SECRET_KEY --body "your-spaces-secret"
gh secret set SPACES_CDN_URL --body "https://your-bucket.nyc3.cdn.digitaloceanspaces.com"

# Email verification (see section 7)
gh secret set BREVO_API_KEY --body "your-brevo-key"

# Bot encryption key
gh secret set BOT_KEY_SECRET --body "$(openssl rand -base64 32)"

# Redis
gh secret set REDIS_PASSWORD --body "$(openssl rand -base64 32)"

# Admin panel access restriction (your home IP /32)
gh secret set ADMIN_ALLOWED_IP --body "$(curl -sf https://ifconfig.me)/32"

# SSH key for server access (public key content, not path)
gh secret set SSH_PUBLIC_KEY --body "$(cat ~/.ssh/id_ed25519.pub)"

# Git repository URL
gh secret set REPO_URL --body "https://github.com/your-username/parley.git"
```

Optional services (leave empty if not using):

```bash
gh secret set LIVEKIT_API_KEY --body ""       # Voice channels
gh secret set LIVEKIT_API_SECRET --body ""
gh secret set GIPHY_API_KEY --body ""         # GIF search
gh secret set OLLAMA_API_KEY --body ""        # AI theme generation
```

---

## 4. Choose a hosting provider

Parley currently ships Terraform configuration for **DigitalOcean** (production) and **Proxmox** (local bench testing). Other providers are planned.

| Provider | Best for | Starting cost | Status |
|---|---|---|---|
| **DigitalOcean** | Simplest setup, managed LB, Spaces storage | ~$36/mo | ✅ Supported |
| **Proxmox** | Local testing and benchmarking on bare metal | Free | ✅ Supported (bench only) |
| **Hetzner** | Best price/performance, ~3× cheaper than DO | ~€12/mo | 🚧 Planned |
| **Vultr** | Drop-in DO alternative, global coverage | ~$30/mo | 🚧 Planned |
| **Linode (Akamai)** | Established, reliable, similar to DO | ~$32/mo | 🚧 Planned |

The minimum production setup on DigitalOcean (3 API nodes + DB + admin + load balancer) runs around **$70/month**. This is sized for meaningful traffic — a single `s-1vcpu-1gb` node is enough for personal use or small communities and costs under $10/month; edit `api_count = 1` and `api_droplet_size = "s-1vcpu-1gb"` in your `terraform.tfvars`.

---

## 5. DigitalOcean setup

1. Create an account at [digitalocean.com](https://digitalocean.com)
2. Go to **API → Personal access tokens → Generate New Token**
3. Give it read and write scope, name it `parley`
4. Copy the token and add it to GitHub secrets: `gh secret set DO_TOKEN --body "your-token"`
5. Add your SSH public key to your DO account: **Settings → Security → Add SSH Key**

No further manual setup is needed — Terraform creates all droplets, networking, firewalls, and the load balancer automatically.

---

## 6. Object storage (file uploads)

Parley stores uploaded images and files in S3-compatible object storage and serves them via CDN. The default configuration uses **DigitalOcean Spaces**.

### DigitalOcean Spaces

1. In the DO dashboard, go to **Spaces Object Storage → Create a Space**
2. Choose the same region as your droplets (e.g. `nyc3`)
3. Name it (e.g. `parley-prod`)
4. Enable the CDN endpoint
5. Go to **API → Spaces access keys → Generate New Key**
6. Copy the key ID and secret

```bash
gh secret set SPACES_ACCESS_KEY --body "your-key-id"
gh secret set SPACES_SECRET_KEY --body "your-secret"
gh secret set SPACES_CDN_URL --body "https://parley-prod.nyc3.cdn.digitaloceanspaces.com"
```

Edit `terraform/terraform.tfvars`:
```hcl
spaces_bucket   = "parley-prod"
spaces_endpoint = "https://nyc3.digitaloceanspaces.com"
```

> **Using a different provider?** Any S3-compatible storage works (Vultr Object Storage, Backblaze B2, Cloudflare R2, AWS S3). Update `spaces_endpoint` to point at your provider's S3 API and set `SPACES_CDN_URL` to the CDN or public URL.

---

## 7. Supporting services

### Email verification — Brevo (free tier)

Email verification uses [Brevo](https://brevo.com) (formerly Sendinblue). The free tier allows 300 emails/day which is plenty for most deployments.

1. Create an account
2. Go to **SMTP & API → API Keys → Generate a new API key**
3. Add it: `gh secret set BREVO_API_KEY --body "your-key"`

### Voice channels — LiveKit Cloud (optional)

Voice channels require a LiveKit server. The easiest option is [LiveKit Cloud](https://livekit.io/cloud), which has a generous free tier.

1. Create a project
2. Copy the API Key, API Secret, and WSS URL from the project settings
3. Add them:
   ```bash
   gh secret set LIVEKIT_API_KEY --body "your-key"
   gh secret set LIVEKIT_API_SECRET --body "your-secret"
   ```
4. Update `terraform.tfvars`:
   ```hcl
   livekit_url = "wss://your-project.livekit.cloud"
   ```

If you don't add LiveKit credentials, voice channels are hidden from the UI automatically.

### GIF search — Giphy (optional)

1. Create an app at [developers.giphy.com](https://developers.giphy.com)
2. `gh secret set GIPHY_API_KEY --body "your-key"`

---

## 8. Terraform state backend

Terraform needs to store state somewhere so it knows what infrastructure already exists. The configuration uses DigitalOcean Spaces as a remote backend (the same bucket as file uploads, under the `terraform-state/` prefix).

The first time you run Terraform locally, migrate the state to the remote backend:

```bash
cd terraform
cp terraform.tfvars.example terraform.tfvars
# fill in your values in terraform.tfvars

export AWS_ACCESS_KEY_ID="your-spaces-key-id"
export AWS_SECRET_ACCESS_KEY="your-spaces-secret"

terraform init -migrate-state
```

After this, every Terraform run — local or CI — shares the same state.

> **terraform.tfvars is gitignored.** Never commit it. It contains your database password, API keys, and other secrets.

---

## 9. First infrastructure deploy

With secrets set and state initialized, trigger the infra workflow:

```bash
gh workflow run infra.yml -f action=apply
```

Or push any change to `terraform/` and it runs automatically.

The workflow will:
1. Query the Cloudflare API to find which zone your `DOMAIN` belongs to
2. Run `terraform apply` to create all droplets, networking, firewalls, and load balancer
3. Create or update the DNS A record pointing `DOMAIN` → load balancer IP via Cloudflare

Monitor it: `gh run watch`

The load balancer IP and server IPs are printed as Terraform outputs at the end.

---

## 10. First application deploy

Push to `main` and the deploy workflow runs automatically:

```bash
git push origin main
```

Or trigger it manually: `gh workflow run deploy.yml`

The workflow:
1. SSH into all API nodes in parallel
2. Pulls latest code, builds the Go binary, builds the React frontend with your domain baked in
3. Restarts the API service and health-checks it
4. Builds and deploys the admin panel

First deploy on a fresh server takes ~5 minutes. Subsequent deploys take ~2 minutes.

### Create your first admin account

The admin panel has its own user database separate from the main app. There are no default credentials — you create your first account via CLI on the admin server after deployment.

Get the admin server IP from the Terraform output:

```bash
cd terraform
terraform output admin_droplet_ip
```

SSH in and create your account:

```bash
ssh root@<admin-server-ip>

# Create a user (prompts for password)
parley-admin create-user yourname

# New users are inactive by default — activate to enable login
parley-admin activate yourname

# Verify
parley-admin list-users
```

Other admin CLI commands you'll want:

```bash
parley-admin reset-password <username>   # Prompted password reset
parley-admin deactivate <username>       # Revoke access without deleting
parley-admin list-users                  # Show all admins, status, last login
```

The admin panel is accessible **only from your IP** (the `ADMIN_ALLOWED_IP` secret you set earlier). Browse to `http://<admin-server-ip>/` from that IP and log in with the credentials you just created.

> **Note:** Admin authentication uses a separate JWT secret (`ADMIN_JWT_SECRET`) from the main app's `JWT_SECRET`. You can rotate one without invalidating the other.

---

## 11. Local development

```bash
# Start PostgreSQL and Redis (requires Docker)
docker compose up -d

# Start the API
JWT_SECRET=dev SITE_URL=http://localhost:8080 go run ./cmd/api

# In another terminal, start the frontend
cd frontend
echo "VITE_SITE_URL=http://localhost:8080" > .env
npm install && npm run dev
```

The frontend dev server runs at `http://localhost:5173` and proxies API requests to `:8080`.

For the admin panel:
```bash
cd admin-frontend
npm install && npm run dev
# Admin panel at http://localhost:5174
```

Run the test suite:
```bash
JWT_SECRET=test-secret go test ./...
```
