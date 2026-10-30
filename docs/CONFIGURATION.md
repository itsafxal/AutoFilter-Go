# AutoFilter-Go Configuration Specification

This document provides a comprehensive reference for all environment variables supported by **AutoFilter-Go**.

All configuration parameters are strictly loaded and validated from environment variables at startup (`internal/config/config.go`). If any required variable is missing or malformed, the application will terminate immediately with an explicit, human-readable error.

---

## Table of Contents

1. [Telegram Credentials & Core](#1-telegram-credentials--core)
2. [Monitored Channels](#2-monitored-channels)
3. [Legacy Bot Migration](#3-legacy-bot-migration)
4. [Authoritative MongoDB Storage](#4-authoritative-mongodb-storage)
5. [SQLite Runtime Cache & Sync Engine](#5-sqlite-runtime-cache--sync-engine)
6. [HTTP Streaming & Fast Download Server](#6-http-streaming--fast-download-server)
7. [Auto-Delete & Weekly Trash Retention](#7-auto-delete--weekly-trash-retention)
8. [Media Delivery & Captions](#8-media-delivery--captions)
9. [Maintenance Mode & Telegram Premium Emojis](#9-maintenance-mode--telegram-premium-emojis)
10. [VPS Resource & Runtime Tuning](#10-vps-resource--runtime-tuning)

---

## 1. Telegram Credentials & Core

| Variable | Required | Default | Description | Example |
| :--- | :---: | :---: | :--- | :--- |
| `BOT_TOKEN` | **Yes** | — | Telegram Bot API token issued by [@BotFather](https://t.me/BotFather). | `7123456789:AAF1a2B3c4D5e6F7g8H9i0` |
| `API_ID` | **Yes** | — | Telegram MTProto API App ID obtained from [my.telegram.org](https://my.telegram.org). Also accepts alias `APP_ID`. | `1234567` |
| `API_HASH` | **Yes** | — | Telegram MTProto API 32-character hex hash from [my.telegram.org](https://my.telegram.org). Also accepts alias `APP_HASH`. | `0123456789abcdef0123456789abcdef` |
| `OWNER_ID` | **Yes** | — | Telegram numerical User ID of the primary bot owner. Automatically granted full administrative authority. | `123456789` |
| `ADMINS` | No | `${OWNER_ID}` | Comma-, space-, or newline-separated list of numerical Telegram User IDs granted administrative permissions. `OWNER_ID` is automatically included and deduplicated. | `123456789,987654321` |
| `SESSION_PATH` | No | `bot.session` | File path for persistent MTProto session data storage. | `data/bot.session` |
| `MEMORY_SESSION` | No | `false` | When set to `true`, the MTProto session is maintained in memory without writing `session.dat` to disk. | `false` |
| `LOG_LEVEL` | No | `INFO` | Application log verbosity level. Supported values: `DEBUG`, `INFO`, `WARN`, `ERROR`. | `INFO` |

---

## 2. Monitored Channels

| Variable | Required | Default | Description | Example |
| :--- | :---: | :---: | :--- | :--- |
| `CHANNELS` | No | *(empty)* | Comma-, space-, or newline-separated list of channel IDs monitored for live media indexing. Normalizes raw IDs (`1234567890` or `-1001234567890`) into standard Telegram format `-100...`. | `-1001234567890, -1009876543210` |

---

## 3. Legacy Bot Migration

Run old or retiring Telegram bots concurrently in background goroutines to seamlessly funnel and migrate users to AutoFilter-Go. When users query or send `/start` to an old bot, it responds with an interactive deep-link button pointing to the new bot.

| Variable | Required | Default | Description | Example |
| :--- | :---: | :---: | :--- | :--- |
| `OLD_BOT_TOKENS` | No | *(empty)* | Delimited list (comma, space, newline, semicolon) of old bot tokens to run concurrently for user migration. | `123456:ABC-...,987654:XYZ-...` |
| `OLD_BOT_TOKEN_1` | No | *(empty)* | Dedicated token variable for first legacy bot. | `123456:ABC-DEF1234ghIkl...` |
| `OLD_BOT_TOKEN_2` | No | *(empty)* | Dedicated token variable for second legacy bot. | `987654:XYZ-UVW9876abCd...` |
| `OLD_BOT_TOKEN` | No | *(empty)* | Fallback token variable for a single legacy bot. | `555555:LMN-OPQ5555...` |

---

## 4. Authoritative MongoDB Storage

AutoFilter-Go implements a multi-cluster database model where the **Primary Database** is dedicated exclusively to users, connected groups, force-subscription loadouts, and configuration, while **Secondary** (`DATABASE_URI2`) and **Tertiary** (`DATABASE_URI3`) databases act as round-robin sharded media clusters.

| Variable | Required | Default | Description | Example |
| :--- | :---: | :---: | :--- | :--- |
| `DATABASE_URI` | No | *(empty)* | MongoDB connection URI for the primary cluster (Users, Groups, Auth, Captions). If unset, bot runs in standalone SQLite mode. | `mongodb+srv://user:pass@cluster0.mongodb.net/?retryWrites=true` |
| `DATABASE_NAME` | No | `CyberTG` | Primary database name inside the MongoDB cluster. | `CyberTG` |
| `COLLECTION_NAME` | No | `Telegram-Document` | Media collection name storing file metadata, identifiers, and references. | `Telegram-Document` |
| `DATABASE_URI2` | No | `${DATABASE_URI}` | MongoDB URI for Media Shard 1 (DB2). If omitted, falls back to `DATABASE_URI`. | `mongodb+srv://user:pass@cluster1.mongodb.net/?retryWrites=true` |
| `DATABASE_URI3` | No | `${DATABASE_URI}` | MongoDB URI for Media Shard 2 (DB3). If omitted, falls back to `DATABASE_URI`. | `mongodb+srv://user:pass@cluster2.mongodb.net/?retryWrites=true` |

---

## 5. SQLite Runtime Cache & Sync Engine

AutoFilter-Go uses a 100% pure-Go, CGO-free SQLite engine (`modernc.org/sqlite`) running in **WAL (Write-Ahead Logging)** mode for sub-millisecond search lookups and pre-write deduplication.

| Variable | Required | Default | Description | Example |
| :--- | :---: | :---: | :--- | :--- |
| `SQLITE_DB_PATH` | No | `media.sqlite3` | File path for the local SQLite database. | `data/media.sqlite3` |
| `ENABLE_SYNC` | No | `true` | When `true`, synchronizes media from MongoDB into SQLite. When `false` (or `DISABLE_SYNC=true`), completely bypasses synchronization and queries MongoDB directly with ~50MB RAM footprint and instant 1-second boot. | `true` |
| `BACKGROUND_SYNC` | No | `true` | When `true`, synchronization runs in a concurrent background goroutine, allowing the bot to start immediately while the index populates. | `true` |
| `SYNC_BATCH_SIZE` | No | `10000` | Number of documents fetched and written per batch during SQLite synchronization. Recommended `1500`–`5000` on 1GB VPS. | `1500` |

---

## 6. HTTP Streaming & Fast Download Server

AutoFilter-Go includes a built-in HTTP server (`internal/stream`) providing fast direct downloads, video streaming with byte-range support (RFC 7233), in-browser HTML5 player, and M3U playlists for external players (VLC, MX Player).

| Variable | Required | Default | Description | Example |
| :--- | :---: | :---: | :--- | :--- |
| `STREAM_SERVER_ENABLED` | No | `true` | Enable or disable the HTTP streaming and download service. | `true` |
| `STREAM_URL` | No | `http://localhost:8080` | Publicly reachable base URL for streaming and download links. Also accepts alias `WEB_URL`. Trailing slashes are automatically stripped. | `https://stream.yourdomain.com` |
| `STREAM_PORT` | No | `8080` | TCP port for the stream HTTP server. Also reads from standard `PORT` variable. | `8080` |
| `STREAM_BIND_ADDR` | No | `0.0.0.0` | IP address to bind the HTTP server. Use `127.0.0.1` when placing behind a reverse proxy (Nginx, Caddy). | `0.0.0.0` |
| `ENABLE_FAST_DOWNLOAD_BUTTON` | No | `true` | Whether to append the interactive "⚡ Get Fast Download" button to delivered media messages. | `true` |
| `FAST_DOWNLOAD_BUTTON_STYLE` | No | `green` | Color/style for the fast download button. Options: `green` (Success) or `blue` (Primary). | `green` |
| `FAST_DOWNLOAD_LINK_EXPIRY` | No | `30m` | Duration before an HMAC-signed streaming/download link expires. Format: `10m`, `30m`, `1h`, `24h`. | `30m` |
| `AUTO_DELETE_FAST_DOWNLOAD_TIME` | No | `5m` | Time before the bot auto-deletes fast download link messages posted in chat. | `5m` |

---

## 7. Auto-Delete & Weekly Trash Retention

Protect your Telegram channels and groups from copyright strikes by automatically deleting messages on a schedule.

| Variable | Required | Default | Description | Example |
| :--- | :---: | :---: | :--- | :--- |
| `AUTO_DELETE_FILTER_TIME` | No | `15m` | Auto-deletion timer for search result messages sent in groups and private chat. Set to `0` to disable. | `15m` |
| `AUTO_DELETE_FILE_TIME` | No | `30m` | Auto-deletion timer for delivered media files. Set to `0` to disable. | `30m` |
| `AUTO_DELETE_WEEKLY_TRASH` | No | `true` | Enables automatic weekly trash garbage collection to purge bot-sent messages older than the retention threshold. | `true` |
| `TRASH_RETENTION_DAYS` | No | `7` | Retention threshold in days before weekly trash permanently deletes tracked messages. | `7` |

---

## 8. Media Delivery & Captions

| Variable | Required | Default | Description | Example |
| :--- | :---: | :---: | :--- | :--- |
| `CUSTOM_CAPTION` | No | `{filename}\n\nFile Size : {file size}` | Default media caption template. Supports placeholders `{filename}`, `{file size}`, and `{caption}`. Literal `\n` is converted to line breaks. | `📂 <b>{filename}</b>\n\n⚡ <i>Size: {file size}</i>\n\n@YourChannel` |
| `FSUB_CHANNEL_DESCRIPTION` | No | `You need to request to join channel to get file 🚀` | Description text displayed in the Force-Subscribe join prompt. | `Please join our update channel to access files 🚀` |
| `START_PIC` | No | `assets/start.jpg` | Local file path or direct HTTP URL to the banner image displayed with `/start`. | `assets/start.jpg` |

---

## 9. Maintenance Mode & Telegram Premium Emojis

### Maintenance Mode

| Variable | Required | Default | Description | Example |
| :--- | :---: | :---: | :--- | :--- |
| `MAINTENANCE_MODE` | No | `false` | When `true`, only bot owners and admins can perform searches and receive files. Regular users receive a maintenance notice and only have access to `/start`. | `false` |

### Custom Telegram MTProto Premium Emojis

Custom Telegram emoji IDs (int64) used to render Telegram Premium icons on buttons and message templates.

| Variable | Default ID | Description |
| :--- | :---: | :--- |
| `ENABLE_PREMIUM_EMOJI` | `true` | Global toggle to enable custom Telegram premium emoji entities. |
| `PREV_BUTTON_EMOJI_ID` | `5469982030773120950` | Previous page button emoji. |
| `NEXT_BUTTON_EMOJI_ID` | `5469715085670772857` | Next page button emoji. |
| `PAGE_BUTTON_EMOJI_ID` | `5837048052061311256` | Current page counter indicator button. |
| `QUALITY_BUTTON_EMOJI_ID` | `5276251908774858442` | Quality selection button. |
| `LANGUAGE_BUTTON_EMOJI_ID` | `5276011261757265100` | Language selection button. |
| `MULTI_LANG_BUTTON_EMOJI_ID` | `5256169350368353876` | Multi-language filter button. |
| `SELECT_LANGUAGE_EMOJI_ID` | `5447410659077661506` | Header emoji for "Select Desired Language". |
| `SELECT_QUALITY_EMOJI_ID` | `5334544901428229844` | Header emoji for "Select Desired Quality". |
| `FAST_DOWNLOAD_BUTTON_EMOJI_ID` | `6156513311585211842` | Fast download link button icon. |
| `GREETING_PREFIX_EMOJI_ID` | `5472055112702629499` | Greeting icon prefix (also: `HEY_PREFIX_EMOJI_ID`). |
| `GREETING_EMOJI_ID` | `5427232022252764138` | Greeting user mention icon (also: `HEY_EMOJI_ID`). |
| `QUERY_EMOJI_ID` | `5190859184312167965` | Query line header icon. |
| `PAGE_TEXT_EMOJI_ID` | `5836954069586946866` | Page status line icon. |
| `NOTE_PREFIX_EMOJI_ID` | `4956461073550017373` | Warning prefix before note (also: `NOTE_EMOJI_ID`). |
| `NOTE_SUFFIX_EMOJI_ID` | `4958724224962265918` | Warning suffix after note (also: `WARN_EMOJI_ID`). |
| `NOTE_EXPIRE_EMOJI_ID` | `5440660757194744323` | Expiration notice icon (also: `EXPIRE_EMOJI_ID`). |
| `BOT_NAME_EMOJI_ID` | `6104631352190043951` | Branding bot name icon (also: `BOT_EMOJI_ID`). |
| `FILE_SIZE_PREFIX_EMOJI_ID` | `5992513524440502236` | File size prefix icon on result buttons. |
| `FILE_SIZE_SUFFIX_EMOJI_ID` | `5992519984071315160` | File size suffix icon on result buttons. |

---

## 10. VPS Resource & Runtime Tuning

Essential configuration parameters for deployment on budget VPS instances (1GB–2GB RAM, 1–2 vCPUs).

| Variable | Required | Default | Description | Example |
| :--- | :---: | :---: | :--- | :--- |
| `GOMEMLIMIT` | No | `512MiB` | Hard soft-limit on the Go runtime heap. Triggers aggressive garbage collection before the host system runs out of physical RAM. | `512MiB` |
| `GOGC` | No | `60` | Garbage collection trigger threshold as a percentage of live heap. Lower values (e.g. `50`–`60`) trigger more frequent GC cycles, saving physical RAM. | `60` |
| `MAX_MEMORY_MB` | No | `512` | Alternative memory cap in megabytes applied when `GOMEMLIMIT` is omitted. | `512` |
| `MAX_CPU_CORES` | No | *(all)* | Caps `runtime.GOMAXPROCS`. Setting `1` on a single-core VPS prevents 100% CPU starvation. | `1` |

