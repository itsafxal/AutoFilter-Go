# AutoFilter-Go Plugin Architecture

This guide explains the compile-time plugin registry, architecture, and step-by-step developer instructions for extending **AutoFilter-Go**.

---

## 1. Architectural Philosophy: Compile-Time Registry

Unlike systems that rely on dynamic shared objects (`.so` plugins or `plugin.Open`) which are non-portable across operating systems and disable static linking, AutoFilter-Go uses a **Compile-Time Plugin Architecture** (`internal/plugins`):

- **100% Statically Linked**: Compiles into a single standalone binary with `CGO_ENABLED=0`.
- **Zero Runtime Reflection Overhead**: Handlers are registered and validated during application startup.
- **Strict Type Safety**: Handlers receive strongly-typed Gogram MTProto events (`*telegram.NewMessage`, `*telegram.CallbackQuery`).
- **Dependency Injection**: Every plugin receives a shared `AppContext` pointer providing thread-safe access to database managers, config, loggers, and delivery engines.

---

## 2. Plugin Interfaces & Contracts

Every plugin implements the `plugins.Plugin` interface defined in `internal/plugins/plugin.go`:

```go
type Plugin interface {
    Name() string
    Commands() []CommandDefinition
}

// Optional interface extensions:
type MessageHandlerPlugin interface {
    MessageHandlers() []MessageDefinition
}

type CallbackHandlerPlugin interface {
    CallbackHandlers() []CallbackDefinition
}
```

### Definitions

#### 1. CommandDefinition
```go
type CommandDefinition struct {
    Name        string                                  // e.g. "ping"
    Aliases     []string                                // e.g. ["latency"]
    Description string                                  // Displayed in /cmds
    Usage       string                                  // e.g. "/ping"
    Category    string                                  // e.g. "General", "Admin", "Media"
    AdminOnly   bool                                    // Restricts execution to Admins/Owner
    Handler     func(m *telegram.NewMessage) error     // Command execution handler
}
```

#### 2. MessageDefinition
```go
type MessageDefinition struct {
    AllowChannel bool                                   // Whether to process channel posts
    Filter       func(m *telegram.NewMessage) bool      // Predicate filter function
    Handler      func(m *telegram.NewMessage) error     // Message processor
}
```

#### 3. CallbackDefinition
```go
type CallbackDefinition struct {
    Prefix  string                                      // Data prefix match (e.g. "file#", "fsub#")
    Exact   string                                      // Exact data match (e.g. "close_data")
    Handler func(cb *telegram.CallbackQuery) error      // Interactive button handler
}
```

---

## 3. The `AppContext` Container

Plugins receive an `AppContext` struct containing all core dependencies:

```go
type AppContext struct {
    Config            *config.Config
    Logger            *slog.Logger
    DB                *database.DatabaseManager
    Delivery          *delivery.DeliveryService
    Search            search.SearchService
    Client            *telegram.Client
    StreamServer      *stream.Server
    OldBotService     *oldbot.Service
    AutoDeleteService autodelete.AutoDeleteService
    FSubService       fsub.FSubService
}
```

---

## 4. How to Create a New Plugin (Step-by-Step)

### Step 1: Create the Plugin Package
Create a new directory under `plugins/`, for example `plugins/echo`:

```go
// plugins/echo/echo.go
package echo

import (
    "strings"
    "github.com/amarnathcjd/gogram/telegram"
    "github.com/itsafxal/AutoFilter-Go/internal/plugins"
)

type Plugin struct {
    appCtx *plugins.AppContext
}

func New(appCtx *plugins.AppContext) *Plugin {
    return &Plugin{appCtx: appCtx}
}

func (p *Plugin) Name() string {
    return "echo"
}

func (p *Plugin) Commands() []plugins.CommandDefinition {
    return []plugins.CommandDefinition{
        {
            Name:        "echo",
            Description: "Echoes back the provided text",
            Usage:       "/echo <text>",
            Category:    "General",
            AdminOnly:   false,
            Handler:     p.HandleEcho,
        },
    }
}

func (p *Plugin) HandleEcho(m *telegram.NewMessage) error {
    text := strings.TrimSpace(m.Args())
    if text == "" {
        _, err := m.Reply("Please provide text to echo! Usage: <code>/echo hello</code>")
        return err
    }
    _, err := m.Reply("📣 " + text)
    return err
}
```

### Step 2: Register in `cmd/bot/main.go`
Open `cmd/bot/main.go` and import your package:
```go
import (
    // ... existing imports ...
    "github.com/itsafxal/AutoFilter-Go/plugins/echo"
)
```

In the `run()` function, add your plugin to `application.RegisterPlugins`:
```go
err = application.RegisterPlugins(
    start.New(application.AppContext),
    // ... existing plugins ...
    echo.New(application.AppContext),
)
```

### Step 3: Verify and Build
```bash
go vet ./...
go build -o ./bin/autofilter-go ./cmd/bot
```
The new command `/echo` is now automatically recognized, security-checked, and added to the dynamic `/cmds` list.

---

## 5. Core Built-In Plugins Reference

| Plugin | Path | Description |
| :--- | :--- | :--- |
| `start` | `plugins/start` | Handles `/start`, deep-link payload routing (`file_`, `filep_`, `from_oldbot`), and welcome banners. |
| `cmds` | `plugins/cmds` | Dynamically generates `/cmds` and administrative `/cmdc` categorized menus. |
| `ping` | `plugins/ping` | Measures bot processing speed and MTProto round-trip latency. |
| `id` | `plugins/id` | Displays numerical IDs for users, chats, and replied-to messages. |
| `info` | `plugins/info` | Diagnostic overview of bot uptime, Gogram version, and user profiles. |
| `delivery` | `plugins/delivery` | Handles `/getfile`, direct file delivery, and file callback data. |
| `autofilter` | `plugins/autofilter` | Ambient text search, spelling suggestions, quality/language filters, and pagination. |
| `fsub` | `plugins/fsub` | 3-tier Force-Subscribe channel management, routing thresholds, and membership validation. |
| `caption` | `plugins/caption` | Administrative custom media caption template editor (`/set_caption`, `/view_caption`, `/del_caption`). |
| `latest` | `plugins/latest` | Displays recently added media files (`/latest`) and accepts user requests (`/request`). |
| `autodelete` | `plugins/autodelete` | Scheduled message deletion and weekly copyright trash cleaner (`/autodel`, `/clean_trash`). |
| `stream` | `plugins/stream` | Generates direct HTTP streaming and fast download links (`/fastdl`, `/stream`). |
| `channel` | `plugins/channel` | Background watcher for monitored channels that executes live media indexing. |
| `index` | `plugins/index` | Bulk channel media indexing (`/index`, `/indexlink`, `/setskip`). |
| `botstats` | `plugins/botstats` | System and database diagnostics (`/stats`) and SQLite compaction (`/vacuum`). |
| `gfilter` | `plugins/gfilter` | Global group keyword auto-replies with inline button formatting (`/gfilter`, `/gfilters`). |

