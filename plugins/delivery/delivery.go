package delivery

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	deliveryCore "github.com/itsafxal/MYTGBOTZ/internal/delivery"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
)

// Plugin implements the media delivery commands and callback query handlers.
type Plugin struct {
	appCtx *plugins.AppContext
}

// New creates a new delivery plugin instance.
func New(appCtx *plugins.AppContext) *Plugin {
	return &Plugin{appCtx: appCtx}
}

// Name returns the unique plugin name.
func (p *Plugin) Name() string {
	return "delivery"
}

// Commands returns the command definitions for the delivery plugin.
func (p *Plugin) Commands() []plugins.CommandDefinition {
	return []plugins.CommandDefinition{
		{
			Name:        "getfile",
			Aliases:     []string{"file"},
			Description: "Retrieve and deliver a media file by its unique identifier",
			Usage:       "/getfile <file_id>",
			Category:    "Media",
			Handler:     p.HandleGetFile,
		},
	}
}

// CallbackHandlers returns the callback query handlers for media redirection and direct delivery.
func (p *Plugin) CallbackHandlers() []plugins.CallbackDefinition {
	return []plugins.CallbackDefinition{
		{
			Prefix:  "file#",
			Handler: p.HandleFileCallback,
		},
		{
			Prefix:  "checksub#",
			Handler: p.HandleCheckSubCallback,
		},
	}
}

// HandleGetFile processes the /getfile command.
func (p *Plugin) HandleGetFile(b *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	chat := ctx.EffectiveChat
	user := ctx.EffectiveSender.User

	args := strings.Fields(msg.Text)
	if len(args) < 2 {
		_, err := msg.Reply(b, "ℹ️ <b>Usage:</b> <code>/getfile &lt;file_id&gt;</code>\n\nPlease provide a valid media identifier.", &gotgbot.SendMessageOpts{
			ParseMode: "HTML",
		})
		return err
	}

	rawID := strings.TrimSpace(args[1])
	prefix := ""
	fileID := rawID
	protectContent := false

	if strings.Contains(rawID, "_") {
		parts := strings.SplitN(rawID, "_", 2)
		prefix = parts[0]
		fileID = parts[1]
		if prefix == "filep" {
			protectContent = true
		}
	}

	if p.appCtx == nil || p.appCtx.Delivery == nil {
		_, err := msg.Reply(b, "⚠️ <b>Service Unavailable:</b> Media delivery service is not ready.", &gotgbot.SendMessageOpts{
			ParseMode: "HTML",
		})
		return err
	}

	var userID int64
	if user != nil {
		userID = user.Id
	}

	opts := deliveryCore.DeliveryOptions{
		UserID:           userID,
		ProtectContent:   protectContent,
		ReplyToMessageID: msg.MessageId,
	}

	_, err := p.appCtx.Delivery.Deliver(context.Background(), chat.Id, fileID, opts)
	if err == nil {
		return nil
	}

	// Safe user-friendly error replies
	if errors.Is(err, deliveryCore.ErrMediaNotFound) {
		_, replyErr := msg.Reply(b, "⚠️ <b>File not found or has been removed.</b>", &gotgbot.SendMessageOpts{ParseMode: "HTML"})
		return replyErr
	}
	if errors.Is(err, deliveryCore.ErrUserBanned) {
		_, replyErr := msg.Reply(b, "⛔ <b>Access Denied:</b> You are suspended from receiving files.", &gotgbot.SendMessageOpts{ParseMode: "HTML"})
		return replyErr
	}
	if errors.Is(err, deliveryCore.ErrChatDisabled) {
		_, replyErr := msg.Reply(b, "⛔ <b>Access Denied:</b> This chat is disabled for media delivery.", &gotgbot.SendMessageOpts{ParseMode: "HTML"})
		return replyErr
	}
	if errors.Is(err, deliveryCore.ErrDuplicateRequest) {
		_, replyErr := msg.Reply(b, "⏳ <b>Please wait:</b> A delivery request for this file is already in progress.", &gotgbot.SendMessageOpts{ParseMode: "HTML"})
		return replyErr
	}
	if errors.Is(err, deliveryCore.ErrRateLimited) {
		_, replyErr := msg.Reply(b, "⚠️ <b>Flood Control:</b> Telegram rate limit reached. Please wait a moment before trying again.", &gotgbot.SendMessageOpts{ParseMode: "HTML"})
		return replyErr
	}
	if errors.Is(err, deliveryCore.ErrBlockedByUser) {
		return nil
	}

	_, replyErr := msg.Reply(b, "❌ <b>Delivery Error:</b> Failed to deliver requested file. Please check the ID and try again.", &gotgbot.SendMessageOpts{ParseMode: "HTML"})
	return replyErr
}

