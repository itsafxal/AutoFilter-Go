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
| **Named Batch System** | Dual-persistence batch collector with exact sequence ordering, sub-millisecond SQLite search, startup reconciliation, and deep-link delivery (`/batch`, `/batchsearch`, `/batchlist`). | [`plugins/batch`](plugins/batch/batch.go) |
| **Fast Broadcast Engine** | Concurrent worker pool (`BROADCAST_CONCURRENCY`) with cursor streaming, dynamic progress bar, FloodWait thread-pause, and automatic cleanup of deactivated/blocked users. | [`internal/broadcast`](internal/broadcast/service.go) |
| **Bot Lifecycle & Welcome** | Automated group registration, rich text-only welcome banner, configurable auto-delete, admin DM addition/removal alerts, and `/leave`, `/disable`, `/enable` controls. | [`plugins/lifecycle`](plugins/lifecycle/lifecycle.go) |
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
        SQLite[("Pure-Go SQLite Cache<br/>WAL Mode - Cache 64MB<br/>Sub-millisecond Search")]
        MongoPrimary[("Primary MongoDB<br/>Users, Groups, FSub, Captions")]
        MongoDB2[("Secondary MongoDB DB2<br/>Media Shard A")]
        MongoDB3[("Tertiary MongoDB DB3<br/>Media Shard B")]
    end

    Gogram --> Registry
    Registry --> Gogram
    OldBots -->|Deep-Link Funneling| Gogram
    StreamServer -->|Byte-Range Streaming| Gogram

    Registry --> SearchSvc
    Registry --> DeliverySvc
    Registry --> FSubSvc
    Registry --> AutoDelSvc

    SearchSvc -->|Indexed Search| SQLite
    SearchSvc -.->|Direct Fallback| MongoDB2
    SearchSvc -.->|Direct Fallback| MongoDB3

    DeliverySvc --> InFlight
    InFlight -->|1. Check Cache| SQLite
    InFlight -->|2. Fallback Fetch| MongoDB2
    InFlight -->|2. Fallback Fetch| MongoDB3

    FSubSvc --> MongoPrimary
    AutoDelSvc --> SQLite
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
│   ├── batch/                   # Named Batch dual persistence, exact ordering & sync engine
│   ├── broadcast/               # High-performance concurrent asynchronous broadcast engine
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
│   ├── batch/                   # Interactive batch creation, search, and list (/batch, /batchsearch, /batchlist)
│   ├── botstats/                # /stats reporting and /vacuum database compaction
│   ├── broadcast/               # Admin broadcast commands (/broadcast, /cancel_broadcast)
│   ├── caption/                 # /set_caption, /view_caption, and /del_caption
│   ├── channel/                 # Monitored channel live media indexing watcher
│   ├── cmds/                    # Dynamic /cmds guide and categorized /cmdc menus
│   ├── delivery/                # /getfile handler and media callback processor
│   ├── fsub/                    # Force-Subscribe administrative configuration commands
│   ├── gfilter/                 # Global filter management commands (/gfilter, /gfilters)
│   ├── index/                   # /index, /indexlink, and /setskip channel indexer
│   ├── info/                    # User, group & chat info system (/info, /about)
│   ├── latest/                  # /latest movie releases and user /request command
│   ├── lifecycle/               # Group welcome, admin alerts, /leave, /disable, /enable
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

All **53 commands** are compiled into the binary, verified at startup, and protected by role-based access controls (`internal/plugins`). Use `/cmd` in Telegram to launch the interactive command browser with dynamic category navigation.

<details open>
<summary><b>1. General Commands (6)</b></summary>
<br/>

| Command | Aliases | Description | Permissions | Context | Usage Example |
| :--- | :--- | :--- | :---: | :---: | :--- |
| `/start` | — | Start the bot, receive welcome message, or process media/batch deep-links | Everyone | All Chats | `/start`<br/>`/start file_12345`<br/>`/start batch_b_9b2e` |
| `/cmds` | `/help`, `/cmd` | Open the interactive categorized command center and usage guide | Everyone | All Chats | `/cmd` |
| `/ping` | — | Measure bot processing latency, gateway heartbeat, and MTProto round-trip time | Everyone | All Chats | `/ping` |
| `/stats` | `/status` | View detailed database record counts, sharded media counts, and RAM stats | Everyone | All Chats | `/stats` |
| `/latest` | — | Browse recently added and indexed movie and series files | Everyone | All Chats | `/latest` |
| `/request` | `/req` | Submit a movie or series title request to the channel administrators | Everyone | All Chats | `/request Inception 2010` |

