package stream

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/MYTGBOTZ/internal/commands"
	"github.com/itsafxal/MYTGBOTZ/internal/delivery"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
)

// Plugin implements the File-To-Link, Streaming, and Fast Download commands.
type Plugin struct {
	appCtx *plugins.AppContext
}

// New creates a new stream plugin instance.
func New(appCtx *plugins.AppContext) *Plugin {
	return &Plugin{appCtx: appCtx}
}

// Name returns the unique plugin name.
func (p *Plugin) Name() string {
	return "stream"
}

// Commands returns the command definitions for the stream plugin.
func (p *Plugin) Commands() []plugins.CommandDefinition {
	return []plugins.CommandDefinition{
		{
			Name:        "stream",
			Aliases:     []string{"link", "fastdl", "streamlink"},
			Description: "Generate high-speed streaming and direct download links for a file",
			Usage:       "/stream <file_id> (or reply to a file with /stream)",
			Category:    "Media",
			Handler:     p.HandleStreamCommand,
		},
	}
}

// CallbackHandlers returns the callback query handlers for direct stream links.
func (p *Plugin) CallbackHandlers() []plugins.CallbackDefinition {
	return []plugins.CallbackDefinition{
		{
			Prefix:  "stream_links#",
			Handler: p.HandleStreamLinksCallback,
		},
	}
}

// MessageHandlers registers listeners for files forwarded or sent directly to the bot.
func (p *Plugin) MessageHandlers() []plugins.MessageDefinition {
	return []plugins.MessageDefinition{
		{
			Filter: func(m *telegram.NewMessage) bool {
				if m == nil || !m.IsPrivate() || !m.IsMedia() {
					return false
				}
				if m.IsCommand() || strings.HasPrefix(m.Text(), "/") {
					return false
				}
				return true
			},
			Handler: p.HandleIncomingMedia,
		},
	}
}

