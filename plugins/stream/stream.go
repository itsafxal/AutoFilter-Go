package stream

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/AutoFilter-Go/internal/autodelete"
	"github.com/itsafxal/AutoFilter-Go/internal/commands"
	"github.com/itsafxal/AutoFilter-Go/internal/plugins"
)

// Plugin implements Fast Download commands and button callback handlers.
type Plugin struct {
	appCtx *plugins.AppContext
}

// New creates a new fast download plugin instance.
func New(appCtx *plugins.AppContext) *Plugin {
	return &Plugin{appCtx: appCtx}
}

// Name returns the unique plugin name.
func (p *Plugin) Name() string {
	return "stream"
}

// Commands returns the command definitions for fast download.
func (p *Plugin) Commands() []plugins.CommandDefinition {
	return []plugins.CommandDefinition{
		{
			Name:        "fastdl",
			Aliases:     []string{"link", "dl", "fastdownload", "stream"},
			Description: "Generate high-speed direct download link for a file",
			Usage:       "/fastdl <file_id> (or reply to a file with /fastdl)",
			Category:    "Media",
			Handler:     p.HandleFastDownloadCommand,
		},
	}
}

// CallbackHandlers returns the callback query handlers for fast download.
func (p *Plugin) CallbackHandlers() []plugins.CallbackDefinition {
	return []plugins.CallbackDefinition{
		{
			Prefix:  "fast_download#",
			Handler: p.HandleFastDownloadCallback,
		},
		{
			Prefix:  "fastdl#",
			Handler: p.HandleFastDownloadCallback,
		},
		{
			Prefix:  "stream_links#",
			Handler: p.HandleFastDownloadCallback,
		},
		{
			Prefix:  "close_data",
			Handler: p.HandleCloseCallback,
		},
	}
}

// HandleFastDownloadCommand processes /fastdl, /link, /dl, and /stream commands.
func (p *Plugin) HandleFastDownloadCommand(m *telegram.NewMessage) error {
	if p.appCtx == nil || p.appCtx.Stream == nil || p.appCtx.Delivery == nil {
		_, err := m.Reply("⚠️ <b>Service Unavailable:</b> Fast download service is not ready.", &telegram.SendOptions{
			ParseMode: "html",
		})
		return err
	}

	args := strings.Fields(m.Text())
	var fileID string

	if len(args) >= 2 {
		raw := strings.TrimSpace(args[1])
		if strings.HasPrefix(raw, "file_") {
			fileID = strings.TrimPrefix(raw, "file_")
		} else if strings.HasPrefix(raw, "filep_") {
			fileID = strings.TrimPrefix(raw, "filep_")
		} else {
			fileID = raw
		}
	} else if m.IsReply() {
		replyMsg, err := m.GetReplyMessage()
		if err == nil && replyMsg != nil && replyMsg.IsMedia() {
			botFileID := telegram.PackBotFileID(replyMsg.Media())
			if botFileID != "" {
				fileID = botFileID
			}
		}
	}

	if fileID == "" {
		_, err := m.Reply("ℹ️ <b>Usage:</b> <code>/fastdl &lt;file_id&gt;</code>\n\nOr reply to any media file with <code>/fastdl</code> to generate a fast download link.", &telegram.SendOptions{
			ParseMode: "html",
		})
		return err
	}

	// Lookup media record
	rec, err := p.appCtx.Delivery.GetMedia(context.Background(), fileID)
	if err != nil {
		_, replyErr := m.Reply("⚠️ <b>File not found or has been removed.</b>", &telegram.SendOptions{
			ParseMode: "html",
		})
		return replyErr
	}

	fileName := rec.FileName
	if fileName == "" {
		fileName = "Media_" + fileID
	}
	sizeStr := rec.Size
	if sizeStr == "" && rec.FileSize > 0 {
		sizeStr = commands.FormatBytes(rec.FileSize)
	}

	linkExpiry := p.getFastDownloadLinkExpiry()
	msgDeleteDur := p.getFastDownloadMsgDeleteDuration()

	downloadURL := p.appCtx.Stream.GetExpiringDownloadURL(fileID, fileName, linkExpiry)
	text := formatFastDownloadText(fileName, sizeStr, downloadURL, msgDeleteDur, linkExpiry)

	replyMarkup := p.buildFastDownloadKeyboard(downloadURL)

	sentMsg, err := m.Reply(text, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: replyMarkup,
	})
	if err == nil && sentMsg != nil && p.appCtx.AutoDelete != nil {
		_ = p.appCtx.AutoDelete.ScheduleFastDownloadDelete(sentMsg.ChatID(), int32(sentMsg.ID))
	}
	return err
}