</details>

<details open>
<summary><b>2. User Commands (1)</b></summary>
<br/>

| Command | Aliases | Description | Permissions | Context | Usage Example |
| :--- | :--- | :--- | :---: | :---: | :--- |
| `/info` | `/about` | Retrieve comprehensive user, group, and Telegram profile information with DC stats and profile photos | Everyone | All Chats | `/info`<br/>`/info @username`<br/>`/info 123456789`<br/>*(or reply to any message)* |

</details>

<details open>
<summary><b>3. File & Media Commands (2)</b></summary>
<br/>

| Command | Aliases | Description | Permissions | Context | Usage Example |
| :--- | :--- | :--- | :---: | :---: | :--- |
| `/getfile` | `/file` | Retrieve and deliver a media file by its unique identifier | Everyone | All Chats | `/getfile 65f...` |
| `/fastdl` | `/link`, `/dl`, `/fastdownload`, `/stream` | Generate an HMAC-signed high-speed HTTP streaming & download link | Everyone | All Chats | `/fastdl 65f...`<br/>*(or reply to media)* |

</details>

<details open>
<summary><b>4. FCB Commands — Force-Subscribe Controls (16)</b></summary>
<br/>

| Command | Aliases | Description | Permissions | Context | Usage Example |
| :--- | :--- | :--- | :---: | :---: | :--- |
| `/sub1` | — | Configure Primary Force-Subscribe Channel 1 (or forward post) | **Admin Only** | Private | `/sub1 -1001234567890` |
| `/sub2` | — | Configure Primary Force-Subscribe Channel 2 (or forward post) | **Admin Only** | Private | `/sub2 -1001234567890` |
| `/sub3` | — | Configure Primary Force-Subscribe Channel 3 (or forward post) | **Admin Only** | Private | `/sub3 -1001234567890` |
| `/2sub1` | — | Configure Secondary Force-Subscribe Channel 1 (or forward post) | **Admin Only** | Private | `/2sub1 -1009876543210` |
| `/2sub2` | — | Configure Secondary Force-Subscribe Channel 2 (or forward post) | **Admin Only** | Private | `/2sub2 -1009876543210` |
| `/2sub3` | — | Configure Secondary Force-Subscribe Channel 3 (or forward post) | **Admin Only** | Private | `/2sub3 -1009876543210` |
| `/setcount1` | — | Set join request threshold count for switching Channel 1 to secondary | **Admin Only** | Private | `/setcount1 1000` |
| `/setcount2` | — | Set join request threshold count for switching Channel 2 to secondary | **Admin Only** | Private | `/setcount2 1000` |
| `/setcount3` | — | Set join request threshold count for switching Channel 3 to secondary | **Admin Only** | Private | `/setcount3 1000` |
| `/view_sub` | — | View active FSub channels status, channel IDs, and pending join request counts | **Admin Only** | Private | `/view_sub` |
| `/delsub1` | — | Remove Primary Force-Subscribe Channel 1 | **Admin Only** | Private | `/delsub1` |
| `/delsub2` | — | Remove Primary Force-Subscribe Channel 2 | **Admin Only** | Private | `/delsub2` |
| `/delsub3` | — | Remove Primary Force-Subscribe Channel 3 | **Admin Only** | Private | `/delsub3` |
| `/2delsub1` | — | Remove Secondary Force-Subscribe Channel 1 | **Admin Only** | Private | `/2delsub1` |
| `/2delsub2` | — | Remove Secondary Force-Subscribe Channel 2 | **Admin Only** | Private | `/2delsub2` |
| `/2delsub3` | — | Remove Secondary Force-Subscribe Channel 3 | **Admin Only** | Private | `/2delsub3` |

</details>

<details open>
<summary><b>5. Batch Management Commands (3)</b></summary>
<br/>

| Command | Aliases | Description | Permissions | Context | Usage Example |
| :--- | :--- | :--- | :---: | :---: | :--- |
| `/batch` | `/addbatch`, `/newbatch` | Create a new named batch of files, media, and text in strict sequential order | Everyone | Private | `/batch`<br/>`/batch Marvel Phase 4` |
| `/batchsearch` | `/searchbatch`, `/findbatch` | Search saved batches by name with sub-millisecond SQLite queries | Everyone | All Chats | `/batchsearch Marvel` |
| `/batchlist` | `/batches`, `/mybatches` | Display a paginated list of all saved batches with inline navigation | Everyone | All Chats | `/batchlist` |

