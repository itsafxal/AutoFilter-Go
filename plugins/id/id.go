package id

import (
	"fmt"
	"strings"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/AutoFilter-Go/internal/commands"
	"github.com/itsafxal/AutoFilter-Go/internal/plugins"
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
func FormatPrivateID(user *telegram.UserObj, chatID int64) string {
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
		userIDStr = fmt.Sprintf("<code>%d</code>", user.ID)
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
func FormatGroupID(chatID int64, chatTitle string, user *telegram.UserObj, replyMsg *telegram.NewMessage) string {
	if chatTitle == "" {
		chatTitle = "Unknown"
	} else {
		chatTitle = commands.EscapeHTML(chatTitle)
	}

	userIDStr := "Anonymous"
	if user != nil {
		userIDStr = fmt.Sprintf("<code>%d</code>", user.ID)
	}

	var sb strings.Builder
	sb.WriteString("📊 <b>IDs Information</b>\n\n")
	sb.WriteString(fmt.Sprintf("💬 <b>Chat ID:</b> <code>%d</code>\n", chatID))
	sb.WriteString(fmt.Sprintf("🏷️ <b>Chat Title:</b> %s\n", chatTitle))
	sb.WriteString(fmt.Sprintf("👤 <b>Your User ID:</b> %s\n", userIDStr))

	if replyMsg != nil {
		repliedUserIDStr := "Anonymous"
		if replyMsg.Sender != nil {
			repliedUserIDStr = fmt.Sprintf("<code>%d</code>", replyMsg.Sender.ID)
		} else if replyMsg.SenderChat != nil {
			repliedUserIDStr = fmt.Sprintf("<code>%d</code> (Channel)", replyMsg.SenderChat.ID)
		} else if replyMsg.SenderID() != 0 {
			repliedUserIDStr = fmt.Sprintf("<code>%d</code>", replyMsg.SenderID())
		}
		sb.WriteString(fmt.Sprintf("↩️ <b>Replied User ID:</b> %s\n", repliedUserIDStr))
		sb.WriteString(fmt.Sprintf("📨 <b>Replied Message ID:</b> <code>%d</code>\n", replyMsg.ID))
	}

	sb.WriteString("────────────────────")
	return sb.String()
}

// HandleID handles the /id command.
func (p *Plugin) HandleID(m *telegram.NewMessage) error {
	user := m.Sender
	chatID := m.ChatID()

	var report string
	if m.IsPrivate() {
		report = FormatPrivateID(user, chatID)
	} else {
		chatTitle := "Unknown"
		if m.Channel != nil && m.Channel.Title != "" {
			chatTitle = m.Channel.Title
		} else if m.Chat != nil && m.Chat.Title != "" {
			chatTitle = m.Chat.Title
		}
		var replyMsg *telegram.NewMessage
		if m.IsReply() {
			reply, err := m.GetReplyMessage()
			if err == nil {
				replyMsg = reply
			}
		}
		report = FormatGroupID(chatID, chatTitle, user, replyMsg)
	}

	_, err := m.Reply(report, &telegram.SendOptions{
		ParseMode: "html",
	})
	return err
}