func (p *Plugin) isMaintenanceMode() bool {
	return p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.MaintenanceMode
}

// HandleFastDownloadCallback handles the callback query when the Fast Download button is clicked.
func (p *Plugin) HandleFastDownloadCallback(cb *telegram.CallbackQuery) error {
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

	data := cb.DataString()
	parts := strings.SplitN(data, "#", 2)
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		_, err := cb.Answer("⚠️ Invalid file reference", &telegram.CallbackOptions{
			Alert: true,
		})
		return err
	}

	fileID := strings.TrimSpace(parts[1])

	if p.appCtx == nil || p.appCtx.Stream == nil || p.appCtx.Delivery == nil {
		_, err := cb.Answer("⚠️ Fast download service unavailable", &telegram.CallbackOptions{
			Alert: true,
		})
		return err
	}

	rec, err := p.appCtx.Delivery.GetMedia(context.Background(), fileID)
	if err != nil {
		_, aErr := cb.Answer("⚠️ File metadata not found", &telegram.CallbackOptions{
			Alert: true,
		})
		return aErr
	}

	fileName := rec.FileName
	if fileName == "" {
		fileName = "Media_" + fileID
	}
	sizeStr := rec.Size
	if sizeStr == "" && rec.FileSize > 0 {
		sizeStr = commands.FormatBytes(rec.FileSize)
	}

	linkExpiry := p.getFastDownloadLinkExpiry()
	msgDeleteDur := p.getFastDownloadMsgDeleteDuration()

	downloadURL := p.appCtx.Stream.GetExpiringDownloadURL(fileID, fileName, linkExpiry)
	text := formatFastDownloadText(fileName, sizeStr, downloadURL, msgDeleteDur, linkExpiry)

	replyMarkup := p.buildFastDownloadKeyboard(downloadURL)

	// Send fast download link to the user in chat
	if p.appCtx.Client != nil && cb.SenderID != 0 {
		sentMsg, sErr := p.appCtx.Client.SendMessage(cb.SenderID, text, &telegram.SendOptions{
			ParseMode:   "html",
			ReplyMarkup: replyMarkup,
		})
		if sErr == nil && sentMsg != nil && p.appCtx.AutoDelete != nil {
			_ = p.appCtx.AutoDelete.ScheduleFastDownloadDelete(sentMsg.ChatID(), int32(sentMsg.ID))
		}
		_, _ = cb.Answer("✅ Fast Download link sent to your chat!")
		return sErr
	}

	if commands.IsValidTelegramButtonURL(downloadURL) {
		_, aErr := cb.Answer("", &telegram.CallbackOptions{
			URL: downloadURL,
		})
		return aErr
	}

	_, aErr := cb.Answer("✅ Fast Download link sent to your chat!")
	return aErr
}

// HandleCloseCallback deletes the message.
func (p *Plugin) HandleCloseCallback(cb *telegram.CallbackQuery) error {
	if cb != nil {
		_, _ = cb.Delete()
		_, _ = cb.Answer("")
	}
	return nil
}

// HandleStreamCommand aliases HandleFastDownloadCommand for backwards compatibility.
func (p *Plugin) HandleStreamCommand(m *telegram.NewMessage) error {
	return p.HandleFastDownloadCommand(m)
}

// HandleStreamLinksCallback aliases HandleFastDownloadCallback for backwards compatibility.
func (p *Plugin) HandleStreamLinksCallback(cb *telegram.CallbackQuery) error {
	return p.HandleFastDownloadCallback(cb)
}

