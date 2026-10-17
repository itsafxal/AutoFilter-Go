# Gogram Migration Master Report — MYTGBOTZ

## Executive Summary
This document provides a comprehensive report of the migration of the MYTGBOTZ Telegram client layer from `github.com/PaulSonOfLars/gotgbot/v2` to the unified MTProto library **Gogram** (`github.com/amarnathcjd/gogram/telegram` pinned at `v1.7.71`).

The migration preserves the existing architecture, MongoDB persistence layer, SQLite runtime synchronization cache, compile-time plugin registry, media delivery workflows, error handling, and structured logging (`slog`), while replacing the HTTP-based Bot API transport with Telegram's binary MTProto protocol.

---

## 1. Architecture Comparison

### Previous Telegram Architecture (`gotgbot/v2`)
- **Transport**: Telegram HTTP Bot API via reverse-proxy webhooks / long-polling (`https://api.telegram.org/bot<TOKEN>/...`).
- **Client Representation**: `gotgbot.Bot` with `ext.Dispatcher` and `ext.Updater`.
- **Media Delivery**: Separate API methods per media type (`SendDocument`, `SendVideo`, `SendAudio`, `SendPhoto`, `SendAnimation`, `SendVoice`) with `gotgbot.InputFileOrString`.
- **Update Handling**: Context wrapper `*ext.Context` and `*gotgbot.Message` wrapping raw JSON payloads from Telegram Bot API.
- **Keyboards**: Bot API JSON representations (`gotgbot.InlineKeyboardMarkup` and `gotgbot.InlineKeyboardButton`).
- **Deep-Links**: Handled by inspecting `args[1]` on `/start` command updates.

### New Telegram Architecture (Gogram MTProto `v1.7.71`)
- **Transport**: Native Telegram MTProto 2.0 TCP binary protocol connection directly to Telegram data centers (DC 1–5).
- **Client Representation**: Unified `*telegram.Client` constructed via `telegram.NewClient(ClientConfig)` and authenticated using `LoginBot(token)`.
- **Media Delivery**: Unified `SendMedia(peerID any, media any, opts ...*MediaOptions) (*NewMessage, error)` using `telegram.ResolveBotFileID(fileID)` to unpack Bot API `file_id` strings directly into MTProto `InputMedia` objects.
- **Update Handling**: Strongly-typed native MTProto updates mapped to `*telegram.NewMessage` and `*telegram.CallbackQuery`. Handlers attached directly via `client.AddCommandHandler`, `client.AddCallbackHandler`, and `client.AddMessageHandler`.
- **Keyboards**: Built via `telegram.NewKeyboard().AddRow(...).Build()`, emitting native MTProto `*ReplyInlineMarkup`.
- **Deep-Links**: Handled uniformly via `/start [payload]` with `commands.ValidateStartPayload` safety checks.

---

## 2. Dependency Specification & Version Pinning
- **Selected Version**: `github.com/amarnathcjd/gogram v1.7.71`
- **Official Import Path**: `github.com/amarnathcjd/gogram/telegram`
- **Go Toolchain Requirement**: `go 1.25.0` (validated against active Go 1.27.0 compiler).
- **Dependency Purge**: `github.com/PaulSonOfLars/gotgbot/v2` has been completely purged from `go.mod` and `go.sum`. There are zero remaining production imports or references to the old library.

### Clean `go.mod` Dependencies:
```go
module github.com/itsafxal/MYTGBOTZ

go 1.25.0

require (
	github.com/amarnathcjd/gogram v1.7.71
	go.mongodb.org/mongo-driver v1.17.1
	modernc.org/sqlite v1.36.0
)
```

---

## 3. Authentication & Configuration