</details>

<details open>
<summary><b>6. Global Filter Commands (5)</b></summary>
<br/>

| Command | Aliases | Description | Permissions | Context | Usage Example |
| :--- | :--- | :--- | :---: | :---: | :--- |
| `/gfilter` | `/addgfilter` | Add a global keyword response with text, media, and button rows | **Admin Only** | All Chats | `/gfilter rules\n[Read | https://...]` |
| `/gfilters` | `/allgfilters` | List all active global keyword filters with pagination | **Admin Only** | All Chats | `/gfilters` |
| `/viewgfilter` | `/gfilterinfo` | View details and preview the formatted response of a global filter | **Admin Only** | All Chats | `/viewgfilter rules` |
| `/delg` | `/delgfilter` | Delete a specific global filter keyword from MongoDB and SQLite | **Admin Only** | All Chats | `/delg rules` |
| `/delallg` | `/cleargfilters` | Purge all global filters across the bot with confirmation | **Admin Only** | All Chats | `/delallg` |

</details>

<details open>
<summary><b>7. Indexing Commands (3)</b></summary>
<br/>

| Command | Aliases | Description | Permissions | Context | Usage Example |
| :--- | :--- | :--- | :---: | :---: | :--- |
| `/index` | — | Start indexing a public channel by URL or by forwarding a message in PM | **Admin Only** | Private | `/index https://t.me/c/123/1` |
| `/indexlink` | — | Index a specific range of media posts between start and end message URLs | **Admin Only** | Private | `/indexlink https://t.me/c/123/10 https://t.me/c/123/50` |
| `/setskip` | — | Set message ID offset for the next indexing run | **Admin Only** | Private | `/setskip 500` |

</details>

<details open>
<summary><b>8. Group Management Commands (4)</b></summary>
<br/>

| Command | Aliases | Description | Permissions | Context | Usage Example |
| :--- | :--- | :--- | :---: | :---: | :--- |
| `/groups` | `/chats` | View total registered, active, and disabled connected group counts | **Admin Only** | Private | `/groups` |
| `/leave` | `/leave_chat` | Instruct the bot to leave the current group or a specified chat ID | **Admin Only** | All Chats | `/leave`<br/>`/leave -1001234567890` |
| `/disable` | `/disable_chat` | Disable search and media delivery in the current group or specified chat ID | **Admin Only** | All Chats | `/disable -1001234567890 Violation` |
| `/enable` | `/enable_chat`, `/re_enable` | Re-enable search and media delivery in the current group or specified chat ID | **Admin Only** | All Chats | `/enable -1001234567890` |

</details>

<details open>
<summary><b>9. Admin Commands (12)</b></summary>
<br/>

| Command | Aliases | Description | Permissions | Context | Usage Example |
| :--- | :--- | :--- | :---: | :---: | :--- |
| `/cmdc` | — | Open the admin command master center directly | **Admin Only** | Private | `/cmdc` |
| `/broadcast` | — | Broadcast replied message concurrently to registered users/groups with copy/forward modes and offset skipping | **Admin Only** | Private | `/broadcast -users -copy`<br/>`/broadcast -all -forward`<br/>`/broadcast -skip=50` |
| `/broadcast_groups` | — | Broadcast replied message specifically to all registered groups | **Admin Only** | Private | `/broadcast_groups -copy` |
| `/cancel_broadcast` | `/stop_broadcast` | Abort and gracefully terminate an active broadcast worker pool | **Admin Only** | Private | `/cancel_broadcast` |
| `/broadcast_stats` | — | Inspect active broadcast worker pool status and real-time delivery counts | **Admin Only** | Private | `/broadcast_stats` |
| `/set_caption` | `/setcaption` | Set custom media file caption template across all deliveries | **Admin Only** | All Chats | `/set_caption <b>{filename}</b>` |
| `/view_caption` | `/viewcaption` | Preview the active media file caption template | **Admin Only** | All Chats | `/view_caption` |
| `/del_caption` | `/delcaption` | Reset media file caption template to system default | **Admin Only** | All Chats | `/del_caption` |
| `/autodel` | `/autodelete` | View and configure auto-delete timers and weekly trash retention | **Admin Only** | All Chats | `/autodel` |
| `/set_filter_del` | `/autodel_filter` | Set auto-delete duration for filter search results (0 to disable) | **Admin Only** | All Chats | `/set_filter_del 15m` |
| `/set_file_del` | `/autodel_file` | Set auto-delete duration for delivered media files (0 to disable) | **Admin Only** | All Chats | `/set_file_del 30m` |
| `/clean_trash` | `/weekly_trash` | Manually trigger weekly trash cleanup to purge old bot messages | **Admin Only** | All Chats | `/clean_trash` |

