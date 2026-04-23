# Parley

Parley is a self-hosted community platform. It provides servers with text, voice, and code channels; direct messages; a theme system with AI generation; and a bot API with granular per-server permissions — all deployable to your own infrastructure with a single workflow run.

---

## Features

### Servers and channels
- **Text channels** — messages with attachments, replies, reactions, and edit history
- **Voice channels** — peer-to-peer and SFU voice via LiveKit
- **Bin channels** — a code/documentation editor with syntax highlighting, line comments, version history, and tags
- Server roles with a permission bitmask and per-channel overrides
- Invite links, vanity URLs, member management (kick, ban, role assignment)

### Messaging
- Markdown rendering with LaTeX math and syntax-highlighted code blocks
- File and image attachments (stored on S3-compatible object storage)
- Typing indicators, message reactions, reply threading, search
- Real-time delivery via WebSocket; Redis pub/sub fans events across API nodes

### Direct messages and social
- One-to-one DMs with full message feature parity
- Friend requests, friend list

### Themes
- Per-user custom color themes
- Community theme repository — publish, browse, and install themes
- AI theme generation via Ollama cloud (optional)

### Bots and developer API
- Create bots from the Developer settings tab; each gets an encrypted API key
- Shareable invite tokens with granular permission scopes
- Status and presence (online, idle, dnd, invisible, custom status text)
- Python SDK (`extras/parley/`) for writing bots
- See [the bot documentation](docs/bots.md) for the full API reference

### Auth and accounts
- Email verification (Brevo), password reset
- Passkey / WebAuthn support (requires Redis)
- Admin impersonation for support workflows

---

## Architecture

```
                         ┌─────────────────────────────────────────┐
                         │             Cloudflare CDN              │
                         │   (TLS termination, DDoS, DNS, cache)   │
                         └──────────────────┬──────────────────────┘
                                            │
                         ┌──────────────────▼──────────────────────┐
                         │         DigitalOcean Load Balancer      │
                         └────┬──────────────┬──────────────────┬──┘
                              │              │                  │
                   ┌──────────▼──┐  ┌────────▼────┐  ┌──────────▼──┐
                   │  API node 1 │  │  API node 2 │  │  API node 3 │
                   │  Go binary  │  │  Go binary  │  │  Go binary  │
                   │  + nginx    │  │  + nginx    │  │  + nginx    │
                   └──────┬───┬──┘  └───────┬─────┘  └──────┬───┬──┘
                          │   └─────────────╋───────────────┘   │
                          │             Redis pub/sub           │
                          │        (cross-node broadcast)       │
                          │                                     │
                   ┌──────▼─────────────────────────────────────▼─────┐
                   │          PostgreSQL + PgBouncer (DB node)        │
                   └──────────────────────────────────────────────────┘

  Separate services:
  ┌─────────────────┐   ┌──────────────────┐   ┌──────────────────┐
  │   Admin panel   │   │  LiveKit Cloud   │   │  DO Spaces / S3  │
  │ (own JWT, own DB│   │  (voice channels)│   │  (file uploads + │
  │  admin_users)   │   │                  │   │   TF state)      │
  └─────────────────┘   └──────────────────┘   └──────────────────┘
```

**Backend:** Go 1.25 · chi router · PostgreSQL 16 · PgBouncer · Redis 7
**Frontend:** React 18 · TypeScript · Vite
**Infrastructure:** Terraform · GitHub Actions CI/CD · Cloudflare DNS
**Voice:** LiveKit Cloud (or self-hosted)
**Email:** Brevo transactional API
**Storage:** DigitalOcean Spaces (any S3-compatible provider works)

Each API node is stateless — horizontal scaling is adding a droplet. WebSocket fan-out uses Redis pub/sub so any node can broadcast to any connected client.

---

## Getting Started

This guide walks you through self-hosting Parley. You'll need accounts with a few services; most have generous free tiers or cost a few dollars per month.

