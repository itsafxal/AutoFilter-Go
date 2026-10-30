# AutoFilter-Go Deployment Guide

This guide provides step-by-step instructions for deploying **AutoFilter-Go** across various environments: Local Development, Docker, Docker Compose, and production Linux VPS servers.

---

## Table of Contents

1. [Prerequisites](#1-prerequisites)
2. [Local Development](#2-local-development)
3. [Docker Deployment](#3-docker-deployment)
4. [Docker Compose Deployment](#4-docker-compose-deployment)
5. [Linux VPS Deployment (Systemd)](#5-linux-vps-deployment-systemd)
6. [Low-Spec VPS Optimization (1GB RAM)](#6-low-spec-vps-optimization-1gb-ram)
7. [Reverse Proxy Setup for Streaming (Nginx & Caddy)](#7-reverse-proxy-setup-for-streaming-nginx--caddy)
8. [Maintenance & Upgrades](#8-maintenance--upgrades)

---

## 1. Prerequisites

Before deploying, ensure you have gathered the following:

- **Telegram Bot Token**: Obtained from [@BotFather](https://t.me/BotFather).
- **Telegram API ID & API Hash**: Obtained from [my.telegram.org](https://my.telegram.org).
- **Owner Telegram ID**: Your numerical Telegram User ID (obtain via [@userinfobot](https://t.me/userinfobot) or `/id`).
- **MongoDB Database**: A free or paid cluster from [MongoDB Atlas](https://www.mongodb.com/cloud/atlas) or self-hosted MongoDB instance.
- **Go Toolchain**: Go 1.25 or later (for native compilation).
- **Docker & Docker Compose**: (Optional, for containerized deployments).

---

## 2. Local Development

### Step 1: Clone Repository
```bash
git clone https://github.com/itsafxal/AutoFilter-Go.git
cd AutoFilter-Go
```

### Step 2: Configure Environment
Copy the example environment template and configure your credentials:
```bash
cp .env.example .env
```
Edit `.env` using your preferred text editor (e.g. `nano .env` or VS Code) and set your `BOT_TOKEN`, `API_ID`, `API_HASH`, `OWNER_ID`, and `DATABASE_URI`.

### Step 3: Verify & Download Dependencies
```bash
go mod download
go mod verify
```

### Step 4: Run Tests & Static Analysis
```bash
gofmt -l .
go vet ./...
go test -v ./internal/config ./internal/commands ./internal/delivery
```

### Step 5: Build Binary
```bash
# Windows
go build -o ./bin/autofilter-go.exe ./cmd/bot

# Linux / macOS
go build -trimpath -ldflags="-w -s" -o ./bin/autofilter-go ./cmd/bot
```

### Step 6: Start Application
```bash
# Windows (PowerShell)
Get-Content .env | ForEach-Object { if ($_ -match '^([^#=]+)=(.*)$') { [System.Environment]::SetEnvironmentVariable($matches[1].Trim(), $matches[2].Trim()) } }
./bin/autofilter-go.exe

# Linux / macOS
set -a; source .env; set +a
./bin/autofilter-go
```

---

## 3. Docker Deployment

AutoFilter-Go features a hardened multi-stage `Dockerfile`:
- **Builder Stage**: `golang:alpine` builds a stripped, static standalone binary (`CGO_ENABLED=0`).
- **Runtime Stage**: `alpine:3.21` with `ca-certificates` and `tzdata`.
- **Security**: Runs under unprivileged user `botuser` (UID `10001`).

### Step 1: Build Docker Image
```bash
docker build -t autofilter-go:latest .
```

### Step 2: Run Container with Persistent Data
> [!IMPORTANT]
> Always mount a volume to `/app/data` to persist your MTProto session (`bot.session`) and SQLite runtime cache (`media.sqlite3`) across container rebuilds.

```bash
docker run -d \
  --name autofilter-go \
  --restart unless-stopped \
  --env-file .env \
  -e SESSION_PATH=/app/data/bot.session \
  -e SQLITE_DB_PATH=/app/data/media.sqlite3 \
  -v autofilter_data:/app/data \
  -p 8080:8080 \
  autofilter-go:latest
```

### Step 3: Monitor Logs
```bash
docker logs -f autofilter-go
```

---

## 4. Docker Compose Deployment

The repository includes a ready-to-use `docker-compose.yml` preconfigured with memory guards, health monitoring, and persistent volumes.

### Step 1: Launch Application
```bash
docker compose up -d
```

### Step 2: View Logs
```bash
docker compose logs -f bot
```

### Step 3: Stop or Restart
```bash
# Stop application
docker compose down

# Restart application
docker compose restart bot
```

### Step 4: Rebuild After Changes
```bash
docker compose build --no-cache
docker compose up -d
```

---

## 5. Linux VPS Deployment (Systemd)

This is the recommended production deployment method for dedicated or virtual Linux servers (Ubuntu 22.04 / 24.04 LTS, Debian 12).

### Step 1: Install Prerequisites
```bash
sudo apt update && sudo apt install -y git curl build-essential
```

### Step 2: Install Go (if not installed)
```bash
curl -OL https://go.dev/dl/go1.27.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.27.0.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
```

### Step 3: Create Dedicated Service User
```bash
sudo adduser --system --group --no-create-home --home /opt/autofilter-go botuser
```

### Step 4: Clone & Build
```bash
sudo git clone https://github.com/itsafxal/AutoFilter-Go.git /opt/autofilter-go
cd /opt/autofilter-go
sudo cp .env.example .env
sudo nano .env # Fill in your credentials

# Build standalone binary
sudo /usr/local/go/bin/go build -trimpath -ldflags="-w -s" -o /opt/autofilter-go/autofilter-go ./cmd/bot

# Create data directory and set permissions
sudo mkdir -p /opt/autofilter-go/data
sudo chown -R botuser:botuser /opt/autofilter-go
sudo chmod 0750 /opt/autofilter-go/data
```

### Step 5: Create Systemd Service Unit
Create `/etc/systemd/system/autofilter-go.service`:
```ini
[Unit]
Description=AutoFilter-Go Telegram Bot & Streaming Engine
After=network.target

[Service]
Type=simple
User=botuser
Group=botuser
WorkingDirectory=/opt/autofilter-go
EnvironmentFile=/opt/autofilter-go/.env
ExecStart=/opt/autofilter-go/autofilter-go
Restart=always
RestartSec=5s

# Hardening & Security
NoNewPrivileges=true
ProtectSystem=full
ProtectHome=true
PrivateTmp=true

# Memory Protection (Prevents VPS OOM-Kills)
Environment=GOMEMLIMIT=512MiB
Environment=GOGC=60
MemoryHigh=700M
MemoryMax=850M

[Install]
WantedBy=multi-user.target
```

### Step 6: Enable & Start Service
```bash
sudo systemctl daemon-reload
sudo systemctl enable autofilter-go
sudo systemctl start autofilter-go
sudo systemctl status autofilter-go
```

To tail live logs:
```bash
journalctl -u autofilter-go -f -o cat
```

---

## 6. Low-Spec VPS Optimization (1GB RAM)

If deploying on a budget VPS (e.g. 1 vCPU, 1GB RAM from Hetzner, DigitalOcean, or AWS Lightsail), follow these recommendations:

### 1. Enable Zero-Sync Mode (`ENABLE_SYNC=false`)
If you have a massive MongoDB collection (500k+ media files) and your VPS has only 1GB RAM, SQLite synchronization can take memory. You can configure:
```env
ENABLE_SYNC=false
```
In this mode:
- Bot boots in **1 second**.
- No SQLite indexing runs.
- Search queries query MongoDB directly.
- Idle RAM consumption drops to **~50 MB**.

### 2. Configure Go Runtime Memory Limits
AutoFilter-Go has built-in runtime tuning (`initRuntimeTuning`). Set these in your `.env`:
```env
GOMEMLIMIT=512MiB
GOGC=60
MAX_CPU_CORES=1
SYNC_BATCH_SIZE=1500
```

### 3. Add a Swap File
On Linux, a 2GB swap file prevents sudden out-of-memory crashes:
```bash
sudo fallocate -l 2G /swapfile
sudo chmod 600 /swapfile
sudo mkswap /swapfile
sudo swapon /swapfile
echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab
```

---

## 7. Reverse Proxy Setup for Streaming (Nginx & Caddy)

To serve HTTP streaming and fast download links over secure HTTPS (`https://stream.yourdomain.com`), place the bot behind a reverse proxy.

### Nginx Configuration
```nginx
server {
    server_name stream.yourdomain.com;

    client_max_body_size 0;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Disable buffering for video byte-range streaming
        proxy_buffering off;
        proxy_request_buffering off;
        proxy_read_timeout 600s;
        proxy_send_timeout 600s;
    }
}
```
Install SSL via Certbot:
```bash
sudo certbot --nginx -d stream.yourdomain.com
```

### Caddy Configuration (Automatic SSL)
```caddyfile
stream.yourdomain.com {
    reverse_proxy 127.0.0.1:8080 {
        header_up Host {host}
        header_up X-Real-IP {remote_host}
    }
}
```

Then in `.env`, update:
```env
STREAM_URL=https://stream.yourdomain.com
STREAM_BIND_ADDR=127.0.0.1
```

---

## 8. Maintenance & Upgrades

To update AutoFilter-Go to the latest release:

```bash
cd /opt/autofilter-go
sudo systemctl stop autofilter-go
sudo git pull origin main
sudo /usr/local/go/bin/go build -trimpath -ldflags="-w -s" -o autofilter-go ./cmd/bot
sudo systemctl start autofilter-go
sudo journalctl -u autofilter-go -f -o cat
```