</details>

<details open>
<summary><b>10. Developer / Owner Commands (1)</b></summary>
<br/>

| Command | Aliases | Description | Permissions | Context | Usage Example |
| :--- | :--- | :--- | :---: | :---: | :--- |
| `/vacuum` | — | Reclaim and compact SQLite database, rebuild indices, and reset WAL journal | **Owner / Admin** | Private | `/vacuum` |

</details>

---

### 📢 Fast Broadcast System

AutoFilter-Go incorporates a high-performance, asynchronous broadcast engine ([`internal/broadcast`](internal/broadcast/service.go), [`plugins/broadcast`](plugins/broadcast/broadcast.go)) engineered to broadcast announcements, media, and updates to thousands of users and groups concurrently without exceeding Telegram rate limits or exhausting system RAM.

#### 1. System Overview & Bounded Concurrency
- **Asynchronous Worker Pool**: Delivery is driven by a concurrent worker pool bounded by `BROADCAST_CONCURRENCY` (default: `10`, range: `1`–`50`). Goroutines concurrently pull recipient IDs from a shared producer channel.
- **Streaming Database Cursors**: Recipient IDs are streamed on demand directly from MongoDB via `col.Find` with ID-only projections (`_id`) into a bounded buffer of capacity `2 * concurrency`. The complete recipient collection is **never** loaded into memory, guaranteeing $O(\text{concurrency})$ RAM usage even with millions of registered users.
- **Atomic Single-Broadcast Lock**: An atomic state guard prevents concurrent conflicting broadcasts. If an admin attempts to run `/broadcast` while another broadcast is active, the engine rejects the request with an informative warning and current execution status.

#### 2. Supported Recipient Types
- **Registered Users** (`-users` / `-u`): Targets individual users who have started or interacted with the bot.
- **Registered Groups** (`-groups` / `-g`): Targets supergroups and groups where the bot is installed.
- **All Recipients** (`-all` / `-a`): Sequentially streams and delivers to all registered users followed by all registered groups.

#### 3. Supported Message Types & Entity Preservation
- **All Media Types Supported**: Text messages, photos, videos, audio files, documents, animations (GIFs), voice notes, video notes, and stickers.
- **Entity & Formatting Integrity**: Preserves all formatting entities including HTML/Markdown bold, italics, underline, strikethrough, spoiler, inline code blocks, hyperlinks, and Telegram Premium custom emoji entities.
- **Delivery Modes**:
  - **Copy Mode** (`-copy` / `-c` or default): Copies the message without forwarding attribution (`HideAuthor: true`), appearing as an original post from the bot.
  - **Forward Mode** (`-forward` / `-f`): Preserves the original sender's forwarding credits (`HideAuthor: false`).

#### 4. Permissions & Role-Based Access Control
- Broadcast execution, progress inspection, and cancellation commands are strictly **restricted to Bot Admins and the Bot Owner** (`ADMIN_USERS` / `AUTH_CHANNEL`).
- Unauthenticated execution attempts are immediately rejected without performing database queries.

#### 5. Command Syntax & Options

| Command | Target | Default Mode | Description |
| :--- | :--- | :--- | :--- |
| `/broadcast [flags]` | Users | Copy | Broadcast replied message to registered users or all targets |
| `/broadcast_groups [flags]` | Groups | Copy | Broadcast replied message to all connected groups |
| `/cancel_broadcast` | Active Task | — | Gracefully terminate active broadcast workers |
| `/broadcast_stats` | Active Task | — | Inspect current live broadcast delivery counts and throughput |