---

### Table of Contents

1. [Prerequisites](#1-prerequisites)
2. [Cloudflare — DNS and caching](#2-cloudflare--dns-and-caching)
3. [Fork and configure the repository](#3-fork-and-configure-the-repository)
4. [Choose a hosting provider](#4-choose-a-hosting-provider)
5. [Hosting provider setup](#5-hosting-provider-setup)
6. [Object storage (file uploads)](#6-object-storage-file-uploads)
7. [Supporting services](#7-supporting-services)
8. [Terraform state backend](#8-terraform-state-backend)
9. [First infrastructure deploy](#9-first-infrastructure-deploy)
10. [First application deploy](#10-first-application-deploy)
11. [Local development](#11-local-development)
12. [Environment variable reference](#12-environment-variable-reference)
13. [Bot and developer API](#13-bot-and-developer-api)
14. [Load testing](#14-load-testing)

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
# Separate from JWT_SECRET — admin signs impersonation tokens with this key;
# api verifies. See docs/security/runbooks/admin-jwt-secret-separation.md.
gh secret set IMPERSONATION_JWT_SECRET --body "$(openssl rand -base64 32)"

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
| **Proxmox** | Bare-metal self-hosting, home lab, bench testing | Free | ✅ Supported |
| **Hetzner** | Best price/performance, ~3× cheaper than DO | ~€12/mo | 🚧 Planned |
| **Vultr** | Drop-in DO alternative, global coverage | ~$30/mo | 🚧 Planned |
| **Linode (Akamai)** | Established, reliable, similar to DO | ~$32/mo | 🚧 Planned |

The minimum production setup on DigitalOcean (3 API nodes + DB + admin + load balancer) runs around **$72/month**. This is sized for meaningful traffic — a single `s-1vcpu-1gb` node is enough for personal use or small communities and costs under $10/month; edit `api_count = 1` and `api_droplet_size = "s-1vcpu-1gb"` in your `terraform.tfvars`.

---

## 5. Hosting provider setup

### 5.1 DigitalOcean

1. Create an account at [digitalocean.com](https://digitalocean.com)
2. Go to **API → Personal access tokens → Generate New Token**
3. Give it read and write scope, name it `parley`
4. Copy the token and add it to GitHub secrets: `gh secret set DO_TOKEN --body "your-token"`
5. Add your SSH public key to your DO account: **Settings → Security → Add SSH Key**

No further manual setup is needed — Terraform creates all droplets, networking, firewalls, and the load balancer automatically. DNS is managed automatically via Cloudflare (section 2).

### 5.2 Proxmox

Proxmox VE lets you run Parley on bare metal — a home server, a mini PC, or any machine running Proxmox VE 8+. When `api_count = 1` (the default), clients connect directly to the API VM. When `api_count > 1`, Terraform automatically provisions a lightweight nginx VM that load-balances across all API nodes using `ip_hash` sticky sessions and a 1800s WebSocket idle timeout matching the DigitalOcean LB.

**Prerequisites:**
- Proxmox VE 8.x running and reachable on your LAN
- An **Ubuntu 24.04 cloud-init template VM** on Proxmox. Create one like this:

```bash
# On the Proxmox host (or via the web UI terminal)
wget https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img
qm create 9000 --name ubuntu-2404-template --memory 2048 --cores 2 --net0 virtio,bridge=vmbr0
qm importdisk 9000 noble-server-cloudimg-amd64.img local-lvm
qm set 9000 --scsihw virtio-scsi-pci --scsi0 local-lvm:vm-9000-disk-0
qm set 9000 --ide2 local-lvm:cloudinit --boot c --bootdisk scsi0
qm set 9000 --serial0 socket --vga serial0
qm set 9000 --agent enabled=1
qm template 9000
```

**Create a Terraform API token in Proxmox:**
1. Go to **Datacenter → Permissions → API Tokens → Add**
2. User: `root@pam`, Token ID: `terraform`, uncheck **Privilege Separation**
3. Copy the token secret — you won't see it again

**Configure and apply:**

```bash
cd terraform/proxmox
cp terraform.tfvars.example terraform.tfvars
# fill in proxmox_api_url, proxmox_api_token_id, proxmox_api_token_secret
# set your desired IP addresses (api_ip_base, db_ip, admin_ip, gateway)
# fill in db_password, jwt_secret, bot_key_secret, admin_jwt_secret, redis_password

terraform init
terraform apply
```

Terraform provisions a DB VM, one or more API VMs, an admin VM, and (when `api_count > 1`) an nginx LB VM using the same `userdata-*.sh` scripts as DigitalOcean. The `entry_ip` output gives you the address to use — the LB IP when multi-node, the first API VM IP when single-node.

**Key differences from DigitalOcean:**
- Load balancing is handled by a provisioned nginx VM, not a managed service — set `lb_ip` in `terraform.tfvars` before scaling beyond one API node
- No automatic DNS — set a Cloudflare A record to `entry_ip` manually, or use a local DNS entry
- No remote Terraform state by default — state is stored locally in `terraform/proxmox/terraform.tfstate`
- Object storage is handled by a provisioned MinIO VM — no `spaces_*` variables needed, wired automatically

---

## 6. Object storage (file uploads)

Parley stores uploaded images and files in S3-compatible object storage and serves them via CDN.

> **Proxmox users:** A MinIO VM is provisioned automatically — no manual setup needed. The API is pre-configured to point at it. The MinIO web console is available at `http://<minio_ip>:9001`.

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

> **Proxmox users:** State is stored locally in `terraform/proxmox/terraform.tfstate`. Skip this section — no remote backend setup needed.

Terraform needs to store state somewhere so it knows what infrastructure already exists. The DigitalOcean configuration uses DigitalOcean Spaces as a remote backend (the same bucket as file uploads, under the `terraform-state/` prefix).

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

> **Proxmox users:** Run `terraform apply` directly in `terraform/proxmox/` — there is no CI workflow for Proxmox. Skip to section 10 after your VMs are up.

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

### The "admin" badge vs. the admin panel

These are two different things:

- **Admin panel** — a separate service with its own login. Used for server moderation, user management, impersonation, etc. Access is controlled by the `parley-admin` CLI as described above.

- **Admin badge** — a badge you can assign to any Parley user account from the admin panel. It grants exactly one in-app privilege: the ability to **feature and unfeature themes** in the theme repository. It does nothing else.

---

## 11. Local development

### Option A — Docker Compose (everything in one command)

Requires Docker. Starts PostgreSQL, Redis, the Go API, and the React frontend:

```bash
docker compose up
```

| Service | URL |
|---|---|
| Frontend | http://localhost:5173 |
| API | http://localhost:8081 |
| PostgreSQL | localhost:5432 |
| Redis | localhost:6379 |

The API container runs `go run ./cmd/api` with source bind-mounted, so it picks up code changes after a restart (`docker compose restart api`). The frontend container runs the Vite dev server with hot module replacement — edits to `frontend/src/` update instantly without restarting.

First run builds the images (a few minutes). Subsequent starts reuse the cache.

```bash
# Rebuild after changing go.mod or package.json
docker compose build

# View logs
docker compose logs -f api
docker compose logs -f frontend

# Tear down (keeps volumes)
docker compose down

# Tear down and wipe all data
docker compose down -v
```

### Option B — Native (faster Go iteration)

For rapid Go development, run the API natively — no container rebuild needed on code changes.

**Start dependencies:**
```bash
docker compose up -d postgres redis
```

**Start the API:**
```bash
DATABASE_URL="postgres://parley:parley@localhost:5432/parley?sslmode=disable" \
JWT_SECRET=dev \
BOT_KEY_SECRET=dev \
SITE_URL=http://localhost:5173 \
REDIS_URL=redis://:parley@localhost:6379 \
PARLEY_ENV=dev \
PORT=8081 \
go run ./cmd/api
```

`PARLEY_ENV=dev` tells the WebAuthn configuration to accept `http://localhost:5173` and `http://localhost:8080` as valid RP origins for passkeys. Leave this variable unset in production so only the canonical `SITE_URL` is accepted.

**Start the frontend:**
```bash
cd frontend
echo "VITE_SITE_URL=http://localhost:8081" > .env
npm install && npm run dev
```

Frontend dev server at `http://localhost:5173`. API requests are proxied to `:8081`.

### Admin panel (optional, either mode)

```bash
cd admin-frontend
npm install && npm run dev
# Admin panel at http://localhost:5174
```

Admin server (native):
```bash
DATABASE_URL="postgres://parley:parley@localhost:5432/parley?sslmode=disable" \
ADMIN_JWT_SECRET=dev-admin \
IMPERSONATION_JWT_SECRET=dev-imp \
REDIS_HOST=localhost \
REDIS_PASSWORD=parley \
go run ./cmd/admin serve
```

For impersonation to work in dev, the api must be started with the same
`IMPERSONATION_JWT_SECRET` (append `IMPERSONATION_JWT_SECRET=dev-imp \` to
the API server command above).

### Run the test suite

```bash
JWT_SECRET=test-secret BOT_KEY_SECRET=test-secret go test ./...
```

---

## 12. Environment variable reference

All variables are read at API startup. Required variables cause a fatal error if unset; optional ones degrade gracefully with a warning logged.

### API server (`cmd/api`)

| Variable | Required | Default | Purpose |
|---|---|---|---|
| `JWT_SECRET` | **Yes** | — | JWT signing secret for user authentication |
| `IMPERSONATION_JWT_SECRET` | No | — | Key used to verify admin-minted impersonation tokens. Leave unset on api nodes that should refuse all impersonation traffic (e.g. deploys without an admin panel). See `docs/security/runbooks/admin-jwt-secret-separation.md`. |
| `BOT_KEY_SECRET` | **Yes** | — | Derives the AES-256 key for bot API key encryption |
| `DATABASE_URL` | No | `postgres://postgres:postgres@localhost:5432/parley?sslmode=disable` | PostgreSQL connection string |
| `PORT` | No | `8080` | HTTP listen port |
| `SITE_URL` | No | — | Public URL (e.g. `https://parley.example.com`) — used in email verification links and CORS allowlist |
| `PARLEY_ENV` | No | — | Set to `dev` to allow `http://localhost:5173` and `http://localhost:8080` as additional WebAuthn RP origins for local passkey development. Must be unset in production. |
| `REDIS_URL` | No | `redis://localhost:6379` | Redis connection URL (e.g. `redis://:password@localhost:6379`); omit to run in single-node mode without pub/sub |
| `BREVO_API_KEY` | No | — | Brevo transactional email API key; omit to disable email verification |
| `BREVO_FROM_EMAIL` | No | — | Sender address for verification emails |
| `SPACES_ACCESS_KEY` | No | — | S3/Spaces access key; omit to disable file uploads |
| `SPACES_SECRET_KEY` | No | — | S3/Spaces secret key |
| `SPACES_BUCKET` | No | — | Bucket name |
| `SPACES_REGION` | No | — | Region (e.g. `nyc3`) |
| `SPACES_ENDPOINT` | No | — | S3-compatible endpoint URL |
| `SPACES_CDN_URL` | No | — | CDN base URL for serving uploaded files |
| `LIVEKIT_API_KEY` | No | — | LiveKit API key; omit to hide voice channels in the UI |
| `LIVEKIT_API_SECRET` | No | — | LiveKit API secret |
| `LIVEKIT_URL` | No | — | LiveKit WSS URL (e.g. `wss://project.livekit.cloud`) |
| `GIPHY_API_KEY` | No | — | Giphy API key; omit to disable GIF search |
| `OLLAMA_API_KEY` | No | — | Ollama API key; omit to disable AI theme generation |
| `OLLAMA_API_URL` | No | `https://ollama.com/api` | Ollama API base URL |
| `OLLAMA_MODEL` | No | `devstral-small-2:24b-cloud` | Ollama model name |
| `ADMIN_IMPERSONATE_SECRET` | No | — | Shared secret for admin impersonation; omit to disable the endpoint |

### Admin server (`cmd/admin serve`)

| Variable | Required | Default | Purpose |
|---|---|---|---|
| `DATABASE_URL` | **Yes** | — | PostgreSQL connection string |
| `ADMIN_JWT_SECRET` | **Yes** | — | JWT signing secret for admin panel sessions |
| `IMPERSONATION_JWT_SECRET` | **Yes** | — | Signing key for user-impersonation tokens (separate from api's `JWT_SECRET`; see `docs/security/runbooks/admin-jwt-secret-separation.md`) |
| `REDIS_HOST` | No | — | Redis address; used to broadcast force-logout events cross-node |
| `REDIS_PASSWORD` | No | — | Redis password |

---

## 13. Bot and developer API

Bots are first-class users with their own accounts, encrypted API keys, and per-server permission scopes.

### Quick start

1. Open **User Settings → Developer** and click **Create Bot**
2. Copy your API key — it's shown once
3. Generate a shareable invite link from the **My Bots** section and share it with server owners
4. Authenticate all API requests with `Authorization: Bearer plk_<key>`

### Python SDK

A Python SDK is included at `extras/parley/`:

```python
import parley

bot = parley.CommandBot(
    "https://parley.example.com",
    api_key="plk_...",
    command_prefix="!",
)

@bot.command()
async def ping(ctx):
    await ctx.reply("pong!")

@bot.event
async def on_message_create(message):
    print(f"[{message.channel_id}] {message.author.username}: {message.content}")

bot.run()
```

### Documentation

Full API reference, event types, permission scopes, and SDK guide: [`docs/bots.md`](docs/bots.md)

---

## 14. Load testing

Parley ships a load testing CLI at `bench/`. It targets dev or Proxmox instances and refuses to run against the production domain.

### Build

```bash
cd bench
go build -o parley-bench ./cmd/parley-bench
```

### Scenarios

| Command | What it measures |
|---|---|
| `auth-flood` | bcrypt latency and rate limiter behaviour under concurrent logins |
| `ws-scale` | WebSocket connection capacity — finds the hub's connection cliff |
| `message-storm` | Hub mutex contention under N concurrent writers in one channel |
| `broadcast-amp` | Fan-out latency from HTTP POST to WebSocket delivery (1 writer + N listeners) |
| `read-heavy` | Message history read throughput — hits the 120 req/min rate limiter |
| `mixed` | Realistic combined load — 20% writers, 60% readers, 20% typers |

### Usage

```bash
# Target a local dev instance
./parley-bench ws-scale --host http://localhost:8080 --max 500 --sustain 2m

# Target a Proxmox bench instance
./parley-bench mixed --host http://192.168.1.50:8080 --users 200 --duration 15m

# JSON output for scripted collection
./parley-bench broadcast-amp --host http://localhost:8080 --listeners 200 --json

# Skip cleanup to inspect test data after the run
./parley-bench message-storm --host http://localhost:8080 --cleanup=false
```

All scenarios require a running API server. The provisioner endpoint used to seed test data requires `--bench-secret` to match the value set in the server's environment.

> **Safety:** `parley-bench` reads the `DOMAIN` (or `PARLEY_PROD_DOMAIN`) environment variable and refuses to run against any host containing that value. This prevents accidentally load-testing production.
