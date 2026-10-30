package plugins

import (
	"context"
	"log/slog"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/MYTGBOTZ/internal/autodelete"
	"github.com/itsafxal/MYTGBOTZ/internal/caption"
	"github.com/itsafxal/MYTGBOTZ/internal/config"
	"github.com/itsafxal/MYTGBOTZ/internal/database"
	"github.com/itsafxal/MYTGBOTZ/internal/delivery"
	"github.com/itsafxal/MYTGBOTZ/internal/gfilter"
	"github.com/itsafxal/MYTGBOTZ/internal/search"
	"github.com/itsafxal/MYTGBOTZ/internal/stream"
)

// GFilterService defines the Global Filter operations available across plugins.
type GFilterService interface {
	MatchAndExecute(m *telegram.NewMessage) (bool, error)
	SaveFilter(ctx context.Context, f *gfilter.GFilter) error
	DeleteFilter(ctx context.Context, keyword string) (bool, error)
	DeleteAllFilters(ctx context.Context) (int64, error)
	GetFilter(ctx context.Context, keyword string) (*gfilter.GFilter, error)
	ListFilters(ctx context.Context, limit, offset int) ([]*gfilter.GFilter, int64, error)
	Count(ctx context.Context) (int64, error)
}

// FSubService defines the force subscribe operations required across plugins.
type FSubService interface {
	BuildDynamicFSubKeyboard(ctx context.Context, client *telegram.Client, userID int64, fileID string) (*telegram.ReplyInlineMarkup, []int64)
	BuildDynamicFSubKeyboardForChannels(ctx context.Context, client *telegram.Client, userID int64, targetChannels []int64) (*telegram.ReplyInlineMarkup, []int64)
	CheckLoopSubDynamic(ctx context.Context, client *telegram.Client, userID int64, chatID int64, promptMsgID int32, fileID string, requiredChannels ...int64) bool
	CheckChannelMembership(ctx context.Context, client *telegram.Client, userID int64, channelID int64, slot int) bool
	Init(ctx context.Context, client *telegram.Client, botID int64) error
}

// AppContext carries shared application dependencies passed to plugins.
type AppContext struct {
	Config     *config.Config
	Logger     *slog.Logger
	StartTime  time.Time
	BotUser    *telegram.UserObj
	Registry   *Registry
	DB         *database.DatabaseManager
	Delivery   delivery.MediaDeliveryService
	Search     search.SearchService
	Client     *telegram.Client
	FSub       FSubService
	Caption    caption.CaptionService
	AutoDelete autodelete.AutoDeleteService
	Stream     stream.StreamService
	GFilter    GFilterService
}


// HandlerFunc is the Gogram message/command handler function signature.
type HandlerFunc = telegram.MessageHandler

// CallbackHandlerFunc is the Gogram callback query handler function signature.
type CallbackHandlerFunc = telegram.CallbackHandler

// MessageFilterFunc is a predicate function for filtering messages.
type MessageFilterFunc func(m *telegram.NewMessage) bool

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
	Handler CallbackHandlerFunc
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
	Filter       MessageFilterFunc
	Handler      HandlerFunc
	AllowChannel bool
}

// MessagePlugin is an optional interface plugins can implement to register generic message handlers (e.g. search).
type MessagePlugin interface {
	Plugin
	MessageHandlers() []MessageDefinition
}

// JoinRequestHandlerFunc is the handler function for Telegram join requests.
type JoinRequestHandlerFunc func(req *telegram.JoinRequestUpdate) error

// JoinRequestPlugin is an optional interface plugins can implement to receive chat join requests.
type JoinRequestPlugin interface {
	Plugin
	JoinRequestHandlers() []JoinRequestHandlerFunc
}