**Supported Flags & Arguments**:
- `-users` / `-u`: Target all registered users.
- `-groups` / `-g`: Target all registered groups.
- `-all` / `-a`: Target both users and groups.
- `-copy`: Deliver as clean copy (author tag hidden).
- `-forward`: Deliver with original forward attribution header.
- `-skip=N` / `<number>`: Skip the first `N` recipients from the database (ideal for resuming interrupted tasks).
- `-c=N`: Set a custom worker concurrency limit for this specific broadcast run (capped at `50`).

#### 6. Step-by-Step Practical Usage Example

1. Send or forward the announcement message, photo, video, or document into the bot's private chat.
2. Reply directly to that message with the command and desired flags:
   ```text
   /broadcast -copy
   ```
   *To broadcast to all registered groups:*
   ```text
   /broadcast_groups -copy
   ```
   *To broadcast to both users and groups, skipping the first 250 recipients:*
   ```text
   /broadcast -all -copy -skip=250
   ```

#### 7. Live Throttled Progress Monitoring
Rather than editing progress after every single recipient (which triggers Telegram rate limits), the engine updates the status message at throttled intervals of **3.5 seconds**:

```text
📢 Broadcasting in progress...

[████████████░░░░░░░░] 60.5%

• Total Recipients: 15,200
• Successfully Sent: 9,196
• Failed: 18
• Blocked Users: 124
• Deactivated Users: 14
• Speed: 29.8 msg/s
• Elapsed Time: 5m 08s
• Estimated Remaining: 3m 21s

Press the button below or send /cancel_broadcast to stop.
[ 🛑 Cancel Broadcast ]
```

#### 8. Broadcast Cancellation
Broadcasting can be halted at any moment:
- By clicking the inline **`[ 🛑 Cancel Broadcast ]`** button attached to the progress message.
- By sending `/cancel_broadcast` (or alias `/stop_broadcast`).
The engine cancels the worker pool context, shuts down cursor streaming, waits for active workers to complete in-flight requests, and publishes final delivery statistics.

#### 9. Rate Limit & FloodWait Coordination
- **Thread-Safe FloodWait Barrier**: When Telegram issues `FLOOD_WAIT_X`, the affected worker detects the pause duration, acquires the global flood coordinator, and pauses **all** broadcast workers for `X + 1` seconds.
- **Exponential Retry Backoff**: Temporary network failures or rate limits automatically re-queue the target recipient up to 3 times with exponential backoff before recording a failure.

#### 10. Automatic Database Cleanup & Error Handling
- **Deactivated Accounts** (`InputUserDeactivated` / `UserDeactivated`): Identified users are flagged as deactivated and automatically deleted from the MongoDB `users` collection to keep the database healthy.
- **Blocked Users** (`UserIsBlocked` / `BotBlocked`): Increments the blocked counter and ceases delivery attempts to the peer.
- **Forbidden Chats** (`ChatWriteForbidden` / `ChannelPrivate`): Groups where the bot has been removed or restricted are automatically pruned from the MongoDB `groups` collection.
- **Fault-Isolated Execution**: Individual recipient failures or invalid peer IDs never interrupt or terminate the overall broadcast job.

---

### 👤 User Information System

AutoFilter-Go provides a modern, unified information system ([`plugins/info`](plugins/info/info.go)) that consolidates all user, chat, group, and identifier inspection into a single, comprehensive command: `/info`. The redundant `/id` command has been fully retired and its functionality integrated into `/info`.

#### 1. Command Overview & Lookup Methods
The `/info` command supports three flexible lookup strategies with automated input validation:
- **Direct User Lookup**: Send `/info` in private chat to inspect your own profile.
- **Reply-Based Lookup**: Reply to any message in a private chat or group with `/info` to inspect the author.
- **Username Lookup**: Run `/info @username` to resolve and inspect any public Telegram user or channel.
- **User ID Lookup**: Run `/info <user_id>` (e.g. `/info 123456789`) to inspect any registered or cached Telegram user.

#### 2. Supported User Information Fields
The generated HTML card presents complete, verified user metadata:
- **User Identity**: Full Name (escaped against XSS/HTML injections), Username (or *Not Set*), Telegram User ID (`<code>%d</code>`), and Direct Profile Link.
- **Account Badges & Attributes**:
  - `Bot`: `Yes` / `No`
  - `Premium`: `Yes` / `No`
  - `Verified`: `Yes` / `No`
  - `Scam`: `Yes` / `No`
  - `Fake`: `Yes` / `No`
  - `Restricted`: `Yes` / `No` (with active restriction reasons when exposed)
  - `Language`: Language code (e.g. `en`) or `Not Available`