// HandleFileCallback redirects users clicking a file button in a group/channel to private chat.
// Format: file#<file_id>
func (p *Plugin) HandleFileCallback(b *gotgbot.Bot, ctx *ext.Context) error {
	cb := ctx.Update.CallbackQuery
	if cb == nil {
		return nil
	}

	botUsername := b.User.Username
	if botUsername == "" {
		botUsername = "bot"
	}

	parts := strings.SplitN(cb.Data, "#", 2)
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		_, err := cb.Answer(b, &gotgbot.AnswerCallbackQueryOpts{
			Text:      "⚠️ Invalid file reference",
			ShowAlert: true,
		})
		return err
	}

	fileID := strings.TrimSpace(parts[1])
	startURL := fmt.Sprintf("https://t.me/%s?start=file_%s", botUsername, fileID)

	_, err := cb.Answer(b, &gotgbot.AnswerCallbackQueryOpts{
		Url: startURL,
	})
	return err
}

// HandleCheckSubCallback processes direct delivery requests from callback buttons.
// Format: checksub#<file_id> or checksubp#<file_id>
func (p *Plugin) HandleCheckSubCallback(b *gotgbot.Bot, ctx *ext.Context) error {
	cb := ctx.Update.CallbackQuery
	if cb == nil {
		return nil
	}

	parts := strings.SplitN(cb.Data, "#", 2)
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		_, err := cb.Answer(b, &gotgbot.AnswerCallbackQueryOpts{
			Text:      "⚠️ Invalid file reference",
			ShowAlert: true,
		})
		return err
	}

	fileID := strings.TrimSpace(parts[1])
	protectContent := strings.HasPrefix(parts[0], "checksubp")

	if p.appCtx == nil || p.appCtx.Delivery == nil {
		_, err := cb.Answer(b, &gotgbot.AnswerCallbackQueryOpts{
			Text:      "⚠️ Delivery service unavailable",
			ShowAlert: true,
		})
		return err
	}

	opts := deliveryCore.DeliveryOptions{
		UserID:         cb.From.Id,
		ProtectContent: protectContent,
	}

	_, err := p.appCtx.Delivery.Deliver(context.Background(), cb.From.Id, fileID, opts)
	if err == nil {
		_, answerErr := cb.Answer(b, &gotgbot.AnswerCallbackQueryOpts{
			Text: "✅ Sending file...",
		})
		return answerErr
	}

	alertText := "❌ Failed to deliver file"
	if errors.Is(err, deliveryCore.ErrMediaNotFound) {
		alertText = "⚠️ File not found or has been removed."
	} else if errors.Is(err, deliveryCore.ErrUserBanned) {
		alertText = "⛔ Your account is suspended from receiving files."
	} else if errors.Is(err, deliveryCore.ErrDuplicateRequest) {
		alertText = "⏳ File delivery is already in progress. Please wait."
	} else if errors.Is(err, deliveryCore.ErrRateLimited) {
		alertText = "⚠️ Rate limited. Please try again shortly."
	}

	_, answerErr := cb.Answer(b, &gotgbot.AnswerCallbackQueryOpts{
		Text:      alertText,
		ShowAlert: true,
	})
	return answerErr
}
