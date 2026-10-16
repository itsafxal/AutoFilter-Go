# MYTGBOTZ — Telegram Bot Foundation & Database Architecture

A modern, high-performance Telegram bot written in Go, powered by [`gotgbot/v2`](https://github.com/PaulSonOfLars/gotgbot/v2), featuring an authoritative MongoDB persistence layer, temporary high-speed SQLite runtime indexing, a compile-time plugin registry, structured logging (`slog`), non-root multi-stage Docker deployment, and strict startup configuration validation.

---

## Architecture & Directory Structure

```text
MYTGBOTZ/
├── cmd/
│   └── bot/
│       └── main.go              # Application entrypoint & signal orchestration
├── internal/
│   ├── app/                     # Application lifecycle & container
│   ├── commands/                # Common command utilities & payload safety
│   ├── config/                  # Environment variable parsing & validation
│   ├── database/                # Dual-database layer
│   │   ├── manager.go           # Unified MongoDB & SQLite coordinator
│   │   ├── mongodb/             # Authoritative MongoDB models & repositories
│   │   └── sql/                 # Temporary SQLite runtime schema & sync engine
│   ├── logging/                 # Structured logging & credential redaction
│   ├── plugins/                 # Extensible plugin registry & validation
│   └── telegram/                # Telegram bot client & updater service
├── plugins/
│   ├── cmds/                    # /cmds (and /help) dynamic command guide
│   ├── id/                      # /id personal & group chat ID inspector
│   ├── info/                    # /info (and /about) safe system overview
│   ├── ping/                    # /ping response speed & latency measurement
│   └── start/                   # /start welcome message & user persistence
├── tests/                       # Integrated end-to-end unit tests
├── docs/                        # Architecture & verification reports
│   ├── STEP_01_REPORT.md        # Step 1 foundation verification report
│   └── STEP_02_REPORT.md        # Step 2 database & sync verification report
├── .dockerignore                # Build context exclusion rules
├── .env.example                 # Environment configuration template
├── Dockerfile                   # Multi-stage production build (non-root)
├── docker-compose.yml           # Optional local testing composition
├── go.mod                       # Go module dependencies
└── go.sum                       # Cryptographic dependency checksums
```

---

## Dual-Database Architecture

### 1. Authoritative Primary Database: MongoDB
- **Collections**:
  - `users`: Tracks registered users, names, and ban status.
  - `groups`: Tracks connected groups and chat enabled/disabled status.
  - `COLLECTION_NAME` (Default: `Telegram-Document`): Authoritative repository of media documents, file references, file IDs, sizes, and captions.
- **Data Protection**: Read operations on existing media are non-destructive. No collections are dropped and no schemas are migrated destructively.

### 2. Runtime Temporary Database: SQLite
- **Engine**: Pure-Go SQLite (`modernc.org/sqlite`, 100% CGO-free).
- **Role**: High-speed, local read cache populated during startup from authoritative MongoDB data.
- **Performance**: Configured in WAL mode (`PRAGMA journal_mode=WAL`) with memory-backed cache (`PRAGMA cache_size=-64000`) and sub-millisecond indexed lookups on `file_id`, `file_name`, and `caption`.
- **Sync Safety**: Idempotent synchronization streams MongoDB records in batches (`BATCH_SIZE=10000`). If already synchronized, it skips full copying. Incomplete syncs are tracked via `sync_metadata` and never exposed as ready.

---

## Foundational Commands

| Command | Aliases | Description | Context Support |
| :--- | :--- | :--- | :--- |
| `/start` | — | Welcome greeting, inline buttons, user/group persistence, safe deep-link validation | Private, Group, Supergroup |
| `/cmds` | `/help` | Dynamically generated usage guide reflecting only compiled commands | All Chats |
| `/ping` | — | Response timing report distinguishing local processing from Telegram API round-trip | All Chats |
| `/id` | — | Telegram user ID, chat ID, and replied message metadata | Private, Group, Supergroup |
| `/info` | `/about` | Bot information, runtime diagnostics, and replied user profile details | Private, Group, Supergroup |
| `/getfile` | `/file` | Retrieve and deliver a media file by its unique identifier | Private, Group, Supergroup |

---

## Media Delivery System

The application incorporates a production-grade, modular **Media Delivery Service** (`internal/delivery`) designed to securely resolve and transmit stored media without modifying existing database collections:

### 1. Delivery Architecture
- **Two-Tier Resolution**: Queries the high-speed local SQLite cache (`SqlMediaRepository`) first for sub-millisecond retrieval. If missing or cache is cold, it seamlessly falls back to authoritative MongoDB collections (`MongoMediaRepository`).
- **Telegram Bot API Method Dispatching**: Automatically inspects `file_type` and routes to the appropriate native `gotgbot/v2` send method:
  - `video` → `SendVideo` (with automatic fallback to `SendDocument` for video container incompatibilities such as MKV)
  - `audio` → `SendAudio`
  - `photo` → `SendPhoto`
  - `animation` → `SendAnimation`
  - `voice` → `SendVoice`
  - `document` (or unspecified) → `SendDocument`
- **Protected Content**: Supports `protect_content` flags to prevent saving or forwarding of delivered media when requested via `filep_<id>`.

### 2. Access Control & Moderation
- **User Ban Enforcement**: Checks the authoritative MongoDB `users` collection. Banned users (`ban_status.is_banned = true`) are immediately rejected.
- **Group Status Verification**: Verifies chat status in the `groups` collection (`chat_status.is_disabled = true`). Disabled chats cannot receive automated media deliveries.
- **In-Flight Request Deduplication**: A thread-safe `InFlightTracker` prevents duplicate deliveries and race conditions when users rapidly tap buttons or repeat callback triggers.

### 3. Interaction Entry Points
- **Deep-Links**: Users can access media via `/start file_<file_id>` or `/start filep_<file_id>`.
- **Direct Command**: Admins and users can request files via `/getfile <file_id>`.
- **Inline Callback Buttons**: Handles `file#<id>` (redirecting group users to private chat deep-links) and `checksub#<id>` (delivering files directly in private chats).


## Environment Configuration

Configuration is loaded strictly from environment variables at startup. Missing or invalid required variables cause an immediate, explicit startup failure.

| Variable | Required | Default | Description |
| :--- | :---: | :---: | :--- |
| `BOT_TOKEN` | **Yes** | — | Telegram Bot API token obtained from [@BotFather](https://t.me/BotFather) |
| `OWNER_ID` | **Yes** | — | Numeric Telegram User ID of the primary owner |
| `ADMINS` | No | `${OWNER_ID}` | Comma- or space-separated list of additional admin User IDs |
| `LOG_LEVEL` | No | `INFO` | Logging verbosity (`DEBUG`, `INFO`, `WARN`, `ERROR`) |
| `DATABASE_URI` | No | — | Authoritative MongoDB connection URI |
| `DATABASE_NAME` | No | `CyberTG` | Authoritative MongoDB database name |
| `COLLECTION_NAME` | No | `Telegram-Document` | Authoritative MongoDB media collection name |
| `DATABASE_URI2` | No | `${DATABASE_URI}` | Secondary MongoDB URI for sharded media |
| `DATABASE_URI3` | No | `${DATABASE_URI}` | Tertiary MongoDB URI for sharded media |
| `SQLITE_DB_PATH` | No | `media.sqlite3` | Local SQLite database file path |
| `SYNC_BATCH_SIZE` | No | `10000` | Synchronization batch size |

---

## Local Development & Testing

### 1. Run All Tests
```bash
go test -count=1 ./...
```

### 2. Verify Static Analysis & Formatting
```bash
gofmt -l .
go vet ./...
```

### 3. Build Binary
```bash
go build -o ./bin/bot ./cmd/bot
```

---

## Docker Deployment

The application features a hardened, multi-stage `Dockerfile`:
- **Builder Stage**: `golang:1.24-alpine3.21` compiles a statically linked, stripped executable (`CGO_ENABLED=0`).
- **Runtime Stage**: `alpine:3.21` with `ca-certificates` and `tzdata`.
- **Security**: Runs under an unprivileged user `botuser` (UID `10001`).
- **No baked secrets**: All configuration is injected via environment variables at runtime.

```bash
docker build -t mytgbotz:latest .
docker run -d --name mytgbotz_app --restart unless-stopped --env-file .env mytgbotz:latest
```