// HandleStreamCommand processes the /stream and /link commands.
func (p *Plugin) HandleStreamCommand(m *telegram.NewMessage) error {
	if p.appCtx == nil || p.appCtx.Stream == nil || p.appCtx.Delivery == nil {
		_, err := m.Reply("⚠️ <b>Service Unavailable:</b> Stream service is not ready.", &telegram.SendOptions{
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
			// Extract file ID from replied media
			botFileID := telegram.PackBotFileID(replyMsg.Media())
			if botFileID != "" {
				fileID = botFileID
			}
		}
	}

	if fileID == "" {
		_, err := m.Reply("ℹ️ <b>Usage:</b> <code>/stream &lt;file_id&gt;</code>\n\nOr reply to any media file with <code>/stream</code> to generate instant links.", &telegram.SendOptions{
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

	streamURL := p.appCtx.Stream.GetStreamURL(fileID, fileName)
	downloadURL := p.appCtx.Stream.GetDownloadURL(fileID, fileName)
	watchURL := p.appCtx.Stream.GetWatchURL(fileID)
	text := fmt.Sprintf(
		"🎬 <b><u>Fast Stream & Download Links</u></b>\n\n"+
			"📁 <b>File:</b> <code>%s</code>\n"+
			"📦 <b>Size:</b> <code>%s</code>\n\n"+
			"📺 <b>Online Stream Link:</b>\n"+
			"<a href=\"%s\">▶️ Click to Stream (High Quality)</a>\n\n"+
			"⚡ <b>Fast Download Link:</b>\n"+
			"<a href=\"%s\">🚀 Click to Download (High Speed)</a>\n\n"+
			"🌐 <b>Web Player:</b>\n"+
			"<a href=\"%s\">💻 Open Web Player & Controls</a>\n\n"+
			"ℹ️ <i>To stream in VLC: Open VLC -> Ctrl+N -> Paste Stream Link!</i>",
		commands.EscapeHTML(fileName),
		sizeStr,
		streamURL,
		downloadURL,
		watchURL,
	)

	var replyMarkup *telegram.ReplyInlineMarkup
	if commands.IsValidTelegramButtonURL(watchURL) && commands.IsValidTelegramButtonURL(downloadURL) {
		replyMarkup = telegram.NewKeyboard().
			AddRow(
				telegram.Button.URL("⚡ Fast Download", downloadURL),
				telegram.Button.URL("📺 Watch Online", watchURL),
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

// HandleStreamLinksCallback handles the "stream_links#<id>" callback query.
func (p *Plugin) HandleStreamLinksCallback(cb *telegram.CallbackQuery) error {
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
		_, err := cb.Answer("⚠️ Stream service unavailable", &telegram.CallbackOptions{
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

	streamURL := p.appCtx.Stream.GetStreamURL(fileID, fileName)
	downloadURL := p.appCtx.Stream.GetDownloadURL(fileID, fileName)
	watchURL := p.appCtx.Stream.GetWatchURL(fileID)

	text := fmt.Sprintf(
		"🎬 <b><u>Fast Stream & Download Links</u></b>\n\n"+
			"📁 <b>File:</b> <code>%s</code>\n"+
			"📦 <b>Size:</b> <code>%s</code>\n\n"+
			"📺 <b>Online Stream Link:</b>\n"+
			"<a href=\"%s\">▶️ Click to Stream (VLC / MX / Chrome)</a>\n\n"+
			"⚡ <b>Fast Download Link:</b>\n"+
			"<a href=\"%s\">🚀 Click to Download (High Speed)</a>\n\n"+
			"🌐 <b>Web Player:</b>\n"+
			"<a href=\"%s\">💻 Open Web Player & Controls</a>\n\n"+
			"ℹ️ <i>Stream link can be played directly in Chrome, VLC, or MX Player!</i>",
		commands.EscapeHTML(fileName),
		sizeStr,
		streamURL,
		downloadURL,
		watchURL,
	)

	var replyMarkup *telegram.ReplyInlineMarkup
	if commands.IsValidTelegramButtonURL(watchURL) && commands.IsValidTelegramButtonURL(downloadURL) {
		replyMarkup = telegram.NewKeyboard().
			AddRow(
				telegram.Button.URL("⚡ Fast Download", downloadURL),
				telegram.Button.URL("📺 Watch Online", watchURL),
			).
			Build()
	}

	// Send links to the user in chat
	if p.appCtx.Client != nil && cb.SenderID != 0 {
		sentMsg, sErr := p.appCtx.Client.SendMessage(cb.SenderID, text, &telegram.SendOptions{
			ParseMode:   "html",
			ReplyMarkup: replyMarkup,
		})
		if sErr == nil && sentMsg != nil && p.appCtx.AutoDelete != nil {
			_ = p.appCtx.AutoDelete.TrackSentMessage(sentMsg.ChatID(), sentMsg.ID, "stream")
		}
		_, _ = cb.Answer("✅ Stream & Download links sent to your chat!")
		return sErr
	}

	if commands.IsValidTelegramButtonURL(watchURL) {
		_, aErr := cb.Answer("", &telegram.CallbackOptions{
			URL: watchURL,
		})
		return aErr
	}

	_, aErr := cb.Answer("✅ Stream & Download links sent to your chat!")
	return aErr
}

// HandleIncomingMedia automatically extracts metadata and generates stream & download links for any file sent or forwarded.
func (p *Plugin) HandleIncomingMedia(m *telegram.NewMessage) error {
	if p.appCtx == nil || p.appCtx.Stream == nil || p.appCtx.Delivery == nil {
		return nil
	}

	media := m.Media()
	if media == nil {
		return nil
	}

	botFileID := telegram.PackBotFileID(media)
	if botFileID == "" {
		return nil
	}

	var fileName, mimeType, fileType string
	var fileSize int64
	var fileRef []byte

	switch doc := media.(type) {
	case *telegram.MessageMediaDocument:
		if d, ok := doc.Document.(*telegram.DocumentObj); ok {
			fileSize = d.Size
			mimeType = d.MimeType
			fileRef = d.FileReference
			fileType = "document"
			for _, attr := range d.Attributes {
				if fn, ok := attr.(*telegram.DocumentAttributeFilename); ok {
					fileName = fn.FileName
					break
				}
			}
		}
	case *telegram.MessageMediaPhoto:
		if ph, ok := doc.Photo.(*telegram.PhotoObj); ok {
			fileRef = ph.FileReference
			fileType = "photo"
			mimeType = "image/jpeg"
			fileName = fmt.Sprintf("photo_%d.jpg", ph.ID)
			if len(ph.Sizes) > 0 {
				if lastSize, ok := ph.Sizes[len(ph.Sizes)-1].(*telegram.PhotoSizeObj); ok {
					fileSize = int64(lastSize.Size)
				}
			}
		}
	}

	if fileName == "" {
		fileName = "Media_" + botFileID
	}
	sizeStr := commands.FormatBytes(fileSize)

	if len(fileRef) > 0 {
		delivery.StoreFreshFileRef(botFileID, fileRef)
	}

	// Persist media metadata so /stream, /dl, and /watch can immediately resolve it
	rec := &delivery.MediaDetails{
		FileID:    botFileID,
		FileRef:   base64.RawURLEncoding.EncodeToString(fileRef),
		FileName:  fileName,
		FileSize:  fileSize,
		Size:      sizeStr,
		FileType:  fileType,
		MimeType:  mimeType,
		Caption:   m.Text(),
		DateSaved: time.Now(),
	}
	_ = p.appCtx.Delivery.SaveMedia(context.Background(), rec)

	streamURL := p.appCtx.Stream.GetStreamURL(botFileID, fileName)
	downloadURL := p.appCtx.Stream.GetDownloadURL(botFileID, fileName)
	watchURL := p.appCtx.Stream.GetWatchURL(botFileID)

	text := fmt.Sprintf(
		"🎬 <b><u>Fast Stream & Download Links</u></b>\n\n"+
			"📁 <b>File:</b> <code>%s</code>\n"+
			"📦 <b>Size:</b> <code>%s</code>\n\n"+
			"📺 <b>Online Stream Link:</b>\n"+
			"<a href=\"%s\">▶️ Click to Stream (High Quality)</a>\n\n"+
			"⚡ <b>Fast Download Link:</b>\n"+
			"<a href=\"%s\">🚀 Click to Download (High Speed)</a>\n\n"+
			"🌐 <b>Web Player:</b>\n"+
			"<a href=\"%s\">💻 Open Web Player & Controls</a>\n\n"+
			"ℹ️ <i>Stream link can be played directly in Chrome, VLC, or MX Player!</i>",
		commands.EscapeHTML(fileName),
		sizeStr,
		streamURL,
		downloadURL,
		watchURL,
	)

	replyMarkup := telegram.NewKeyboard().
		AddRow(
			telegram.Button.URL("⚡ Fast Download", downloadURL),
			telegram.Button.URL("📺 Watch Online", watchURL),
		).
		Build()

	sentMsg, err := m.Reply(text, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: replyMarkup,
	})
	if err == nil && sentMsg != nil && p.appCtx.AutoDelete != nil {
		_ = p.appCtx.AutoDelete.TrackSentMessage(sentMsg.ChatID(), sentMsg.ID, "stream")
	}
	return err
}
