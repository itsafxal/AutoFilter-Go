package delivery

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/amarnathcjd/gogram/telegram"
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
func (p *Plugin) HandleGetFile(m *telegram.NewMessage) error {
	user := m.Sender

	args := strings.Fields(m.Text())
	if len(args) < 2 {
		_, err := m.Reply("ℹ️ <b>Usage:</b> <code>/getfile &lt;file_id&gt;</code>\n\nPlease provide a valid media identifier.", &telegram.SendOptions{
			ParseMode: "html",
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
		_, err := m.Reply("⚠️ <b>Service Unavailable:</b> Media delivery service is not ready.", &telegram.SendOptions{
			ParseMode: "html",
		})
		return err
	}

	var userID int64
	if user != nil {
		userID = user.ID
	} else {
		userID = m.SenderID()
	}

	opts := deliveryCore.DeliveryOptions{
		UserID:           userID,
		ProtectContent:   protectContent,
		ReplyToMessageID: int64(m.ID),
	}

	res, err := p.appCtx.Delivery.Deliver(context.Background(), m.ChatID(), fileID, opts)
	if err == nil {
		if res != nil && res.MessageID != 0 && p.appCtx != nil && p.appCtx.AutoDelete != nil {
			_ = p.appCtx.AutoDelete.ScheduleFileDelete(m.ChatID(), int32(res.MessageID))
		}
		return nil
	}

	// Safe user-friendly error replies
	if errors.Is(err, deliveryCore.ErrMediaNotFound) {
		_, replyErr := m.Reply("⚠️ <b>File not found or has been removed.</b>", &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}
	if errors.Is(err, deliveryCore.ErrUserBanned) {
		_, replyErr := m.Reply("⛔ <b>Access Denied:</b> You are suspended from receiving files.", &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}
	if errors.Is(err, deliveryCore.ErrChatDisabled) {
		_, replyErr := m.Reply("⛔ <b>Access Denied:</b> This chat is disabled for media delivery.", &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}
	if errors.Is(err, deliveryCore.ErrDuplicateRequest) {
		_, replyErr := m.Reply("⏳ <b>Please wait:</b> A delivery request for this file is already in progress.", &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}
	if errors.Is(err, deliveryCore.ErrRateLimited) {
		_, replyErr := m.Reply("⚠️ <b>Flood Control:</b> Telegram rate limit reached. Please wait a moment before trying again.", &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}
	if errors.Is(err, deliveryCore.ErrBlockedByUser) {
		return nil
	}

	_, replyErr := m.Reply("❌ <b>Delivery Error:</b> Failed to deliver requested file. Please check the ID and try again.", &telegram.SendOptions{ParseMode: "html"})
	return replyErr
}

func (p *Plugin) isMaintenanceMode() bool {
	return p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.MaintenanceMode
}

// HandleFileCallback redirects users clicking a file button in a group/channel to private chat.
// Format: file#<file_id>
func (p *Plugin) HandleFileCallback(cb *telegram.CallbackQuery) error {
	if cb == nil {
		return nil
	}

	if p.isMaintenanceMode() {
		isAdmin := p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.IsAdmin(cb.SenderID)
		if !isAdmin {
			_, err := cb.Answer("⚠️ Bot is currently under maintenance. Please try again later.", &telegram.CallbackOptions{
				Alert: true,
			})
			return err
		}
	}

	botUsername := "bot"
	if p.appCtx != nil && p.appCtx.BotUser != nil && p.appCtx.BotUser.Username != "" {
		botUsername = p.appCtx.BotUser.Username
	}

	data := cb.DataString()
	parts := strings.SplitN(data, "#", 2)
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		_, err := cb.Answer("⚠️ Invalid file reference", &telegram.CallbackOptions{
			Alert: true,
		})
		return err
	}

	fileID := strings.TrimSpace(parts[1])
	startURL := fmt.Sprintf("https://t.me/%s?start=file_%s", botUsername, fileID)

	_, err := cb.Answer("", &telegram.CallbackOptions{
		URL: startURL,
	})
	return err
}

// HandleCheckSubCallback processes direct delivery requests from callback buttons.
// Format: checksub#<file_id> or checksubp#<file_id>
func (p *Plugin) HandleCheckSubCallback(cb *telegram.CallbackQuery) error {
	if cb == nil {
		return nil
	}

	if p.isMaintenanceMode() {
		userID := cb.SenderID
		if userID == 0 && cb.Sender != nil {
			userID = cb.Sender.ID
		}
		isAdmin := p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.IsAdmin(userID)
		if !isAdmin {
			_, err := cb.Answer("⚠️ Bot is currently under maintenance. Please try again later.", &telegram.CallbackOptions{
				Alert: true,
			})
			return err
		}
	}

	data := cb.DataString()
	parts := strings.SplitN(data, "#", 2)
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		_, err := cb.Answer("⚠️ Invalid file reference", &telegram.CallbackOptions{
			Alert: true,
		})
		return err
	}

	fileID := strings.TrimSpace(parts[1])
	protectContent := strings.HasPrefix(parts[0], "checksubp")

	if p.appCtx == nil || p.appCtx.Delivery == nil {
		_, err := cb.Answer("⚠️ Delivery service unavailable", &telegram.CallbackOptions{
			Alert: true,
		})
		return err
	}

	userID := cb.SenderID
	if userID == 0 && cb.Sender != nil {
		userID = cb.Sender.ID
	}

	// Verify Force Subscribe status matching legacy pm_filter checksub logic
	if p.appCtx != nil && p.appCtx.FSub != nil {
		isAdmin := false
		if p.appCtx.Config != nil {
			isAdmin = p.appCtx.Config.IsAdmin(userID) || p.appCtx.Config.IsOwner(userID)
		}
		if !isAdmin {
			payload := "file_" + fileID
			if protectContent {
				payload = "filep_" + fileID
			}
			markup, unjoined := p.appCtx.FSub.BuildDynamicFSubKeyboard(context.Background(), p.appCtx.Client, userID, payload)
			if len(unjoined) > 0 {
				if cb.MessageID != 0 && markup != nil && p.appCtx.Client != nil {
					_, _ = p.appCtx.Client.EditMessage(cb.GetChatID(), cb.MessageID, "<b>Pʟᴇᴀꜱᴇ Jᴏɪɴ Oᴜʀ Cʜᴀɴɴᴇʟs Tᴏ Gᴇᴛ Yᴏᴜʀ Rᴇǫᴜᴇꜱᴛᴇᴅ Mᴏᴠɪᴇ Fɪʟᴇ 🚀</b>", &telegram.SendOptions{
						ParseMode:   "html",
						ReplyMarkup: markup,
					})
				}
				_, alertErr := cb.Answer("🚨 You haven't joined all required channels yet! Joined channel buttons have been hidden. Please join the remaining channel(s) shown above.", &telegram.CallbackOptions{
					Alert: true,
				})
				return alertErr
			}
		}
	}

	opts := deliveryCore.DeliveryOptions{
		UserID:         userID,
		ProtectContent: protectContent,
	}

	_, err := p.appCtx.Delivery.Deliver(context.Background(), userID, fileID, opts)
	if err == nil {
		_, answerErr := cb.Answer("✅ Sending file...")
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

	_, answerErr := cb.Answer(alertText, &telegram.CallbackOptions{
		Alert: true,
	})
	return answerErr
}
