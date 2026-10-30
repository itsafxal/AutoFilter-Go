# AutoFilter-Go Troubleshooting & Diagnostic Guide

This guide helps you identify, diagnose, and resolve common issues encountered while deploying or running **AutoFilter-Go**.

---

## Table of Contents

1. [Startup & Configuration Failures](#1-startup--configuration-failures)
2. [Telegram MTProto Connectivity Issues](#2-telegram-mtproto-connectivity-issues)
3. [MongoDB Connection & Replica Set Issues](#3-mongodb-connection--replica-set-issues)
4. [SQLite Database Locks & Sync Problems](#4-sqlite-database-locks--sync-problems)
5. [Media Delivery & Force-Subscribe Issues](#5-media-delivery--force-subscribe-issues)
6. [HTTP Streaming & Fast Download Server Issues](#6-http-streaming--fast-download-server-issues)
7. [Docker & Container Volume Issues](#7-docker--container-volume-issues)
8. [Windows Development Environment](#8-windows-development-environment)

---

## 1. Startup & Configuration Failures

### Issue: `FATAL: configuration validation failure: ... is required`
**Cause**: One or more mandatory environment variables (`BOT_TOKEN`, `API_ID`, `API_HASH`, or `OWNER_ID`) are missing or blank.
**Solution**:
1. Check that `.env` exists in the working directory.
2. Ensure values do not contain unquoted whitespace or comments on the same line.
3. If running via systemd or Docker, ensure the environment file is loaded:
   ```bash
   # Test loading locally
   set -a; source .env; set +a
   ./autofilter-go
   ```

### Issue: `configuration error: API_ID must be a positive integer`
**Cause**: The `API_ID` variable contains letters or quotes.
**Solution**: Ensure `API_ID` is purely numerical without quotes (e.g. `API_ID=1234567`).

---

## 2. Telegram MTProto Connectivity Issues

### Issue: `RPCError: 400 API_ID_INVALID`
**Cause**: The `API_ID` and `API_HASH` combination does not match the app registered at [my.telegram.org](https://my.telegram.org).
**Solution**: Log in to [my.telegram.org](https://my.telegram.org), verify your API development tools credentials, and copy them directly into `.env`.

### Issue: `Unauthorized: Bot token is invalid`
**Cause**: The `BOT_TOKEN` string has been revoked or pasted incorrectly.
**Solution**: Open Telegram, message [@BotFather](https://t.me/BotFather), generate a new token via `/revoke`, and update `BOT_TOKEN`.

### Issue: `FLOOD_WAIT_X: A wait of X seconds is required`
**Cause**: Telegram has rate-limited your bot due to excessive API requests in a short duration (e.g., during aggressive bulk indexing).
**Solution**:
- AutoFilter-Go automatically handles flood waits with exponential backoff.
- When running bulk indexing (`/index` or `/indexlink`), avoid running multiple concurrent indexing operations across massive channels.

---

## 3. MongoDB Connection & Replica Set Issues

### Issue: `server selection error: server selection timeout`
**Cause**: The application cannot reach your MongoDB cluster within the timeout window.
**Common Causes & Solutions**:
1. **Network Access / IP Whitelist**: In MongoDB Atlas, go to **Network Access** > **IP Access List** and ensure your VPS IP address is added (or `0.0.0.0/0` for initial testing).
2. **DNS SRV Resolution Failure**: Some budget VPS providers have DNS resolvers that fail on MongoDB `mongodb+srv://` SRV records.
   *Fix*: Update `/etc/resolv.conf` on your VPS to use Google or Cloudflare DNS:
   ```bash
   echo "nameserver 1.1.1.1" | sudo tee /etc/resolv.conf
   echo "nameserver 8.8.8.8" | sudo tee -a /etc/resolv.conf
   ```
3. **Invalid Credentials**: Ensure special characters in your MongoDB password (such as `@`, `:`, `/`, or `%`) are URL-encoded (`%40`, `%3A`, etc.).

---

## 4. SQLite Database Locks & Sync Problems

### Issue: `database is locked (5) (SQLITE_BUSY)`
**Cause**: Another process or background thread is holding a lock on `media.sqlite3`.
**Solution**:
1. Verify no multiple instances of the bot are running against the same SQLite file:
   ```bash
   pgrep -a autofilter-go
   ```
2. AutoFilter-Go automatically configures SQLite in `WAL` mode with a `busy_timeout = 5000ms`. If you see locks, run the `/vacuum` command to compact WAL logs.

### Issue: SQLite Sync takes too much memory during startup
**Cause**: Attempting to synchronize hundreds of thousands of documents with a huge batch size on a low-RAM VPS.
**Solution**:
1. Lower the batch size in `.env`:
   ```env
   SYNC_BATCH_SIZE=1500
   ```
2. Or run in **Zero-Sync Mode** (`ENABLE_SYNC=false`), which requires ~50MB RAM and zero SQLite sync.

---

## 5. Media Delivery & Force-Subscribe Issues

### Issue: Bot says `File not found in database`
**Cause**: The requested file ID does not exist in SQLite or MongoDB shards.
**Solution**:
- Ensure the channel containing the media has been indexed via `/index` or monitored via `CHANNELS`.
- If the file was indexed before adding secondary MongoDB shards, run synchronization.

### Issue: User is stuck in Force-Subscribe loop even after requesting to join
**Cause**:
1. The bot is not an administrator in the Force-Subscribe channel with "Invite Users via Link" or "Manage Join Requests" permission.
2. The user has not submitted a join request or was banned from the channel.
**Solution**:
- Make sure the bot is added as an **Administrator** with full invite permissions in Channel 1, Channel 2, and Channel 3.
- Check active FSub status using `/view_sub`.

---

## 6. HTTP Streaming & Fast Download Server Issues

### Issue: `listen tcp 0.0.0.0:8080: bind: address already in use`
**Cause**: Another service or an orphaned bot instance is already using port `8080`.
**Solution**:
1. Identify the process using the port:
   ```bash
   sudo lsof -i :8080
   # or
   sudo ss -tulpn | grep 8080
   ```
2. Terminate the blocking process, or change `STREAM_PORT` in `.env` (e.g. `STREAM_PORT=8090`).

### Issue: "Invalid or tampered download link signature" (403 Forbidden)
**Cause**:
1. The link has expired (governed by `FAST_DOWNLOAD_LINK_EXPIRY`, default: 30 minutes).
2. The `sig` query parameter was truncated or modified.
**Solution**: Request a new download link via `/fastdl <file_id>`.

### Issue: Video streaming does not seek or stalls in browser/VLC
**Cause**: Reverse proxy is buffering byte-range requests.
**Solution**: If using Nginx, ensure `proxy_buffering off;` is set in your server block. See [docs/DEPLOYMENT.md](file:///c:/Users/Afsal/OneDrive/Desktop/AutoFilter-Go/docs/DEPLOYMENT.md).

---

## 7. Docker & Container Volume Issues

### Issue: Session or SQLite database resets whenever container restarts
**Cause**: Session and SQLite files were written inside the container's ephemeral layer instead of a persistent volume.
**Solution**: In your `docker run` or `docker compose` configuration, ensure `SESSION_PATH` and `SQLITE_DB_PATH` point to `/app/data/`, and mount a persistent volume to `/app/data/`:
```bash
docker run -d \
  --name autofilter-go \
  -v autofilter_data:/app/data \
  -e SESSION_PATH=/app/data/bot.session \
  -e SQLITE_DB_PATH=/app/data/media.sqlite3 \
  autofilter-go:latest
```

---

## 8. Windows Development Environment

### Issue: `An Application Control policy has blocked this file`
**Cause**: On certain Windows enterprise environments or developer workstations with Windows Defender Application Control / AppLocker enabled, executing temporary `.test.exe` binaries directly out of `%TEMP%` is blocked by policy.
**Solution**:
- Standard compilation (`go build -o ./bin/bot.exe ./cmd/bot`) and static vetting (`go vet ./...`) succeed without issue.
- Individual unit tests without disk binaries run cleanly (`go test ./internal/config ./internal/commands ./internal/delivery`).

