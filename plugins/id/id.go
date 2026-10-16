package id

import (
	"fmt"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/itsafxal/MYTGBOTZ/internal/commands"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
)

// Plugin implements the /id command plugin.
type Plugin struct {
	appCtx *plugins.AppContext
}

// New creates a new id plugin instance.
func New(appCtx *plugins.AppContext) *Plugin {
	return &Plugin{appCtx: appCtx}
}

// Name returns the unique plugin name.
func (p *Plugin) Name() string {
	return "id"
}

// Commands returns the command definitions for id plugin.
func (p *Plugin) Commands() []plugins.CommandDefinition {
	return []plugins.CommandDefinition{
		{
			Name:        "id",
			Description: "Display current chat ID and user Telegram ID",
			Usage:       "/id",
			Category:    "General",
			Handler:     p.HandleID,
		},
	}
}

// FormatPrivateID creates formatted ID card for private chats.
func FormatPrivateID(user *gotgbot.User, chatID int64) string {
	firstName := "N/A"
	lastName := "N/A"
	username := "N/A"
	userIDStr := "N/A"

	if user != nil {
		if user.FirstName != "" {
			firstName = commands.EscapeHTML(user.FirstName)
		}
		if user.LastName != "" {
			lastName = commands.EscapeHTML(user.LastName)
		}
		if user.Username != "" {
			username = "@" + commands.EscapeHTML(user.Username)
		}
		userIDStr = fmt.Sprintf("<code>%d</code>", user.Id)
	}

	return fmt.Sprintf(
		"👤 <b>Personal ID Card</b>\n\n"+
			"────────────────────\n"+
			"📝 <b>First Name:</b> %s\n"+
			"📝 <b>Last Name:</b> %s\n"+
			"✨ <b>Username:</b> %s\n"+
			"🆔 <b>Telegram ID:</b> %s\n"+
			"💬 <b>Chat ID:</b> <code>%d</code>\n"+
			"────────────────────",
		firstName,
		lastName,
		username,
		userIDStr,
		chatID,
	)
}

// FormatGroupID creates formatted ID report for group/supergroup chats.
func FormatGroupID(chat *gotgbot.Chat, user *gotgbot.User, replyMsg *gotgbot.Message) string {
	chatID := int64(0)
	chatTitle := "Unknown"
	if chat != nil {
		chatID = chat.Id
		if chat.Title != "" {
			chatTitle = commands.EscapeHTML(chat.Title)
		}
	}

	userIDStr := "Anonymous"
	if user != nil {
		userIDStr = fmt.Sprintf("<code>%d</code>", user.Id)
	}

	var sb strings.Builder
	sb.WriteString("📊 <b>IDs Information</b>\n\n")
	sb.WriteString(fmt.Sprintf("💬 <b>Chat ID:</b> <code>%d</code>\n", chatID))
	sb.WriteString(fmt.Sprintf("🏷️ <b>Chat Title:</b> %s\n", chatTitle))
	sb.WriteString(fmt.Sprintf("👤 <b>Your User ID:</b> %s\n", userIDStr))

	if replyMsg != nil {
		repliedUserIDStr := "Anonymous"
		if replyMsg.From != nil {
			repliedUserIDStr = fmt.Sprintf("<code>%d</code>", replyMsg.From.Id)
		} else if replyMsg.SenderChat != nil {
			repliedUserIDStr = fmt.Sprintf("<code>%d</code> (Channel)", replyMsg.SenderChat.Id)
		}
		sb.WriteString(fmt.Sprintf("↩️ <b>Replied User ID:</b> %s\n", repliedUserIDStr))
		sb.WriteString(fmt.Sprintf("📨 <b>Replied Message ID:</b> <code>%d</code>\n", replyMsg.MessageId))
	}

	sb.WriteString("────────────────────")
	return sb.String()
}

// HandleID handles the /id command.
func (p *Plugin) HandleID(b *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	chat := ctx.EffectiveChat

	var user *gotgbot.User
	if ctx.EffectiveSender != nil {
		user = ctx.EffectiveSender.User
	}

	var report string
	if chat.Type == "private" {
		report = FormatPrivateID(user, chat.Id)
	} else {
		report = FormatGroupID(chat, user, msg.ReplyToMessage)
	}

	_, err := msg.Reply(b, report, &gotgbot.SendMessageOpts{
		ParseMode: "HTML",
	})
	return err
}