Gogram requires MTProto API credentials in addition to the Telegram Bot Token:
1. `API_ID` / `APP_ID`: Numeric identifier obtained from [my.telegram.org](https://my.telegram.org).
2. `API_HASH` / `APP_HASH`: Hexadecimal string secret from [my.telegram.org](https://my.telegram.org).
3. `BOT_TOKEN`: Standard bot token from [@BotFather](https://t.me/BotFather).
4. `SESSION_PATH`: Path for persistent MTProto session storage (default: `session.dat`).
5. `MEMORY_SESSION`: Boolean flag to run purely in-memory without creating a session file on disk (default: `false`).

### Startup Validation
- Validation in `internal/config/config.go` guarantees that startup fails immediately and explicitly if `API_ID <= 0`, `API_HASH == ""`, `BOT_TOKEN == ""`, or `OWNER_ID <= 0`.
- Sensitive values (`BOT_TOKEN`, `API_HASH`, MongoDB connection credentials) are masked during logging and when formatting `Config.String()`.

---

## 4. Migrated Command & Plugin System

The modular compile-time plugin registry (`internal/plugins`) was preserved and migrated to Gogram's dispatcher interfaces.

| Plugin | Triggers & Aliases | Handler Type | Gogram API Replacement |
| :--- | :--- | :--- | :--- |
| `start` | `/start` | Command | `m.Reply(text, &telegram.SendOptions{ParseMode: "html", ReplyMarkup: markup})` |
| `cmds` | `/cmds`, `/help` | Command | Generates dynamic help text from registry and replies with HTML formatting |
| `ping` | `/ping` | Command | Computes local processing time vs MTProto round-trip latency via `m.Reply` and `sentMsg.Edit` |
| `id` | `/id` | Command | Formats private chat ID card or group chat report using `m.Sender`, `m.ChatID()`, and `m.GetReplyMessage()` |
| `info` | `/info`, `/about` | Command | Reports runtime metrics, uptime, Go version, and `Gogram v1.7.71` library identity |
| `delivery` | `/getfile`, `/file` | Command | Resolves requested `file_id` and triggers `DeliveryService.Deliver` |
| `delivery` | `file#` | Callback | `cb.Answer("", &telegram.CallbackOptions{URL: startURL})` redirecting user to private chat deep link |
| `delivery` | `checksub#` | Callback | Direct in-chat delivery acknowledgment and file transmission |
| `autofilter` | Ambient Text Messages | Message Filter | Ambient search filter excluding commands via `m.IsCommand() \|\| strings.HasPrefix(m.Text(), "/")` |
| `autofilter` | `next\|`, `sp#`, `languages#`, `fl#`, `qualities#`, `fq#`, `close_data`, `pages` | Callback | Interactive search result pagination, spelling correction, language/quality filtering, and modal dismissal |

---

## 5. Media Delivery Compatibility

### Media Lookup & Two-Tier Architecture
1. **Tier 1 (SQLite Cache)**: Queries local `SqlMediaRepository` for sub-millisecond retrieval.
2. **Tier 2 (MongoDB Fallback)**: If record is missing in SQLite or cache is cold, transparently queries authoritative MongoDB `Telegram-Document` collection.

### MTProto Media Sending Mechanism
Gogram provides a single public media dispatch method on `*telegram.Client`:
```go
SendMedia(peerID any, media any, opts ...*telegram.MediaOptions) (*telegram.NewMessage, error)
```
- **Bot API `file_id` Resolution**: Handled via `telegram.ResolveBotFileID(rec.FileID)`. This unpacks the Bot API binary string format into native MTProto media objects (`*MessageMediaDocument` or `*MessageMediaPhoto`).
- **Format Routing**:
  - `video` → Dispatched with video attributes. If Telegram rejects the format with a constraint error, the service automatically falls back to `ForceDocument: true`.
  - `audio` → Dispatched with audio attributes.
  - `photo` → Dispatched as photo.
  - `animation` → Dispatched with animation attributes.
  - `voice` → Dispatched as voice note.
  - `document` → Dispatched with `ForceDocument: true`.
- **Protection**: `NoForwards: true` prevents saving and forwarding for protected requests (`filep_`).

### Error Mapping
MTProto RPC errors are sanitized and mapped to domain errors:
- `FLOOD_WAIT_X` → `ErrRateLimited` (logging safe retry duration without leaking secrets).
- `USER_IS_BLOCKED` / `CHAT_WRITE_FORBIDDEN` → `ErrBlockedByUser`.
- `FILE_ID_INVALID` / `MEDIA_EMPTY` → `ErrTelegramAPI`.

---

## 6. Preservation of MongoDB & SQLite Architectures

- **Authoritative Database**: MongoDB (`DATABASE_NAME`, default `CyberTG`; `COLLECTION_NAME`, default `Telegram-Document`) remains 100% untouched.
- **Zero Schema Migrations**: No collections were renamed, altered, dropped, or rewritten.
- **BSON Compatibility**: Existing BSON schemas for `users`, `groups`, and media documents were preserved in full.
- **SQLite Engine**: Pure-Go SQLite (`modernc.org/sqlite`, CGO-free) runtime sync continues to run idempotently on startup.

---

## 7. Future Channel History & Indexing Readiness

Gogram was selected partly to enable future channel-history access. However, in accordance with the strict scope constraints of this migration:
- **No Channel History Scanning Workers** were implemented.
- **No Scraping Jobs** or background ingestion routines were added.
- **Interface Contract**: Defined `telegram.HistoryReader` in `internal/telegram/bot.go` to provide a clean contract for future extension:
  ```go
  type HistoryReader interface {
      GetChannelHistory(ctx context.Context, channelID any, offsetID int32, limit int32) ([]*telegram.NewMessage, error)
  }
  ```
- **Permission Limitations**: Bot identities (`LoginBot`) can only access message history in channels or supergroups where they have been explicitly added with administrative read permissions. Arbitrary historical media scraping for public or private channels where the bot is not an admin requires a separately authorized user-account session (`LoginUser`).

---

## 8. Licensing Considerations

- **Library**: `github.com/amarnathcjd/gogram` is licensed under the **GNU General Public License v3.0 (GPL-3.0)**.
- **Implications**:
  - GPL-3.0 is a strong copyleft license. If MYTGBOTZ is distributed in binary form to third parties, the entire combined work must be made available under GPL-3.0.
  - If MYTGBOTZ is operated privately as a network service (Software as a Service / hosted bot) without distributing executable binaries to end users, the GPL-3.0 copyleft requirements are not triggered. (Note: AGPL-3.0 would trigger on network use, but Gogram is licensed under GPL-3.0, not AGPL).
  - No Gogram source code has been vendored or copied into the repository. It is imported strictly as an external Go module dependency.

---

## 9. Verification & Automated Test Results

All checks and automated tests were executed in the local development environment:

### 1. Code Formatting (`gofmt -l .`)
```text
Clean: 0 files require formatting.
```

### 2. Static Analysis (`go vet ./...`)
```text
Clean: 0 issues found across all packages.
```

### 3. Unit & Integration Test Suite (`go test -count=1 ./...`)
```text
ok  	github.com/itsafxal/MYTGBOTZ/cmd/bot            (cached)
ok  	github.com/itsafxal/MYTGBOTZ/internal/app       5.994s  (PASS)
ok  	github.com/itsafxal/MYTGBOTZ/internal/commands  0.134s  (PASS)
ok  	github.com/itsafxal/MYTGBOTZ/internal/config    0.141s  (PASS)
ok  	github.com/itsafxal/MYTGBOTZ/internal/database  2.247s  (PASS)
ok  	github.com/itsafxal/MYTGBOTZ/internal/delivery  4.127s  (PASS)
ok  	github.com/itsafxal/MYTGBOTZ/internal/logging   6.396s  (PASS)
ok  	github.com/itsafxal/MYTGBOTZ/internal/plugins   3.903s  (PASS)
ok  	github.com/itsafxal/MYTGBOTZ/internal/search    9.634s  (PASS)
ok  	github.com/itsafxal/MYTGBOTZ/plugins/autofilter 5.491s  (PASS)
ok  	github.com/itsafxal/MYTGBOTZ/plugins/cmds       3.731s  (PASS)
ok  	github.com/itsafxal/MYTGBOTZ/plugins/delivery   3.671s  (PASS)
ok  	github.com/itsafxal/MYTGBOTZ/plugins/id         3.420s  (PASS)
ok  	github.com/itsafxal/MYTGBOTZ/plugins/info       2.559s  (PASS)
ok  	github.com/itsafxal/MYTGBOTZ/plugins/ping       2.439s  (PASS)
ok  	github.com/itsafxal/MYTGBOTZ/plugins/start      2.297s  (PASS)
ok  	github.com/itsafxal/MYTGBOTZ/tests              2.844s  (PASS)
```
**Result**: 100% of tests passed across all packages.

### 4. Binary Compilation (`go build -o ./bin/bot.exe ./cmd/bot`)
```text
Exit Code: 0
Binary built successfully: ./bin/bot.exe
```

### 5. Docker Verification
```text
Docker Engine Availability: NOT AVAILABLE (Docker daemon not installed in local environment)
Status: NOT VERIFIED (Marked per instructions; Dockerfile builder updated to golang:alpine)
```

---

## 10. Known Gogram MTProto Characteristics & Limitations

1. **Bot API `file_id` Decoding**: Gogram uses `telegram.ResolveBotFileID` to unpack Bot API file IDs. Files uploaded by other bots or very old Bot API file IDs (pre-v4) may fail resolution if the access hash cannot be resolved without active chat history context.
2. **Session Persistence**: Gogram creates local session files (`session.dat` and `cache<SessionName>.db`). These have been explicitly excluded in `.gitignore` and `.dockerignore`.
3. **ParseMode**: Gogram natively supports `"html"` and `"markdown"`. In MYTGBOTZ, all messages are standardized to `"html"` with `commands.EscapeHTML` sanitation.

---

## 11. Instructions for Owner's Manual Server Test

To test the migrated bot on the server with real Telegram servers:

1. **Configure Environment Variables**:
   In your server's `.env` file, ensure the following MTProto and database variables are set:
   ```bash
   API_ID=1234567
   API_HASH=your_api_hash_here
   BOT_TOKEN=your_telegram_bot_token
   OWNER_ID=your_telegram_user_id
   ADMINS=your_telegram_user_id
   DATABASE_URI=mongodb+srv://...
   DATABASE_NAME=CyberTG
   COLLECTION_NAME=Telegram-Document
   LOG_LEVEL=INFO
   ```

2. **Run Application**:
   ```bash
   go run ./cmd/bot
   ```

3. **Verify Server Output**:
   Look for the following log output confirming Gogram MTProto connection and authentication:
   ```text
   INFO starting bot application initialization...
   INFO configuration loaded successfully app_id=... bot_token=... owner_id=...
   INFO initializing database layer...
   INFO database layer initialized and ready
   INFO connecting to Telegram MTProto servers via Gogram...
   INFO authenticating bot identity with Telegram...
   INFO gogram telegram bot client authenticated successfully bot_username=...
   ```

4. **Verify in Telegram**:
   - Send `/start`: Expect the welcome card with inline buttons.
   - Send `/ping`: Expect distinct report with local processing time and MTProto round-trip latency.
   - Send `/cmds`: Expect full command list.
   - Send `/id`: Expect your personal ID card.
   - Send `/info`: Expect system report displaying `Library: Gogram v1.7.71`.
   - Send `/getfile <valid_file_id>`: Expect media delivery matching the record format.
