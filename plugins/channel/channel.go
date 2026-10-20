package channel

import (
	"context"
	"encoding/base64"
	"strings"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/dustin/go-humanize"
	"github.com/itsafxal/MYTGBOTZ/internal/database/mongodb"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
)

// Plugin handles live channel media indexing from configured channels.
type Plugin struct {
	appCtx *plugins.AppContext
}

// New creates a new live channel indexing plugin instance.
func New(appCtx *plugins.AppContext) *Plugin {
	return &Plugin{appCtx: appCtx}
}

// Name returns the unique plugin name.
func (p *Plugin) Name() string {
	return "channel"
}

// Commands returns any command definitions (none for background channel watcher).
func (p *Plugin) Commands() []plugins.CommandDefinition {
	return nil
}

// MessageHandlers registers the media receiver for messages in configured channels.
func (p *Plugin) MessageHandlers() []plugins.MessageDefinition {
	return []plugins.MessageDefinition{
		{
			Filter: func(m *telegram.NewMessage) bool {
				if m == nil || p.appCtx == nil || p.appCtx.Config == nil {
					return false
				}
				if len(p.appCtx.Config.Channels) == 0 {
					return false
				}

				chatID := extractChatID(m)
				if !p.appCtx.Config.IsConfiguredChannel(chatID) {
					return false
				}

				return isMediaMessage(m)
			},
			Handler: p.HandleChannelMedia,
		},
	}
}

// HandleChannelMedia indexes incoming documents, videos, and audio from configured channels.
func (p *Plugin) HandleChannelMedia(m *telegram.NewMessage) error {
	if m == nil || p.appCtx == nil || p.appCtx.DB == nil {
		return nil
	}

	doc := ExtractMediaFromMessage(m)
	if doc == nil || doc.FileID == "" {
		return nil
	}

	ctx := context.Background()

	// 1. Check for duplicates in SQLite (<0.1ms) and MongoDB
	exists, err := p.appCtx.DB.CheckMediaExists(ctx, doc.FileID)
	if err == nil && exists {
		if p.appCtx.Logger != nil {
			p.appCtx.Logger.Debug("live channel indexing: skipped duplicate file",
				"file_id", doc.FileID,
				"file_name", doc.FileName,
				"chat_id", extractChatID(m),
			)
		}
		return nil
	}

	// 2. Alternating save between DB2 and DB3 in MongoDB + update SQLite runtime
	shard, err := p.appCtx.DB.SaveMediaLiveAlternating(ctx, *doc)
	if err != nil {
		if p.appCtx.Logger != nil {
			p.appCtx.Logger.Error("live channel indexing: failed to save media",
				"file_id", doc.FileID,
				"file_name", doc.FileName,
				"error", err,
			)
		}
		return err
	}

	if p.appCtx.Logger != nil {
		p.appCtx.Logger.Info("live channel indexing: successfully indexed new media",
			"file_id", doc.FileID,
			"file_name", doc.FileName,
			"size", doc.Size,
			"shard", shard,
			"chat_id", extractChatID(m),
		)
	}

	return nil
}

func isMediaMessage(m *telegram.NewMessage) bool {
	if m == nil {
		return false
	}
	return m.Document() != nil || m.Video() != nil || m.Audio() != nil
}

func extractChatID(m *telegram.NewMessage) int64 {
	if m == nil {
		return 0
	}
	if m.ChatID() != 0 {
		return m.ChatID()
	}
	if m.ChannelID() != 0 {
		return m.ChannelID()
	}
	if m.Channel != nil && m.Channel.ID != 0 {
		return m.Channel.ID
	}
	if m.SenderChat != nil && m.SenderChat.ID != 0 {
		return m.SenderChat.ID
	}
	return 0
}

// ExtractMediaFromMessage extracts standard MediaDoc metadata from a Telegram message.
func ExtractMediaFromMessage(m *telegram.NewMessage) *mongodb.MediaDoc {
	if m == nil {
		return nil
	}

	var doc *telegram.DocumentObj
	var fileType string

	if v := m.Video(); v != nil {
		doc = v
		fileType = "video"
	} else if a := m.Audio(); a != nil {
		doc = a
		fileType = "audio"
	} else if d := m.Document(); d != nil {
		doc = d
		fileType = "document"
	}

	if doc == nil {
		return nil
	}

	fileID := telegram.PackBotFileID(doc)
	if fileID == "" {
		return nil
	}

	fileName := extractFileName(doc)
	caption := strings.TrimSpace(m.Text())
	if caption == "" {
		caption = strings.TrimSpace(m.MessageText())
	}
	if caption == "" {
		caption = fileName
	}
	if fileName == "" {
		fileName = caption
	}
	if fileName == "" {
		fileName = "Media_" + fileID
	}

	var fileRef string
	if len(doc.FileReference) > 0 {
		fileRef = base64.RawURLEncoding.EncodeToString(doc.FileReference)
	}

	sizeStr := humanize.Bytes(uint64(doc.Size))

	return &mongodb.MediaDoc{
		FileID:   fileID,
		FileRef:  fileRef,
		FileName: fileName,
		FileSize: doc.Size,
		Size:     sizeStr,
		FileType: fileType,
		MimeType: doc.MimeType,
		Caption:  caption,
		Date:     time.Now().UTC(),
	}
}

func extractFileName(doc *telegram.DocumentObj) string {
	if doc == nil {
		return ""
	}
	for _, attr := range doc.Attributes {
		if fn, ok := attr.(*telegram.DocumentAttributeFilename); ok && fn.FileName != "" {
			return fn.FileName
		}
		if audio, ok := attr.(*telegram.DocumentAttributeAudio); ok {
			if audio.Title != "" {
				if audio.Performer != "" {
					return audio.Performer + " - " + audio.Title
				}
				return audio.Title
			}
		}
	}
	return ""
}