// getFastDownloadLinkExpiry returns the duration for which the fast download link remains valid on the server (default: 30 minutes).
func (p *Plugin) getFastDownloadLinkExpiry() time.Duration {
	linkExpiry := 30 * time.Minute
	if p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.FastDownloadLinkExpiry != "" {
		if d, err := autodelete.ParseDuration(p.appCtx.Config.FastDownloadLinkExpiry); err == nil && d > 0 {
			linkExpiry = d
		}
	}
	return linkExpiry
}

// getFastDownloadMsgDeleteDuration returns the duration before the fast download Telegram message is auto-deleted (default: 5 minutes).
func (p *Plugin) getFastDownloadMsgDeleteDuration() time.Duration {
	msgDeleteDur := 5 * time.Minute
	if p.appCtx != nil && p.appCtx.AutoDelete != nil {
		if d := p.appCtx.AutoDelete.GetSettings().FastDownloadDeleteDuration; d > 0 {
			msgDeleteDur = d
		}
	} else if p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.AutoDeleteFastDownloadTime != "" {
		if d, err := autodelete.ParseDuration(p.appCtx.Config.AutoDeleteFastDownloadTime); err == nil && d > 0 {
			msgDeleteDur = d
		}
	}
	return msgDeleteDur
}

// formatFastDownloadText builds the HTML text for the fast download response message.
func formatFastDownloadText(fileName, sizeStr, downloadURL string, msgDeleteDur, linkExpiry time.Duration) string {
	msgDurText := autodelete.FormatDuration(msgDeleteDur)
	linkDurText := autodelete.FormatDuration(linkExpiry)
	return fmt.Sprintf(
		"⚡ <b><u>Fast Download Link</u></b>\n\n"+
			"📁 <b>File:</b> <code>%s</code>\n"+
			"📦 <b>Size:</b> <code>%s</code>\n"+
			"⏳ <b>Message Auto-Delete In:</b> <code>%s</code>\n"+
			"🔗 <b>Link Validity:</b> <code>%s</code>\n\n"+
			"🚀 <b>Direct Download Link:</b>\n"+
			"<a href=\"%s\">%s</a>\n\n"+
			"⚡ <i>Click the button below to start high-speed download!</i>\n"+
			"⚠️ <i>This message will be auto-deleted in %s. The download link remains valid on server for %s.</i>",
		commands.EscapeHTML(fileName),
		sizeStr,
		msgDurText,
		linkDurText,
		downloadURL,
		commands.EscapeHTML(fileName),
		msgDurText,
		linkDurText,
	)
}

// buildFastDownloadKeyboard creates the inline keyboard for the generated fast download message.
// The Fast Download button is styled with Green (Success) by default (or Blue via FAST_DOWNLOAD_BUTTON_STYLE=blue),
// providing a fresh contrast against the Red (Danger) Close button.
func (p *Plugin) buildFastDownloadKeyboard(downloadURL string) *telegram.ReplyInlineMarkup {
	if !commands.IsValidTelegramButtonURL(downloadURL) {
		return nil
	}

	btnText := "⚡ Fast Download ⚡"
	var emojiID int64
	if p != nil && p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.EnablePremiumEmoji {
		btnText = "Fast Download"
		emojiID = p.appCtx.Config.FastDownloadButtonEmojiID
		if emojiID == 0 {
			emojiID = 6156513311585211842
		}
	}

	dlBtn := telegram.Button.URL(btnText, downloadURL)
	if emojiID != 0 {
		dlBtn = dlBtn.Icon(emojiID)
	}
	style := "green"
	if p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.FastDownloadButtonStyle != "" {
		style = strings.ToLower(strings.TrimSpace(p.appCtx.Config.FastDownloadButtonStyle))
	}

	if style == "blue" || style == "primary" {
		dlBtn = dlBtn.Primary()
	} else {
		dlBtn = dlBtn.Success()
	}

	return telegram.NewKeyboard().
		AddRow(dlBtn).
		AddRow(
			telegram.Button.Data("❌ Cʟᴏsᴇ", "close_data").Danger(),
		).
		Build()
}
