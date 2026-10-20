package stream

import (
	"context"
	"fmt"
	"strings"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/MYTGBOTZ/internal/commands"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
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

	downloadURL := p.appCtx.Stream.GetDownloadURL(fileID, fileName)
	text := fmt.Sprintf(
		"⚡ <b><u>Fast Download Link</u></b>\n\n"+
			"📁 <b>File:</b> <code>%s</code>\n"+
			"📦 <b>Size:</b> <code>%s</code>\n\n"+
			"🚀 <b>Direct Download Link:</b>\n"+
			"<a href=\"%s\">%s</a>\n\n"+
			"⚡ <i>Click the button below to start high-speed download!</i>",
		commands.EscapeHTML(fileName),
		sizeStr,
		downloadURL,
		commands.EscapeHTML(fileName),
	)

	var replyMarkup *telegram.ReplyInlineMarkup
	if commands.IsValidTelegramButtonURL(downloadURL) {
		replyMarkup = telegram.NewKeyboard().
			AddRow(
				telegram.Button.URL("⚡ Fast Download", downloadURL),
			).
			Build()
	}

	sentMsg, err := m.Reply(text, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: replyMarkup,
	})
	if err == nil && sentMsg != nil && p.appCtx.AutoDelete != nil {
		_ = p.appCtx.AutoDelete.TrackSentMessage(sentMsg.ChatID(), sentMsg.ID, "stream")
	}
	return err
}

// HandleFastDownloadCallback handles the callback query when the Fast Download button is clicked.
func (p *Plugin) HandleFastDownloadCallback(cb *telegram.CallbackQuery) error {
	if cb == nil {
		return nil
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

	downloadURL := p.appCtx.Stream.GetDownloadURL(fileID, fileName)

	text := fmt.Sprintf(
		"⚡ <b><u>Fast Download Link</u></b>\n\n"+
			"📁 <b>File:</b> <code>%s</code>\n"+
			"📦 <b>Size:</b> <code>%s</code>\n\n"+
			"🚀 <b>Direct Download Link:</b>\n"+
			"<a href=\"%s\">%s</a>\n\n"+
			"⚡ <i>Click the button below to start high-speed download!</i>",
		commands.EscapeHTML(fileName),
		sizeStr,
		downloadURL,
		commands.EscapeHTML(fileName),
	)

	var replyMarkup *telegram.ReplyInlineMarkup
	if commands.IsValidTelegramButtonURL(downloadURL) {
		replyMarkup = telegram.NewKeyboard().
			AddRow(
				telegram.Button.URL("⚡ Fast Download", downloadURL),
			).
			Build()
	}

	// Send fast download link to the user in chat
	if p.appCtx.Client != nil && cb.SenderID != 0 {
		sentMsg, sErr := p.appCtx.Client.SendMessage(cb.SenderID, text, &telegram.SendOptions{
			ParseMode:   "html",
			ReplyMarkup: replyMarkup,
		})
		if sErr == nil && sentMsg != nil && p.appCtx.AutoDelete != nil {
			_ = p.appCtx.AutoDelete.TrackSentMessage(sentMsg.ChatID(), sentMsg.ID, "stream")
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

// HandleStreamCommand aliases HandleFastDownloadCommand for backwards compatibility.
func (p *Plugin) HandleStreamCommand(m *telegram.NewMessage) error {
	return p.HandleFastDownloadCommand(m)
}

// HandleStreamLinksCallback aliases HandleFastDownloadCallback for backwards compatibility.
func (p *Plugin) HandleStreamLinksCallback(cb *telegram.CallbackQuery) error {
	return p.HandleFastDownloadCallback(cb)
}