- **Activity & Last Seen**: Dynamically parsed from `UserStatus`:
  - `Online` (with expiration)
  - `Offline` (with exact UTC timestamp)
  - `Recently`, `Within a week`, `Within a month`, or `Hidden / Unknown`
- **Deleted Accounts**: Gracefully identified and rendered with dedicated deleted account status.

#### 3. Group & Supergroup Context
When executed in a group or supergroup, `/info` automatically enriches the card with group-specific context:
- **Group Identifiers**: Current Group Chat ID (`<code>-100...</code>`) and Chat Title.
- **Membership Status**: Real-time permission level (`Owner / Creator`, `Administrator`, `Member`, `Restricted`, or `Banned`).
- **Custom Admin Rank**: Displays custom admin titles (e.g. `Administrator (Moderator)`).
- **Group Join Date**: Formatted join date timestamp (`DD Mon YYYY, HH:MM UTC`) when accessible.
- **Replied Message ID**: Tracks the specific message ID when used as a reply.
- **Anonymous Admins & Channels**: Messages sent by anonymous administrators or linked channels render an adapted Channel Card displaying the channel title, username, channel ID, and group context without errors.

#### 4. Profile Photo Integration & Fast Delivery
- **High-Resolution Photo Preview**: Automatically fetches the user's latest and largest profile photo via `PhotosGetUserPhotos`.
- **Zero-Disk Direct MTProto Delivery**: Attempts direct `InputMediaPhoto` delivery first, eliminating disk I/O and bandwidth overhead.
- **Graceful Fallback**: If direct delivery fails, downloads the photo to a secure temporary path, uploads it as media, and immediately purges the temporary file via `defer os.Remove(...)`.
- **Clean Text Fallback**: If the user has no profile photo, privacy restrictions prevent access, or download fails, falls back cleanly to a text-based card.

#### 5. Telegram Data Centre (DC) Information
AutoFilter-Go strictly avoids guessing or fabricating DC IDs:
- **Profile Photo DC**: Extracted directly from profile photo file metadata (`userPhoto.DcID()`), explicitly identified as `📍 Profile Photo DC: DC <X>`.
- **Bot Session DC**: Extracted from the bot's active MTProto connection (`client.GetDC()`), explicitly identified as `📍 Bot Session DC: DC <Y>`.
- **Integrity**: Account DCs are never assumed to match profile photo DCs; network fields are omitted when not reliably exposed.

#### 6. Interactive Inline Buttons
Every information card includes interactive, actionable inline buttons:
- **`[ 🔗 Open Profile ]`**: Direct link to open the user's profile (`https://t.me/<username>` or `tg://user?id=<user_id>`).
- **`[ 📋 Copy ID ]`**: Utilizes native Telegram `KeyboardButtonCopy` to copy the numerical ID directly to the user's clipboard in one click.
- **`[ ❌ Close ]`**: Dismisses and deletes the information card. Protected by authorization checks so only the requesting user, bot owner, or bot administrators can close it.

#### 7. Example Commands

```text
# Inspect your own profile in private chat
/info

# Inspect a user by replying to their message
/info

# Inspect any public user or channel by username
/info @durov

# Inspect a user by numerical ID
/info 777000
```

---

### 🎉 Bot Welcome & Lifecycle System

AutoFilter-Go includes an automated, reliable Bot Lifecycle and Notification Engine ([`plugins/lifecycle`](plugins/lifecycle/lifecycle.go)) that manages group membership events, welcomes new communities, registers users and groups into MongoDB, and notifies administrators of key lifecycle changes.

#### 1. Automatic Group Registration
- **Event Detection**: Seamlessly detects when the bot is added to supergroups/channels (`ParticipantUpdate`) or basic groups (`MessageActionChatAddUser`).
- **Idempotent Persistence**: Automatically registers new groups into MongoDB (`db.groups`) with non-duplicate keys (`IsGroupExist`). If a previously disabled group re-adds the bot, services are automatically re-enabled.

#### 2. Group Welcome Banner
When the bot is added to a new group, it posts a professional text-only welcome message (no external image assets required):

