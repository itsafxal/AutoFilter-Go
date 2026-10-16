package info

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/itsafxal/MYTGBOTZ/internal/commands"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
)

// Plugin implements the /info and /about command plugin.
type Plugin struct {
	appCtx *plugins.AppContext
}

// New creates a new info plugin instance.
func New(appCtx *plugins.AppContext) *Plugin {
	return &Plugin{appCtx: appCtx}
}

// Name returns the unique plugin name.
func (p *Plugin) Name() string {
	return "info"
}

// Commands returns the command definitions for info plugin.
func (p *Plugin) Commands() []plugins.CommandDefinition {
	return []plugins.CommandDefinition{
		{
			Name:        "info",
			Aliases:     []string{"about"},
			Description: "Display available bot information and user profile details",
			Usage:       "/info",
			Category:    "General",
			Handler:     p.HandleInfo,
		},
	}
}

// BuildBotInfoReport builds a safe bot overview without exposing secrets or fake stats.
func BuildBotInfoReport(botUser *gotgbot.User, startTime time.Time) string {
	botName := "Bot"
	botUsername := "N/A"
	botID := int64(0)
	if botUser != nil {
		botName = commands.EscapeHTML(botUser.FirstName)
		if botUser.Username != "" {
			botUsername = "@" + commands.EscapeHTML(botUser.Username)
		}
		botID = botUser.Id
	}

	uptime := commands.FormatUptime(startTime)
	goVer := runtime.Version()

	return fmt.Sprintf(
		"🤖 <b>Bot Information</b>\n\n"+
			"────────────────────\n"+
			"📛 <b>Bot Name:</b> %s\n"+
			"✨ <b>Username:</b> %s\n"+
			"🆔 <b>Bot ID:</b> <code>%d</code>\n"+
			"📚 <b>Library:</b> <code>gotgbot v2.0.0-rc.36</code>\n"+
			"💻 <b>Language:</b> <code>%s</code>\n"+
			"⏱️ <b>Uptime:</b> <code>%s</code>\n"+
			"────────────────────",
		botName,
		botUsername,
		botID,
		goVer,
		uptime,
	)
}

// BuildUserInfoReport builds safe user profile details.
func BuildUserInfoReport(user *gotgbot.User) string {
	if user == nil {
		return "⚠️ <b>User Profile Details:</b> <i>User information not available.</i>"
	}

	firstName := commands.EscapeHTML(user.FirstName)
	lastName := "<i>Not Set</i>"
	if user.LastName != "" {
		lastName = commands.EscapeHTML(user.LastName)
	}

	username := "<i>Not Set</i>"
	if user.Username != "" {
		username = "@" + commands.EscapeHTML(user.Username)
	}

	return fmt.Sprintf(
		"✨ <b>User Profile Details</b> ✨\n\n"+
			"────────────────────\n"+
			"📝 <b>First Name:</b> %s\n"+
			"📝 <b>Last Name:</b> %s\n"+
			"🆔 <b>Telegram ID:</b> <code>%d</code>\n"+
			"✨ <b>Username:</b> %s\n"+
			"🔗 <b>User Link:</b> <a href=\"tg://user?id=%d\">Click Here</a>\n"+
			"────────────────────",
		firstName,
		lastName,
		user.Id,
		username,
		user.Id,
	)
}

// HandleInfo handles the /info and /about command.
func (p *Plugin) HandleInfo(b *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage

	// If replying to a user's message, display target user info
	if msg.ReplyToMessage != nil && msg.ReplyToMessage.From != nil {
		userReport := BuildUserInfoReport(msg.ReplyToMessage.From)
		_, err := msg.Reply(b, userReport, &gotgbot.SendMessageOpts{
			ParseMode: "HTML",
		})
		return err
	}

	// Otherwise, display bot info
	var startTime time.Time
	if p.appCtx != nil {
		startTime = p.appCtx.StartTime
	}

	botReport := BuildBotInfoReport(&b.User, startTime)
	var sb strings.Builder
	sb.WriteString(botReport)

	// If in private chat, append requesting user details
	if ctx.EffectiveChat.Type == "private" && ctx.EffectiveSender != nil && ctx.EffectiveSender.User != nil {
		sb.WriteString("\n\n")
		sb.WriteString(BuildUserInfoReport(ctx.EffectiveSender.User))
	}

	_, err := msg.Reply(b, sb.String(), &gotgbot.SendMessageOpts{
		ParseMode: "HTML",
	})
	return err
}
