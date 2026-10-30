<div align="center">

<img src="assets/banner.svg" alt="AutoFilter-Go Header Banner" width="100%" />

# AutoFilter-Go

### Enterprise-Grade Telegram MTProto Media Indexer & Auto-Filter Engine

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev/)
[![Gogram MTProto](https://img.shields.io/badge/Gogram_MTProto-v1.7.71-24A1DE?style=for-the-badge&logo=telegram&logoColor=white)](https://github.com/AmarnathCJD/gogram)
[![MongoDB](https://img.shields.io/badge/MongoDB-Multi--Cluster-47A248?style=for-the-badge&logo=mongodb&logoColor=white)](https://www.mongodb.com/)
[![SQLite](https://img.shields.io/badge/SQLite-Pure--Go_WAL-003B57?style=for-the-badge&logo=sqlite&logoColor=white)](https://modernc.org/sqlite)
[![Docker](https://img.shields.io/badge/Docker-Multi--Stage_Build-2496ED?style=for-the-badge&logo=docker&logoColor=white)](https://www.docker.com/)
[![License](https://img.shields.io/badge/License-MIT-blue.style=for-the-badge)](LICENSE)

<p align="center">
  <a href="#-project-overview">Overview</a> •
  <a href="#-feature-showcase">Features</a> •
  <a href="#-system-architecture">Architecture</a> •
  <a href="#-command-reference">Commands</a> •
  <a href="#-configuration-guide">Configuration</a> •
  <a href="#-deployment-guide">Deployment</a> •
  <a href="#-database-topology">Database</a> •
  <a href="#-plugin-system">Plugins</a> •
  <a href="docs/">Documentation</a>
</p>

---

</div>

## 📌 Project Overview

**AutoFilter-Go** is a high-concurrency, enterprise-grade Telegram bot engineered in Go. Powered by the native [Gogram MTProto](https://github.com/AmarnathCJD/gogram) framework (`github.com/amarnathcjd/gogram/telegram` v1.7.71), it bridges the gap between massive cloud media archives and instant client delivery.

Traditional Telegram search bots suffer from HTTP webhook rate limits, excessive MongoDB round-trips, slow query responses, and single-point-of-failure storage bottlenecks. **AutoFilter-Go** solves this with a **sharded dual-database architecture**:
1. **Authoritative Cloud Persistence**: High-volume MongoDB clusters cleanly partition volatile user state from media archives, distributing hundreds of thousands of files across round-robin balanced shards (`DB2` and `DB3`).
2. **Sub-Millisecond Runtime Caching**: A 100% pure-Go, CGO-free SQLite engine (`modernc.org/sqlite`) running in Write-Ahead Logging (`WAL`) mode provides sub-millisecond full-text searches, fuzzy tokenization, and instantaneous pre-write deduplication.
3. **Native MTProto Delivery**: Direct MTProto binary connections resolve Bot API `file_id` strings without external HTTP API hops, supporting protected content delivery (`NoForwards`), inline pagination with Telegram Premium custom emojis, and built-in HTTP streaming with byte-range video seeking (RFC 7233).

---

## ⚡ Feature Showcase

| Feature Category | Technical Capability | Source Package |
| :--- | :--- | :--- |
| **Live Channel Ingestion** | Background watchers monitor designated channels, extract metadata (`file_id`, `file_name`, `file_size`, `file_type`), and run pre-write deduplication (<0.1ms). | [`plugins/channel`](plugins/channel/channel.go) |
| **Bulk Channel Indexing** | Deep indexing of public/private channels via `/index`, range indexing between two post URLs via `/indexlink`, and offset resumption via `/setskip`. | [`plugins/index`](plugins/index/index.go) |
| **Ambient Auto-Filter** | Listens for non-command text queries in connected groups, sanitizes release group tags (`[1TamilMV]`, `@CinemaCourt`), and executes fuzzy multi-word matching. | [`plugins/autofilter`](plugins/autofilter/autofilter.go) |
| **Interactive Pagination** | Dynamic 8-files-per-page keyboard with Telegram Premium custom animated emoji entities and session caching. | [`internal/search`](internal/search/service.go) |
| **Multi-Tier Force-Subscribe** | 3-tier Force-Subscribe engine supporting up to 3 primary and 3 secondary channels with automatic threshold-based failover. | [`internal/fsub`](internal/fsub/service.go) |
| **Two-Tier Media Delivery** | Sub-millisecond lookup against SQLite with fallback to MongoDB shards, protected content (`NoForwards: true`), and double-click deduplication. | [`internal/delivery`](internal/delivery/service.go) |
| **HTTP Streaming Server** | Built-in HTTP server supporting RFC 7233 byte-range video seeking, HMAC-SHA256 signed links, HTML5 player, and M3U playlists for VLC/MX Player. | [`internal/stream`](internal/stream/server.go) |
| **Auto-Delete & Weekly Trash** | Automated scheduled deletion of filter results (`15m`), delivered files (`30m`), and weekly garbage collection to prevent copyright strikes. | [`internal/autodelete`](internal/autodelete/service.go) |
| **Legacy Bot Migration** | Concurrently runs legacy or retiring bot tokens in background goroutines to seamlessly funnel old users into the main bot via deep links. | [`internal/oldbot`](internal/oldbot/service.go) |
| **Global Keyword Filters** | Global auto-responders across all groups with inline buttons formatting (`[Text | https://...]`). | [`plugins/gfilter`](plugins/gfilter/gfilter.go) |
| **VPS Memory Protection** | Native runtime tuning (`GOMEMLIMIT`, `GOGC`, `MAX_CPU_CORES`) with a **Zero-Sync Mode** (`ENABLE_SYNC=false`) that boots in 1s on 512MB VPS. | [`internal/app`](internal/app/app.go) |

---

## 🛠️ Technology Stack

AutoFilter-Go is built with modern, battle-tested Go dependencies:

- **Language Runtime**: [Go 1.25+](https://go.dev/) (Statically compiled with `CGO_ENABLED=0`, `-trimpath`, `-ldflags="-w -s"`).
- **Telegram MTProto Framework**: [`github.com/amarnathcjd/gogram`](https://github.com/amarnathcjd/gogram) `v1.7.71` (Pure Go Telegram MTProto client).
- **Authoritative Database Driver**: [`go.mongodb.org/mongo-driver`](https://pkg.go.dev/go.mongodb.org/mongo-driver) `v1.17.1`.
- **Runtime Index Engine**: [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) `v1.36.0` (100% pure-Go, CGO-free SQLite).
- **Structured Logging**: Standard library `log/slog` with automatic secret masking and redaction.
- **Containerization**: Hardened multi-stage [Docker](https://www.docker.com/) image running on `alpine:3.21` as an unprivileged non-root user (`UID 10001`).

---

## 🏗️ System Architecture

AutoFilter-Go coordinates four specialized operational layers:

```mermaid
flowchart TD
    subgraph ClientLayer ["1. Telegram MTProto Client Layer"]
        Gogram["Gogram MTProto Engine (v1.7.71)"]
        OldBots["Legacy Bots Migration Service (internal/oldbot)"]
        StreamServer["HTTP Streaming Server (internal/stream)"]
    end

    subgraph AppLayer ["2. Application & Dispatch Layer"]
        App["App Container (internal/app)"]
        Registry["Compile-Time Plugin Registry (internal/plugins)"]
        InFlight["In-Flight Request Deduplicator (internal/delivery)"]
    end

    subgraph CoreServices ["3. Core Functional Services"]
        SearchSvc["Search & Normalization (internal/search)"]
        DeliverySvc["Two-Tier Media Delivery (internal/delivery)"]
        FSubSvc["3-Tier FSub Failover (internal/fsub)"]
        AutoDelSvc["Auto-Delete & Weekly Trash (internal/autodelete)"]
    end

    subgraph DatabaseLayer ["4. Dual-Database Topology"]
        SQLite[("Pure-Go SQLite Cache<br/>WAL Mode • PRAGMA cache_size=-64000<br/>Sub-millisecond Search & Pre-Write Dedup")]
        MongoPrimary[("Primary MongoDB<br/>Users, Groups, FSub, Captions")]
        MongoDB2[("Secondary MongoDB (DB2)<br/>Media Shard A")]
        MongoDB3[("Tertiary MongoDB (DB3)<br/>Media Shard B")]
    end

    Gogram <--> Registry
    OldBots -->|Deep-Link Funneling| Gogram
    StreamServer <-->|Byte-Range Streams (RFC 7233)| Gogram

    Registry --> SearchSvc
    Registry --> DeliverySvc
    Registry --> FSubSvc
    Registry --> AutoDelSvc

    SearchSvc <-->|Indexed Search| SQLite
    SearchSvc -.->|Direct Fallback| MongoDB2 & MongoDB3

    DeliverySvc --> InFlight
    InFlight -->|1. Check Cache| SQLite
    InFlight -->|2. Fallback Fetch| MongoDB2 & MongoDB3

    FSubSvc <--> MongoPrimary
    AutoDelSvc <--> SQLite
```

> 📖 **Deep Dive**: For full sequence diagrams and MTProto dispatcher specifications, see [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

---

## 📂 Repository Structure

The codebase is organized into modular packages adhering to standard Go project conventions:

```text
AutoFilter-Go/
├── cmd/
│   └── bot/                     # Application entrypoint & signal orchestration
│       └── main.go              # Runtime tuning, plugin registration, main()
├── internal/
│   ├── app/                     # Unified application lifecycle container
│   ├── autodelete/              # Timed message deletion & weekly trash service
│   ├── caption/                 # Custom caption template rendering engine
│   ├── commands/                # Command parsing, HTML escaping, payload validation
│   ├── config/                  # Strict environment configuration loader & validators
│   ├── database/                # Dual-database coordinator
│   │   ├── manager.go           # MongoDB cluster and SQLite synchronization manager
│   │   ├── mongodb/             # MongoDB client, repositories, and sharded storage
│   │   └── sql/                 # Pure-Go SQLite schema, WAL setup, and sync engine
│   ├── delivery/                # Two-tier media resolution, deduplication, and sender
│   ├── fsub/                    # 3-tier Force Subscribe routing and membership cache
│   ├── gfilter/                 # Global keyword auto-filter model and button parser
│   ├── logging/                 # Structured slog wrapper with credential redaction
│   ├── oldbot/                  # Background legacy bot user migration service
│   ├── plugins/                 # Compile-time plugin registry and interfaces
│   ├── search/                  # Query normalization, tokenizer, and fuzzy matching
│   ├── stream/                  # HTTP video streaming, fast download, and web player
│   └── telegram/                # Gogram MTProto client lifecycle and DC connections
├── plugins/                     # Compile-time functional plugins
│   ├── autodelete/              # Administrative auto-delete configuration commands
│   ├── autofilter/              # Ambient movie search and interactive pagination
│   ├── botstats/                # /stats reporting and /vacuum database compaction
│   ├── caption/                 # /set_caption, /view_caption, and /del_caption
│   ├── channel/                 # Monitored channel live media indexing watcher
│   ├── cmds/                    # Dynamic /cmds guide and categorized /cmdc menus
│   ├── delivery/                # /getfile handler and media callback processor
│   ├── fsub/                    # Force-Subscribe administrative configuration commands
│   ├── gfilter/                 # Global filter management commands (/gfilter, /gfilters)
│   ├── id/                      # /id chat, user, and message metadata inspector
│   ├── index/                   # /index, /indexlink, and /setskip channel indexer
│   ├── info/                    # /info and /about bot diagnostics overview
│   ├── latest/                  # /latest movie releases and user /request command
│   ├── ping/                    # /ping response speed and MTProto latency measurement
│   ├── start/                   # /start welcome message and deep-link processor
│   └── stream/                  # /fastdl and /stream direct download link generator
├── assets/                      # Repository branding, SVG banner, logo, and welcome picture
├── docs/                        # In-depth architectural, deployment, and technical guides
├── docker-compose.yml           # Production Docker Compose specification with volume persistence
├── Dockerfile                   # Hardened multi-stage CGO-free build (non-root botuser)
├── go.mod                       # Go module declaration (github.com/itsafxal/AutoFilter-Go)
└── README.md                    # Repository documentation and user manual
```

---

## 🎮 Command Reference

All commands are compiled into the binary, verified at startup, and protected by role-based access controls (`internal/plugins`).

<details open>
<summary><b>1. General & User Commands</b></summary>
<br/>

| Command | Aliases | Description | Permissions | Context | Usage Example |
| :--- | :--- | :--- | :---: | :---: | :--- |
| `/start` | — | Start the bot, receive welcome message, or process media deep-links | Everyone | All Chats | `/start`<br/>`/start file_12345` |
| `/cmds` | `/help` | View complete, dynamically compiled command guide | Everyone | All Chats | `/cmds` |
| `/ping` | — | Measure bot processing latency and MTProto round-trip time | Everyone | All Chats | `/ping` |
| `/id` | — | Display current chat ID, sender user ID, and replied message ID | Everyone | All Chats | `/id` |
| `/info` | `/about` | View bot information, Gogram MTProto runtime diagnostics, and user profile | Everyone | All Chats | `/info` |
| `/latest` | — | Browse recently added movie and series files | Everyone | All Chats | `/latest` |
| `/request` | `/req` | Submit a movie or series title request to the administrators | Everyone | All Chats | `/request Inception 2010` |

</details>

<details open>
<summary><b>2. Media Retrieval & Streaming Commands</b></summary>
<br/>

| Command | Aliases | Description | Permissions | Context | Usage Example |
| :--- | :--- | :--- | :---: | :---: | :--- |
| `/getfile` | `/file` | Retrieve and deliver a media file by its unique identifier | Everyone | All Chats | `/getfile 65f...` |
| `/fastdl` | `/link`, `/dl`, `/stream` | Generate an HMAC-signed high-speed HTTP streaming & download link | Everyone | All Chats | `/fastdl 65f...`<br/>*(or reply to media)* |

</details>

<details open>
<summary><b>3. Administrative & Maintenance Commands</b></summary>
<br/>

| Command | Aliases | Description | Permissions | Context | Usage Example |
| :--- | :--- | :--- | :---: | :---: | :--- |
| `/cmdc` | — | Display all commands organized into interactive category menus | **Admin Only** | Private | `/cmdc` |
| `/stats` | `/status` | View detailed database record counts, sharded media counts, and RAM stats | Everyone | All Chats | `/stats` |
| `/vacuum` | — | Reclaim and compact SQLite database and reset WAL journal files | **Admin Only** | Private | `/vacuum` |
| `/set_caption` | `/setcaption` | Set custom media file caption template across all deliveries | **Admin Only** | All Chats | `/set_caption <b>{filename}</b>` |
| `/view_caption` | `/viewcaption` | Preview the active media file caption template | **Admin Only** | All Chats | `/view_caption` |
| `/del_caption` | `/delcaption` | Reset media file caption template to default | **Admin Only** | All Chats | `/del_caption` |

</details>

<details>
<summary><b>4. Channel Indexing Commands</b></summary>
<br/>

| Command | Aliases | Description | Permissions | Context | Usage Example |
| :--- | :--- | :--- | :---: | :---: | :--- |
| `/index` | — | Start indexing a public channel by URL or by forwarding a message in PM | **Admin Only** | Private | `/index https://t.me/c/123/1` |
| `/indexlink` | — | Index a specific range of media posts between start and end message URLs | **Admin Only** | Private | `/indexlink https://t.me/c/123/10 https://t.me/c/123/50` |
| `/setskip` | — | Set message ID offset for the next indexing run | **Admin Only** | Private | `/setskip 500` |

</details>

<details>
<summary><b>5. Force-Subscribe Administration Commands</b></summary>
<br/>

| Command | Description | Permissions | Context | Usage Example |
| :--- | :--- | :---: | :---: | :--- |
| `/sub1`, `/sub2`, `/sub3` | Configure Primary Force-Subscribe Channel 1, 2, or 3 (or forward post) | **Admin Only** | Private | `/sub1 -1001234567890` |
| `/2sub1`, `/2sub2`, `/2sub3` | Configure Secondary Force-Subscribe Channel 1, 2, or 3 (or forward post) | **Admin Only** | Private | `/2sub1 -1009876543210` |
| `/setcount1`, `/setcount2`, `/setcount3` | Set join request threshold count for switching to secondary channels | **Admin Only** | Private | `/setcount1 1000` |
| `/view_sub` | View active FSub channels status and pending join request counts | **Admin Only** | Private | `/view_sub` |
| `/delsub1`, `/delsub2`, `/delsub3` | Remove Primary Force-Subscribe Channel 1, 2, or 3 | **Admin Only** | Private | `/delsub1` |
| `/2delsub1`, `/2delsub2`, `/2delsub3` | Remove Secondary Force-Subscribe Channel 1, 2, or 3 | **Admin Only** | Private | `/2delsub1` |

</details>

<details>
<summary><b>6. Auto-Delete & Weekly Trash Commands</b></summary>
<br/>

| Command | Aliases | Description | Permissions | Context | Usage Example |
| :--- | :--- | :--- | :---: | :---: | :--- |
| `/autodel` | `/autodelete` | View and configure auto-delete timers and weekly trash retention | **Admin Only** | All Chats | `/autodel` |
| `/set_filter_del` | `/autodel_filter` | Set auto-delete duration for filter search results (0 to disable) | **Admin Only** | All Chats | `/set_filter_del 15m` |
| `/set_file_del` | `/autodel_file` | Set auto-delete duration for delivered media files (0 to disable) | **Admin Only** | All Chats | `/set_file_del 30m` |
| `/clean_trash` | `/weekly_trash` | Manually trigger weekly trash cleanup to purge old bot messages | **Admin Only** | All Chats | `/clean_trash` |

</details>

<details>
<summary><b>7. Global Keyword Filter Commands</b></summary>
<br/>

| Command | Aliases | Description | Permissions | Context | Usage Example |
| :--- | :--- | :--- | :---: | :---: | :--- |
| `/gfilter` | `/addgfilter` | Add a global keyword response with text, media, and button rows | **Admin Only** | All Chats | `/gfilter rules\n[Read | https://...]` |
| `/gfilters` | `/allgfilters` | List all active global keyword filters with pagination | **Admin Only** | All Chats | `/gfilters` |
| `/viewgfilter` | `/gfilterinfo` | View details and preview the response of a global filter | **Admin Only** | All Chats | `/viewgfilter rules` |
| `/delg` | `/delgfilter` | Delete a specific global filter keyword | **Admin Only** | All Chats | `/delg rules` |
| `/delallg` | `/cleargfilters` | Delete all global filters with confirmation | **Admin Only** | All Chats | `/delallg` |

</details>

---

## ⚙️ Configuration Guide

All configuration is loaded and validated from environment variables at startup (`internal/config`).

### Essential Environment Variables

```env
# ==============================================================================
# PRIMARY TELEGRAM CREDENTIALS (Required)
# ==============================================================================
BOT_TOKEN=7123456789:AAF1a2B3c4D5e6F7g8H9i0
API_ID=1234567
API_HASH=0123456789abcdef0123456789abcdef
OWNER_ID=123456789
ADMINS=123456789,987654321

# ==============================================================================
# MONITORED MEDIA CHANNELS
# ==============================================================================
CHANNELS=-1001234567890,-1009876543210

# ==============================================================================
# AUTHORITATIVE MONGODB CLUSTERS (Sharded Architecture)
# ==============================================================================
DATABASE_URI=mongodb+srv://user:pass@primary.mongodb.net/?retryWrites=true
DATABASE_NAME=CyberTG
COLLECTION_NAME=Telegram-Document
DATABASE_URI2=mongodb+srv://user:pass@shard1.mongodb.net/?retryWrites=true
DATABASE_URI3=mongodb+srv://user:pass@shard2.mongodb.net/?retryWrites=true

# ==============================================================================
# SQLITE RUNTIME CACHE & SYNC ENGINE
# ==============================================================================
ENABLE_SYNC=true
BACKGROUND_SYNC=true
SYNC_BATCH_SIZE=1500
SQLITE_DB_PATH=data/media.sqlite3

# ==============================================================================
# HTTP STREAMING & FAST DOWNLOAD SERVER
# ==============================================================================
STREAM_SERVER_ENABLED=true
STREAM_URL=https://stream.yourdomain.com
STREAM_PORT=8080
STREAM_BIND_ADDR=0.0.0.0
ENABLE_FAST_DOWNLOAD_BUTTON=true
FAST_DOWNLOAD_LINK_EXPIRY=30m

# ==============================================================================
# AUTO-DELETE & TRASH PURGE
# ==============================================================================
AUTO_DELETE_FILTER_TIME=15m
AUTO_DELETE_FILE_TIME=30m
AUTO_DELETE_WEEKLY_TRASH=true
TRASH_RETENTION_DAYS=7

# ==============================================================================
# VPS RUNTIME TUNING (Prevents VPS OOM-Kills on 1GB RAM)
# ==============================================================================
GOMEMLIMIT=512MiB
GOGC=60
MAX_MEMORY_MB=512
```

> 📖 **Complete Specification**: For the complete reference of all 50+ supported variables (including Telegram Premium emoji IDs and legacy bot tokens), see [docs/CONFIGURATION.md](docs/CONFIGURATION.md).

---

## 🚀 Deployment Guide

### Option 1: Docker (Recommended)

AutoFilter-Go provides a minimal, secure multi-stage Docker build running as an unprivileged user (`botuser` UID `10001`):

```bash
# 1. Clone repository
git clone https://github.com/itsafxal/AutoFilter-Go.git
cd AutoFilter-Go

# 2. Configure environment
cp .env.example .env
nano .env

# 3. Build Docker image
docker build -t autofilter-go:latest .

# 4. Run container with persistent data volume
docker run -d \
  --name autofilter-go \
  --restart unless-stopped \
  --env-file .env \
  -e SESSION_PATH=/app/data/bot.session \
  -e SQLITE_DB_PATH=/app/data/media.sqlite3 \
  -v autofilter_data:/app/data \
  -p 8080:8080 \
  autofilter-go:latest

# 5. Tail container logs
docker logs -f autofilter-go
```

---

### Option 2: Docker Compose

The included `docker-compose.yml` comes preconfigured with resource limits and named persistent volumes:

```bash
# Start in background
docker compose up -d

# View live logs
docker compose logs -f bot

# Update and rebuild
docker compose down
git pull origin main
docker compose build --no-cache
docker compose up -d
```

---

### Option 3: Native Linux VPS (Systemd)

For bare-metal or virtual Linux servers (Ubuntu 22.04 / 24.04 LTS, Debian 12):

```bash
# 1. Install Go 1.25+
curl -OL https://go.dev/dl/go1.27.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.27.0.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# 2. Clone and compile
git clone https://github.com/itsafxal/AutoFilter-Go.git /opt/autofilter-go
cd /opt/autofilter-go
cp .env.example .env
nano .env
go build -trimpath -ldflags="-w -s" -o /opt/autofilter-go/autofilter-go ./cmd/bot

# 3. Setup systemd service
sudo cp docs/DEPLOYMENT.md /tmp/ # Refer to systemd unit file in docs/DEPLOYMENT.md
sudo systemctl daemon-reload
sudo systemctl enable --now autofilter-go
```

> 📖 **Full Instructions**: For Caddy/Nginx reverse proxy configs with automated SSL, see [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md).

---

## 📊 Database Topology & Sharding

```text
┌────────────────────────────────────────────────────────────────────────┐
│                        PRIMARY DATABASE (CyberTG)                      │
│                                                                        │
│  • users: Registered users, names, user IDs, and ban statuses          │
│  • groups: Connected groups, supergroups, and enabled/disabled states   │
│  • admins: Force Subscribe loadouts, channel thresholds, captions      │
│  • reqone / reqtwo / reqthree: Pending join requests for FSub channels  │
│  • gfilters: Global filter keywords, response templates, button rows   │
└────────────────────────────────────┬───────────────────────────────────┘
                                     │
           ┌─────────────────────────┴─────────────────────────┐
           ▼                                                   ▼
┌──────────────────────────────────┐        ┌──────────────────────────────────┐
│   SECONDARY CLUSTER: DB2         │        │    TERTIARY CLUSTER: DB3         │
│   (DATABASE_URI2)                │        │    (DATABASE_URI3)               │
│                                  │        │                                  │
│  • Telegram-Document (Shard A)   │        │  • Telegram-Document (Shard B)   │
│    File ID, File Ref, File Name, │        │    File ID, File Ref, File Name, │
│    File Size, Caption, File Type │        │    File Size, Caption, File Type │
└──────────────────────────────────┘        └──────────────────────────────────┘
```

1. **Isolation of State**: Volatile operational data (user accounts, bans, FSub channels) lives entirely on the primary cluster without being slowed down by media cursors.
2. **Round-Robin Media Storage**: Media documents from monitored channels alternate sequentially between `DB2` and `DB3`.
3. **Pure-Go SQLite Runtime Index**: During startup, media metadata is indexed into a local SQLite database in WAL mode (`media.sqlite3`), enabling `<0.1ms` deduplication checks and instantaneous search lookups.
4. **Zero-Sync Mode (`ENABLE_SYNC=false`)**: When deployed on ultra-low-memory VPS servers (512MB RAM), setting `ENABLE_SYNC=false` boots the bot in 1 second and routes queries directly to MongoDB with an idle footprint of only **~50 MB RAM**.

> 📖 **Deep Dive**: For schema definitions and sync internals, see [docs/DATABASE.md](docs/DATABASE.md).

---

## 🧩 Compile-Time Plugin System

AutoFilter-Go uses a compile-time plugin registry (`internal/plugins`):
- **Zero Runtime Reflection**: Strong typing with direct event dispatching.
- **Dependency Injection**: Every plugin receives `*plugins.AppContext` containing database managers, config, loggers, and services.

### Adding a Custom Plugin in 3 Steps:

1. Create a package in `plugins/myplugin/myplugin.go`:
```go
package myplugin

import (
    "github.com/amarnathcjd/gogram/telegram"
    "github.com/itsafxal/AutoFilter-Go/internal/plugins"
)

type Plugin struct {
    appCtx *plugins.AppContext
}

func New(appCtx *plugins.AppContext) *Plugin {
    return &Plugin{appCtx: appCtx}
}

func (p *Plugin) Name() string { return "myplugin" }

func (p *Plugin) Commands() []plugins.CommandDefinition {
    return []plugins.CommandDefinition{
        {
            Name:        "hello",
            Description: "Returns a friendly greeting",
            Usage:       "/hello",
            Category:    "General",
            Handler: func(m *telegram.NewMessage) error {
                _, err := m.Reply("👋 Hello from AutoFilter-Go!")
                return err
            },
        },
    }
}
```

2. Register the plugin in `cmd/bot/main.go`:
```go
application.RegisterPlugins(
    // ... existing plugins ...
    myplugin.New(application.AppContext),
)
```

3. Build and test:
```bash
go vet ./...
go build -o ./bin/autofilter-go ./cmd/bot
```

> 📖 **Plugin Guide**: For callback handlers, channel filters, and stateful sessions, see [docs/PLUGINS.md](docs/PLUGINS.md).

---

## 🧪 Development, Testing & Verification

AutoFilter-Go contains comprehensive unit and integration tests.

### Run Verification Suite
```bash
# 1. Format code
gofmt -w .

# 2. Run static analysis
go vet ./...

# 3. Execute unit tests
go test -v ./internal/config ./internal/commands ./internal/delivery ./plugins/cmds ./plugins/ping

# 4. Build standalone binary
go build -trimpath -ldflags="-w -s" -o ./bin/autofilter-go ./cmd/bot
```

> 📖 **Troubleshooting**: Having issues with FloodWait, MongoDB timeouts, or port collisions? Check [docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md).

---

## 📚 Documentation Directory

Explore the dedicated documentation in the [`docs/`](docs/) directory:

- 📐 **[System Architecture](docs/ARCHITECTURE.md)** — Architectural layers, MTProto networking, and data flows.
- ⚙️ **[Configuration Reference](docs/CONFIGURATION.md)** — Exhaustive guide for all 50+ environment variables.
- 🚀 **[Deployment Manual](docs/DEPLOYMENT.md)** — Guides for Docker, Docker Compose, Linux systemd, and Nginx.
- 🗄️ **[Database Architecture](docs/DATABASE.md)** — Multi-cluster MongoDB, SQLite WAL engine, and sync modes.
- 🧩 **[Plugin Developer Guide](docs/PLUGINS.md)** — How to write and register custom plugins.
- 🔧 **[Troubleshooting Guide](docs/TROUBLESHOOTING.md)** — Solutions for common deployment and runtime issues.
- 📜 **[Migration & Verification Reports](docs/)** — Historical development and validation reports.

---

## 🤝 Contributing

Contributions are welcome! Please feel free to open issues or submit pull requests.
1. Fork the repository.
2. Create your feature branch (`git checkout -b feature/AmazingFeature`).
3. Commit your changes (`git commit -m 'Add some AmazingFeature'`).
4. Ensure tests and formatting pass (`gofmt -w . && go vet ./...`).
5. Push to the branch (`git push origin feature/AmazingFeature`).
6. Open a Pull Request.

---

## 📄 License

This project is licensed under the MIT License — see the [LICENSE](LICENSE) file for details.

<div align="center">
  <sub>Engineered with ❤️ for the high-performance Go and Telegram open-source communities.</sub>
</div>
