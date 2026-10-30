# AutoFilter-Go Database Architecture Specification

This document details the multi-database architecture, storage sharding, SQLite runtime indexing, and synchronization mechanisms in **AutoFilter-Go**.

---

## 1. Multi-Cluster MongoDB Architecture

AutoFilter-Go implements a 3-database architecture to cleanly separate volatile application state from high-volume media storage:

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

### Why Separate State from Media?
In large movie bot deployments, media collections often hold 200,000 to 1,000,000+ documents. By isolating user/group/admin data from media collections:
1. **Zero Index Interference**: Queries on users and groups are never blocked by heavy cursor iterations on media files.
2. **Quota & Bandwidth Sharding**: Splitting media documents evenly between DB2 and DB3 allows bot operators to stay within free or budget MongoDB tier limits.
3. **Data Protection**: User state can be backed up or migrated independently without copying gigabytes of static media metadata.

### Round-Robin Alternating Storage
When live media posts arrive in monitored channels (`plugins/channel`), the bot executes `SaveMediaLiveAlternating(ctx, doc)`:
- File 1 is persisted to **DB2**.
- File 2 is persisted to **DB3**.
- File 3 is persisted to **DB2**, and so forth.
- Both write operations simultaneously update the local SQLite index.

---

## 2. Pure-Go SQLite Runtime Engine

To deliver instantaneous search results (<0.5ms) and eliminate MongoDB query costs for ambient text messages, AutoFilter-Go maintains a local SQLite cache (`SQLITE_DB_PATH`, default: `media.sqlite3`).

### 1. Pure-Go / CGO-Free
Unlike traditional Go SQLite drivers (e.g. `mattn/go-sqlite3`) that require CGO, GCC, and host-dependent shared libraries, AutoFilter-Go uses [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) (v1.36.0):
- 100% pure Go.
- Zero CGO dependency (`CGO_ENABLED=0`).
- Seamless cross-compilation for any architecture (`linux/amd64`, `linux/arm64`, `windows/amd64`).

### 2. SQLite PRAGMA Optimizations
Upon opening the SQLite database (`internal/database/sql/db.go`), the manager executes high-performance PRAGMAs:
```sql
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA temp_store = MEMORY;
PRAGMA cache_size = -64000;
PRAGMA busy_timeout = 5000;
```
- **`journal_mode = WAL`**: Write-Ahead Logging allows concurrent readers to query search results while background synchronization or channel indexers write new records without table locks.
- **`cache_size = -64000`**: Allocates 64MB of dedicated RAM for SQLite memory cache, ensuring indexed lookups stay in memory.
- **`synchronous = NORMAL`**: Minimizes disk write flushes without risking database corruption in WAL mode.

### 3. SQLite Schema
```sql
CREATE TABLE IF NOT EXISTS media (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    file_id TEXT UNIQUE NOT NULL,
    file_ref TEXT,
    file_name TEXT NOT NULL,
    file_name_clean TEXT NOT NULL,
    file_size INTEGER NOT NULL,
    file_type TEXT NOT NULL,
    caption TEXT,
    source_db TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_media_file_id ON media(file_id);
CREATE INDEX IF NOT EXISTS idx_media_name_clean ON media(file_name_clean);
CREATE INDEX IF NOT EXISTS idx_media_dedup ON media(file_name, file_size);
```

---

## 3. Synchronization Engine (`internal/database/sql/sync.go`)

### 1. Idempotent Sync Tracking
SQLite maintains a `sync_metadata` table recording synchronization status:
```sql
CREATE TABLE IF NOT EXISTS sync_metadata (
    shard_label TEXT PRIMARY KEY,
    last_synced_id TEXT,
    synced_records INTEGER DEFAULT 0,
    is_complete BOOLEAN DEFAULT FALSE,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

### 2. Startup & Background Sync
When AutoFilter-Go starts:
- **`ENABLE_SYNC=true` (Default)**:
  - If `BACKGROUND_SYNC=true`: The bot checks if `is_complete` is true and data exists. If populated, it skips migration instantly. If cold, it spawns a concurrent background goroutine to stream records from MongoDB to SQLite in batches (`SYNC_BATCH_SIZE=10000`). The bot starts immediately and accepts user requests while the index warms up.
  - If `BACKGROUND_SYNC=false`: The bot blocks during startup until full migration completes.

### 3. Pre-Write Deduplication
Before writing an incoming media post to MongoDB, the bot queries the local SQLite table:
```go
exists, err := db.CheckMediaExistsByDoc(ctx, doc)
```
Because SQLite lookups take `<0.1ms`, duplicates are rejected before any network traffic is sent to MongoDB.

---

## 4. Zero-Sync MongoDB Direct Mode (`ENABLE_SYNC=false`)

For users hosting on ultra-low-spec VPS servers (512MB RAM) or systems with strict disk space limits:

```env
ENABLE_SYNC=false
```

In this mode:
- SQLite startup synchronization is completely bypassed.
- The bot boots in **1 second**.
- Search and retrieval requests query authoritative MongoDB clusters directly.
- Runtime RAM consumption drops to **~50 MB**.
- No `media.sqlite3` file is generated on disk.

---

## 5. Maintenance & Compaction (`/vacuum`)

Over time, deleting messages or updating indexes in SQLite can create internal fragmentation.

Administrators can trigger database compaction on demand by sending:
```text
/vacuum
```
This executes `VACUUM;` and `PRAGMA wal_checkpoint(TRUNCATE);`, compacting the SQLite database and resetting WAL journal logs on disk.