```text
🎉 BOT SUCCESSFULLY ADDED!

━━━━━━━━━━━━━━━━━━

👋 Hello, everyone!

Thanks for adding @AutoFilterBot to your group.

📌 GROUP INFORMATION

🏷 Group: Movie Buffs Community
🆔 Group ID: -1001234567890

👤 ADDED BY

📝 Name: Alice Smith
🆔 ID: 987654321

━━━━━━━━━━━━━━━━━━

✨ Your bot is now ready to use!

Use the buttons below to explore more.
```

- **Interactive Inline Buttons**:
  - `[ 🤖 @AutoFilterBot ]`: Direct link to open the bot with `/start`.
  - `[ 💻 Source Code ]`: Official link to the repository (`REPO_URL`).
- **Configurable Auto-Deletion**: Welcome messages can be automatically deleted after a configured interval (`WELCOME_DELETE_DELAY=5m`) to keep the group chat pristine.

#### 3. Administrator DM Notifications
- **Addition Alert (`#NEW_GROUP_ADDED`)**: Whenever the bot is added to a group, all configured administrators (`ADMINS` / `OWNER_ID`) receive an instant DM containing the group's title, ID, public username/link, adder identity, addition timestamp, and bot handle.
- **Removal Alert (`#BOT_REMOVED_FROM_GROUP`)**: When the bot is removed or banned from a group, administrators receive an alert noting the group title, ID, responsible admin, and timestamp.
- **Fail-Safe Dispatch**: DM notifications are delivered concurrently in background goroutines. If an administrator has blocked the bot or not started it, the error is safely ignored without interrupting notifications to other administrators.

#### 4. New User Registration
When a user launches `/start` in private chat:
- The system checks `db.is_user_exist(user_id)`.
- Genuinely new users are recorded in the authoritative MongoDB `users` collection.
- Duplicate database writes are prevented on subsequent interactions.

#### 5. Group Moderation & Administrative Commands
Authorized administrators can manage connected groups remotely:

| Command | Usage | Description |
| :--- | :--- | :--- |
| `/leave` | `/leave [chat_id]` | Instruct the bot to leave the current group or a target chat ID |
| `/disable` | `/disable [chat_id] [reason]` | Disable search and media delivery in a specific group |
| `/enable` | `/enable [chat_id]` | Re-enable search and media delivery in a disabled group |
| `/groups` | `/groups` | View total connected groups, active groups, and disabled groups |

#### 6. Configuration Options

| Variable | Default | Description |
| :--- | :---: | :--- |
| `ENABLE_GROUP_WELCOME` | `true` | Toggle automatic group welcome message upon addition |
| `WELCOME_DELETE_DELAY` | `0` *(disabled)* | Duration before welcome banner is auto-deleted (e.g. `5m`, `15m`, `1h`) |
| `REPO_URL` | `https://github.com/itsafxal/AutoFilter-Go` | Official repository link bound to welcome buttons |

---

### 📦 Named Batch Management System

AutoFilter-Go features an enterprise-grade **Named Batch Management System** ([`plugins/batch`](plugins/batch/batch.go), [`internal/batch`](internal/batch/service.go)) that allows users to assemble, organize, and share sequential collections of heterogeneous content (movies, series episodes, music albums, documents, voice notes, and text messages) through a single shareable Telegram deep link.

The Batch System implements the **exact same dual-persistence and startup synchronization lifecycle** as Global Filters:
- **Authoritative Cloud Persistence**: Batch headers and sequential items are stored in primary MongoDB (`DATABASE_URI`) within `batch_metadata` and `batch_items` collections.
- **Sub-Millisecond Runtime Caching**: All finalized batches and ordered items are indexed locally in the shared `data/media.sqlite3` database for instantaneous searches and zero-latency deep-link resolution.
- **Idempotent Startup Synchronization**: Upon bot startup, the coordinator validates MongoDB records, synchronizes them into SQLite, reconciles missing or stale rows, and validates sequential ordering before accepting user requests.

---

#### 1. Interactive Batch Creation (`/batch`)

Users can initiate a batch session in bot private chat using two workflows:

1. **Interactive Prompt**: Run `/batch` without arguments. The bot prompts for a collection title:
   ```text
   🏷️ Please send a name for your new Batch:
   Example: Marvel Cinematic Universe (Phase 1-4)
   ```
2. **Direct Shortcut**: Run `/batch <name>`:
   ```text
   /batch Christopher Nolan Filmography [1080p HEVC]
   ```

