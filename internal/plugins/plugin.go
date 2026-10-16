package plugins

import (
	"log/slog"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters"
	"github.com/itsafxal/MYTGBOTZ/internal/config"
	"github.com/itsafxal/MYTGBOTZ/internal/database"
	"github.com/itsafxal/MYTGBOTZ/internal/delivery"
	"github.com/itsafxal/MYTGBOTZ/internal/search"
)

// AppContext carries shared application dependencies passed to plugins.
type AppContext struct {
	Config    *config.Config
	Logger    *slog.Logger
	StartTime time.Time
	BotUser   *gotgbot.User
	Registry  *Registry
	DB        *database.DatabaseManager
	Delivery  delivery.MediaDeliveryService
	Search    search.SearchService
}

// HandlerFunc is the gotgbot command handler function signature.
type HandlerFunc = handlers.Response

// CommandDefinition defines a single command exposed by a plugin.
type CommandDefinition struct {
	// Name is the primary command trigger without a leading slash (e.g. "start").
	Name string

	// Aliases are alternative triggers that map to the same handler (e.g. ["help"]).
	Aliases []string

	// Description is a concise human-readable description of what the command does.
	Description string

	// Usage explains command syntax (e.g. "/start [payload]").
	Usage string

	// Category groups commands logically in generated guides (e.g. "General").
	Category string

	// AdminOnly specifies whether only bot admins/owner can execute this command.
	AdminOnly bool

	// Handler is the function executed when the command or its aliases are invoked.
	Handler HandlerFunc
}

// CallbackDefinition defines a callback query prefix handler.
type CallbackDefinition struct {
	// Prefix is the matching prefix for query data (e.g. "file#").
	Prefix string

	// Handler is the function executed when the callback query matches the prefix.
	Handler HandlerFunc
}

// Plugin is the contract that every modular plugin must satisfy.
type Plugin interface {
	// Name returns the unique, identifier name of the plugin (e.g. "start", "ping").
	Name() string

	// Commands returns all command definitions registered by this plugin.
	Commands() []CommandDefinition
}

// CallbackPlugin is an optional interface plugins can implement to register callback query handlers.
type CallbackPlugin interface {
	Plugin
	CallbackHandlers() []CallbackDefinition
}

// MessageDefinition defines a message handler with a filter.
type MessageDefinition struct {
	Filter  filters.Message
	Handler HandlerFunc
}

// MessagePlugin is an optional interface plugins can implement to register generic message handlers (e.g. search).
type MessagePlugin interface {
	Plugin
	MessageHandlers() []MessageDefinition
}