Upon creation, the bot displays a live progress status card with interactive inline buttons:

```text
📦 BATCH: Christopher Nolan Filmography [1080p HEVC]
━━━━━━━━━━━━━━━━━━

📁 Files: 0
🎬 Videos: 0
🎵 Audio: 0
🖼 Photos: 0
📝 Texts: 0

📊 Total Items: 0

🟢 Status: Collecting

━━━━━━━━━━━━━━━━━━
Send files, media, or text messages now. When finished, press DONE below.

[ ✅ DONE ]   [ ❌ CANCEL ]
```

---

#### 2. Supported Content & Strict Sequence Ordering

While the batch session is active, the bot listens to all incoming messages in private chat:

| Content Type | Attributes Extracted & Preserved |
| :--- | :--- |
| **Videos** | Video file, file ID, MIME type, dimensions, file size, original caption |
| **Documents** | Generic files, PDFs, archives, file name, file size, custom caption |
| **Audio / Music** | Track file ID, artist, title, duration, performer, caption |
| **Voice Notes** | Voice audio file ID, duration, waveform, caption |
| **Photos** | Photo file ID, dimensions, original caption |
| **Animations / GIFs** | GIF file ID, animation attributes, caption |
| **Plain Text** | Formatted text message, HTML entities, clickable links |

**Strict Order Preservation**: Every item receives a unique, concurrency-safe sequential index ($1, 2, 3 \dots$). The system guarantees that items will never be sorted alphabetically or reordered by type—they are strictly preserved and delivered in the exact order they were sent.

---

#### 3. Finalization (`DONE`) & Cancellation (`CANCEL`)

- **`[ ✅ DONE ]`**:
  1. Validates session ownership (only the batch creator can finalize).
  2. Ensures the batch contains at least 1 item.
  3. Authoritatively persists metadata and ordered items into MongoDB.
  4. Synchronizes records into local SQLite runtime tables.
  5. Automatically deletes temporary incoming upload messages to keep user chat clean.
  6. Generates a shareable deep link and renders the final summary card.
- **`[ ❌ CANCEL ]`**:
  1. Immediately halts active session collection.
  2. Purges temporary upload messages.
  3. Ensures no unfinalized records pollute production search indices.

---

#### 4. Shareable Deep-Link Retrieval

Every finalized batch is accessible via a unique Telegram deep link:

$$\text{https://t.me/}\langle\text{BOT\_USERNAME}\rangle\text{?start=batch\_}\langle\text{BATCH\_ID}\rangle$$

*Example*: `https://t.me/AutoFilterBot?start=batch_b_9b2e7a4f10cd`

When a user opens the deep link:
1. **Security & Force-Subscribe**: Evaluates user ban status and validates multi-tier Force-Subscribe channel requirements.
2. **Sub-Millisecond Resolution**: Fetches batch header and ordered item references from local SQLite (<0.1ms), falling back to authoritative MongoDB if cache is missing.
3. **Sequential Dispatch**: Iterates through items in exact sequence ($1 \dots N$), sending text messages and streaming media.
4. **FloodWait Resilience**: Automatically intercepts Telegram `FLOOD_WAIT_X` rate limits, pausing delivery worker threads and resuming seamlessly without dropping items.

---

#### 5. High-Speed Batch Search & List

Users can search and browse saved collections directly from Telegram:

- **`/batchsearch <query>`**: Searches SQLite `batch_metadata` using case-insensitive exact matching, normalized token matching, and substring search (`LIKE %query%`). Returns detailed cards with item breakdown and direct `[ 🔗 Open Batch ]` links.
- **`/batchlist`**: Displays a paginated directory of all saved collections with interactive `[ ◀️ Prev ]`, `[ 📄 Page X/Y ]`, and `[ Next ▶️ ]` inline navigation.
- **Administrative Deletion**: Batch creators and bot administrators can delete collections using `[ 🗑️ Delete Batch ]`, safely purging both MongoDB records and local SQLite index entries.

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
# FAST BROADCAST ENGINE
# ==============================================================================
BROADCAST_CONCURRENCY=10

# ==============================================================================
# BOT LIFECYCLE & GROUP WELCOME
# ==============================================================================
ENABLE_GROUP_WELCOME=true
WELCOME_DELETE_DELAY=0
REPO_URL=https://github.com/itsafxal/AutoFilter-Go

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
