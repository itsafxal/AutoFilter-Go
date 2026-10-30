package batch

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/AutoFilter-Go/internal/batch"
	"github.com/itsafxal/AutoFilter-Go/internal/commands"
	"github.com/itsafxal/AutoFilter-Go/internal/plugins"
)

const (
	listPageSize   = 6
	searchPageSize = 5
)

// Plugin implements the Named Batch management system.
type Plugin struct {
	appCtx     *plugins.AppContext
	editThrott map[string]time.Time
	editMu     sync.Mutex
}

// New creates a new batch plugin instance.
func New(appCtx *plugins.AppContext) *Plugin {
	return &Plugin{
		appCtx:     appCtx,
		editThrott: make(map[string]time.Time),
	}
}

// Name returns the unique plugin name.
func (p *Plugin) Name() string {
	return "batch"
}

// Commands returns the command definitions for the batch system.
func (p *Plugin) Commands() []plugins.CommandDefinition {
	return []plugins.CommandDefinition{
		{
			Name:        "batch",
			Aliases:     []string{"addbatch", "newbatch"},
			Description: "Create a new named batch of files, media, and text",
			Usage:       "/batch [name]",
			Category:    "Batch Management",
			Handler:     p.HandleBatch,
		},
		{
			Name:        "batchsearch",
			Aliases:     []string{"searchbatch", "findbatch"},
			Description: "Search saved batches by name",
			Usage:       "/batchsearch <name>",
			Category:    "Batch Management",
			Handler:     p.HandleBatchSearch,
		},
		{
			Name:        "batchlist",
			Aliases:     []string{"batches", "mybatches"},
			Description: "Display a paginated list of all saved batches",
			Usage:       "/batchlist",
			Category:    "Batch Management",
			Handler:     p.HandleBatchList,
		},
	}
}

// CallbackHandlers returns the interactive callback definitions for batch navigation.
func (p *Plugin) CallbackHandlers() []plugins.CallbackDefinition {
	return []plugins.CallbackDefinition{
		{Prefix: "batch_done:", Handler: p.HandleDoneCallback},
		{Prefix: "batch_cancel:", Handler: p.HandleCancelCallback},
		{Prefix: "batch_page:", Handler: p.HandleListPageCallback},
		{Prefix: "batch_spage:", Handler: p.HandleSearchPageCallback},
		{Prefix: "batch_view:", Handler: p.HandleViewCallback},
		{Prefix: "batch_del:", Handler: p.HandleDeleteCallback},
		{Prefix: "batch_noop", Handler: p.HandleNoopCallback},
	}
}

// HandleNoopCallback answers noop button clicks.
func (p *Plugin) HandleNoopCallback(cb *telegram.CallbackQuery) error {
	if cb != nil {
		_, _ = cb.Answer("")
	}
	return nil
}

// MessageHandlers implements the MessagePlugin interface to intercept items for active batch sessions.
func (p *Plugin) MessageHandlers() []plugins.MessageDefinition {
	return []plugins.MessageDefinition{
		{
			AllowChannel: false,
			Filter: func(m *telegram.NewMessage) bool {
				if m == nil || m.IsGroup() || m.IsChannel() || m.IsChannelPost() {
					return false
				}
				if p.appCtx == nil || p.appCtx.Batch == nil {
					return false
				}
				userID := p.getSenderID(m)
				if userID == 0 {
					return false
				}
				sm := p.appCtx.Batch.Sessions()
				if sm == nil {
					return false
				}
				return sm.IsPendingName(userID) || sm.GetSession(userID) != nil
			},
			Handler: p.HandleIncomingBatchItem,
		},
	}
}

func (p *Plugin) getSenderID(m *telegram.NewMessage) int64 {
	if m == nil {
		return 0
	}
	if m.Sender != nil && m.Sender.ID != 0 {
		return m.Sender.ID
	}
	return m.SenderID()
}

// HandleBatch starts an interactive batch creation session.
func (p *Plugin) HandleBatch(m *telegram.NewMessage) error {
	if p.appCtx == nil || p.appCtx.Batch == nil {
		_, err := m.Reply("⚠️ <b>Batch service is unavailable.</b>", &telegram.SendOptions{ParseMode: "html"})
		return err
	}

	if m.IsGroup() || m.IsChannel() || m.IsChannelPost() {
		botUsername := "bot"
		if p.appCtx.BotUser != nil && p.appCtx.BotUser.Username != "" {
			botUsername = p.appCtx.BotUser.Username
		}
		keyboard := telegram.NewKeyboard().
			AddRow(telegram.Button.URL("📦 Create Batch in Private", fmt.Sprintf("https://t.me/%s?start=newbatch", botUsername))).
			Build()
		_, err := m.Reply("ℹ️ <b>Batch creation is only available in private chat.</b>", &telegram.SendOptions{
			ParseMode:   "html",
			ReplyMarkup: keyboard,
		})
		return err
	}

	userID := p.getSenderID(m)
	chatID := m.ChatID()
	sm := p.appCtx.Batch.Sessions()

	// Check if user passed batch name as argument: /batch <name>
	text := strings.TrimSpace(m.Text())
	parts := strings.Fields(text)
	var batchName string
	if len(parts) > 1 {
		batchName = strings.TrimSpace(strings.TrimPrefix(text, parts[0]))
	}

	if batchName == "" {
		// Set pending name state
		sm.SetPendingName(userID, chatID)
		msg := "🏷️ <b>Please send a name for your new Batch:</b>\n\n" +
			"<i>Example:</i> <code>Marvel Cinematic Universe (Phase 1-4)</code>\n\n" +
			"Send /cancel at any time to abort."
		_, err := m.Reply(msg, &telegram.SendOptions{ParseMode: "html"})
		return err
	}

	return p.startSessionWithCard(m, userID, chatID, batchName)
}

func (p *Plugin) startSessionWithCard(m *telegram.NewMessage, userID, chatID int64, batchName string) error {
	sm := p.appCtx.Batch.Sessions()
	sess := sm.StartSession(userID, chatID, batchName)

	cardText := p.formatStatusCard(sess)
	markup := p.buildSessionKeyboard(sess.BatchID)

	statusMsg, err := m.Reply(cardText, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: markup,
	})
	if err != nil {
		return err
	}

	if statusMsg != nil {
		sess.StatusMessageID = statusMsg.ID
	}
	return nil
}

func (p *Plugin) formatStatusCard(s *batch.Session) string {
	return fmt.Sprintf(
		"📦 <b>BATCH:</b> %s\n"+
			"━━━━━━━━━━━━━━━━━━\n\n"+
			"📁 <b>Files:</b> %d\n"+
			"🎬 <b>Videos:</b> %d\n"+
			"🎵 <b>Audio:</b> %d\n"+
			"🖼 <b>Photos:</b> %d\n"+
			"📝 <b>Texts:</b> %d\n\n"+
			"📊 <b>Total Items:</b> %d\n\n"+
			"🟢 <b>Status:</b> Collecting\n\n"+
			"━━━━━━━━━━━━━━━━━━\n"+
			"<i>Send files, media, or text messages now. When finished, press DONE below.</i>",
		commands.EscapeHTML(s.BatchName),
		s.TotalFiles,
		s.TotalVideos,
		s.TotalAudio,
		s.TotalPhotos,
		s.TotalTexts,
		s.TotalItems,
	)
}

func (p *Plugin) buildSessionKeyboard(batchID string) *telegram.ReplyInlineMarkup {
	return telegram.NewKeyboard().
		AddRow(
			telegram.Button.Data("✅ DONE", "batch_done:"+batchID).Success(),
			telegram.Button.Data("❌ CANCEL", "batch_cancel:"+batchID).Danger(),
		).
		Build()
}

// HandleIncomingBatchItem intercepts incoming messages for pending batch creation or active collection.
func (p *Plugin) HandleIncomingBatchItem(m *telegram.NewMessage) error {
	userID := p.getSenderID(m)
	if userID == 0 || p.appCtx == nil || p.appCtx.Batch == nil {
		return nil
	}
	sm := p.appCtx.Batch.Sessions()
	if sm == nil {
		return nil
	}

	// 1. Check if user was waiting to supply batch name
	if sm.IsPendingName(userID) {
		text := strings.TrimSpace(m.Text())
		if text == "/cancel" {
			sm.ClearPendingName(userID)
			_, err := m.Reply("❌ <b>Batch creation cancelled.</b>", &telegram.SendOptions{ParseMode: "html"})
			return err
		}
		if strings.HasPrefix(text, "/") {
			_, err := m.Reply("⚠️ Please enter a valid name for your batch (or send /cancel):", &telegram.SendOptions{ParseMode: "html"})
			return err
		}

		sm.ClearPendingName(userID)
		return p.startSessionWithCard(m, userID, m.ChatID(), text)
	}

	// 2. Check if user has an active session
	sess := sm.GetSession(userID)
	if sess == nil {
		return nil
	}

	text := strings.TrimSpace(m.Text())
	if text == "/cancel" {
		sm.RemoveSession(userID)
		_, err := m.Reply("❌ <b>Batch creation cancelled.</b>", &telegram.SendOptions{ParseMode: "html"})
		return err
	}
	if text == "/done" {
		return p.finalizeSession(m.ChatID(), sess, m.ID)
	}

	// 3. Extract media or text into BatchItem
	item := p.extractItemFromMessage(m)
	if item == nil {
		return nil
	}

	// Assign atomic sequence and update in-memory counters
	seq := sess.AddItem(item, m.ID)

	// Live Dual Persistence: Persist item immediately to MongoDB, then update SQLite
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = p.appCtx.Batch.SaveItemDual(ctx, item)
	cancel()

	// Update status message card in-place with debounced rate-limiting
	p.triggerCardUpdate(sess)

	if p.appCtx.Logger != nil {
		p.appCtx.Logger.Info("batch item captured",
			"batch_id", sess.BatchID,
			"sequence", seq,
			"type", item.ItemType,
			"file_name", item.FileName,
		)
	}
	return nil
}

func (p *Plugin) extractItemFromMessage(m *telegram.NewMessage) *batch.BatchItem {
	if m == nil {
		return nil
	}

	item := &batch.BatchItem{
		CreatedAt: time.Now().UTC(),
	}

	caption := strings.TrimSpace(m.Text())
	if caption == "" {
		caption = strings.TrimSpace(m.MessageText())
	}

	// 1. Video
	if v := m.Video(); v != nil {
		item.ItemType = batch.ItemTypeVideo
		item.TelegramFileID = telegram.PackBotFileID(v)
		if len(v.FileReference) > 0 {
			item.StorageReference = base64.RawURLEncoding.EncodeToString(v.FileReference)
		}
		item.FileName = p.extractFileNameFromDoc(v)
		item.FileSize = v.Size
		item.MimeType = v.MimeType
		item.Caption = caption
		return item
	}

	// 2. Audio
	if a := m.Audio(); a != nil {
		item.ItemType = batch.ItemTypeAudio
		item.TelegramFileID = telegram.PackBotFileID(a)
		if len(a.FileReference) > 0 {
			item.StorageReference = base64.RawURLEncoding.EncodeToString(a.FileReference)
		}
		item.FileName = p.extractFileNameFromDoc(a)
		item.FileSize = a.Size
		item.MimeType = a.MimeType
		item.Caption = caption
		return item
	}

	// 3. Voice
	if vc := m.Voice(); vc != nil {
		item.ItemType = batch.ItemTypeVoice
		item.TelegramFileID = telegram.PackBotFileID(vc)
		if len(vc.FileReference) > 0 {
			item.StorageReference = base64.RawURLEncoding.EncodeToString(vc.FileReference)
		}
		item.FileName = "Voice Message"
		item.FileSize = vc.Size
		item.MimeType = vc.MimeType
		item.Caption = caption
		return item
	}

	// 4. Photo
	if pht := m.Photo(); pht != nil {
		item.ItemType = batch.ItemTypePhoto
		item.TelegramFileID = telegram.PackBotFileID(pht)
		if len(pht.FileReference) > 0 {
			item.StorageReference = base64.RawURLEncoding.EncodeToString(pht.FileReference)
		}
		item.FileName = "Photo.jpg"
		item.Caption = caption
		return item
	}

	// 5. Document
	if d := m.Document(); d != nil {
		item.ItemType = batch.ItemTypeDocument
		if d.MimeType == "image/gif" {
			item.ItemType = batch.ItemTypeAnimation
		}
		item.TelegramFileID = telegram.PackBotFileID(d)
		if len(d.FileReference) > 0 {
			item.StorageReference = base64.RawURLEncoding.EncodeToString(d.FileReference)
		}
		item.FileName = p.extractFileNameFromDoc(d)
		item.FileSize = d.Size
		item.MimeType = d.MimeType
		item.Caption = caption
		return item
	}

	// 6. Plain Text
	if text := strings.TrimSpace(m.Text()); text != "" {
		item.ItemType = batch.ItemTypeText
		item.TextContent = text
		item.FileName = "Text Message"
		return item
	}

	return nil
}

func (p *Plugin) extractFileNameFromDoc(doc *telegram.DocumentObj) string {
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
	return "File"
}

func (p *Plugin) triggerCardUpdate(s *batch.Session) {
	p.editMu.Lock()
	lastEdit := p.editThrott[s.BatchID]
	now := time.Now()
	// Throttle in-place message edits to at most once per 800ms
	if now.Sub(lastEdit) < 800*time.Millisecond {
		p.editMu.Unlock()
		return
	}
	p.editThrott[s.BatchID] = now
	p.editMu.Unlock()

	if p.appCtx.Client == nil || s.StatusMessageID == 0 {
		return
	}

	go func() {
		cardText := p.formatStatusCard(s)
		markup := p.buildSessionKeyboard(s.BatchID)
		_, _ = p.appCtx.Client.EditMessage(s.ChatID, s.StatusMessageID, cardText, &telegram.SendOptions{
			ParseMode:   "html",
			ReplyMarkup: markup,
		})
	}()
}

// HandleDoneCallback finalizes the active batch creation session.
func (p *Plugin) HandleDoneCallback(cb *telegram.CallbackQuery) error {
	if cb == nil || p.appCtx == nil || p.appCtx.Batch == nil {
		return nil
	}

	batchID := strings.TrimPrefix(cb.DataString(), "batch_done:")
	sm := p.appCtx.Batch.Sessions()
	sess := sm.GetSessionByBatchID(batchID)
	if sess == nil {
		_, _ = cb.Answer("⚠️ Session expired or batch already finalized.", &telegram.CallbackOptions{Alert: true})
		return nil
	}

	var userID int64
	if cb.Sender != nil && cb.Sender.ID != 0 {
		userID = cb.Sender.ID
	} else {
		userID = cb.SenderID
	}

	if sess.OwnerID != userID {
		_, _ = cb.Answer("⛔ Only the creator of this batch can finalize it.", &telegram.CallbackOptions{Alert: true})
		return nil
	}

	if sess.TotalItems == 0 {
		_, _ = cb.Answer("⚠️ Please send at least one item before pressing DONE.", &telegram.CallbackOptions{Alert: true})
		return nil
	}

	_, _ = cb.Answer("✅ Finalizing batch...")
	return p.finalizeSession(cb.ChatID, sess, 0)
}

func (p *Plugin) finalizeSession(chatID int64, sess *batch.Session, triggerMsgID int32) error {
	if !sess.BeginFinalize() {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	meta, items, tempIDs := sess.GetSnapshot()

	// Persist authoritative records to MongoDB and SQLite
	if err := p.appCtx.Batch.FinalizeBatch(ctx, meta, items); err != nil {
		sess.AbortFinalize()
		if p.appCtx.Client != nil && sess.StatusMessageID != 0 {
			_, _ = p.appCtx.Client.SendMessage(chatID, fmt.Sprintf("❌ <b>Persistence Error:</b> %s", commands.EscapeHTML(err.Error())), &telegram.SendOptions{ParseMode: "html"})
		}
		return err
	}

	sess.MarkDone()

	// Remove session from active pool
	p.appCtx.Batch.Sessions().RemoveSession(sess.OwnerID)

	// Clean up temporary user upload messages
	if p.appCtx.Client != nil && len(tempIDs) > 0 {
		go func(toDelete []int32) {
			// Chunk into batches of 100 for Telegram DeleteMessages API
			for i := 0; i < len(toDelete); i += 100 {
				end := i + 100
				if end > len(toDelete) {
					end = len(toDelete)
				}
				_, _ = p.appCtx.Client.DeleteMessages(chatID, toDelete[i:end])
				time.Sleep(100 * time.Millisecond)
			}
		}(tempIDs)
	}

	// Generate shareable link
	botUsername := "bot"
	if p.appCtx.BotUser != nil && p.appCtx.BotUser.Username != "" {
		botUsername = p.appCtx.BotUser.Username
	}
	batchLink := fmt.Sprintf("https://t.me/%s?start=batch_%s", botUsername, meta.BatchID)

	finalSummary := fmt.Sprintf(
		"🎉 <b>BATCH FINALIZED SUCCESSFULLY!</b>\n"+
			"━━━━━━━━━━━━━━━━━━\n\n"+
			"📦 <b>Batch:</b> %s\n"+
			"🆔 <b>Batch ID:</b> <code>%s</code>\n"+
			"📊 <b>Total Items:</b> %d (📁 %d files, 📝 %d texts)\n\n"+
			"🔗 <b>Shareable Link:</b>\n"+
			"<code>%s</code>\n\n"+
			"━━━━━━━━━━━━━━━━━━\n"+
			"<i>Anyone with this link can retrieve the entire batch in exact sequential order.</i>",
		commands.EscapeHTML(meta.BatchName),
		meta.BatchID,
		meta.TotalItems,
		meta.TotalFiles,
		meta.TotalTexts,
		batchLink,
	)

	keyboard := telegram.NewKeyboard().
		AddRow(
			telegram.Button.URL("🔗 Open Batch Link", batchLink),
			telegram.Button.Data("📋 Batch List", "batch_page:0"),
		).
		Build()

	if p.appCtx.Client != nil && sess.StatusMessageID != 0 {
		_, err := p.appCtx.Client.EditMessage(chatID, sess.StatusMessageID, finalSummary, &telegram.SendOptions{
			ParseMode:   "html",
			ReplyMarkup: keyboard,
		})
		return err
	}

	if p.appCtx.Client != nil {
		_, err := p.appCtx.Client.SendMessage(chatID, finalSummary, &telegram.SendOptions{
			ParseMode:   "html",
			ReplyMarkup: keyboard,
		})
		return err
	}
	return nil
}

// HandleCancelCallback aborts an in-progress batch session.
func (p *Plugin) HandleCancelCallback(cb *telegram.CallbackQuery) error {
	if cb == nil || p.appCtx == nil || p.appCtx.Batch == nil {
		return nil
	}

	batchID := strings.TrimPrefix(cb.DataString(), "batch_cancel:")
	sm := p.appCtx.Batch.Sessions()
	sess := sm.GetSessionByBatchID(batchID)
	if sess == nil {
		_, _ = cb.Answer("Session already expired or cancelled.")
		_, _ = cb.Edit("❌ <b>Batch creation cancelled.</b>", &telegram.SendOptions{ParseMode: "html"})
		return nil
	}

	var userID int64
	if cb.Sender != nil && cb.Sender.ID != 0 {
		userID = cb.Sender.ID
	} else {
		userID = cb.SenderID
	}

	if sess.OwnerID != userID {
		_, _ = cb.Answer("⛔ Only the creator of this batch can cancel it.", &telegram.CallbackOptions{Alert: true})
		return nil
	}

	sm.RemoveSession(userID)
	_, _ = cb.Answer("❌ Batch creation cancelled.")
	_, _ = cb.Edit("❌ <b>Batch creation cancelled.</b>", &telegram.SendOptions{ParseMode: "html"})

	// Clean up temporary messages in background
	_, _, tempIDs := sess.GetSnapshot()
	if p.appCtx.Client != nil && len(tempIDs) > 0 {
		go func(toDelete []int32) {
			_, _ = p.appCtx.Client.DeleteMessages(sess.ChatID, toDelete)
		}(tempIDs)
	}

	return nil
}

// HandleBatchSearch searches saved batches by name.
func (p *Plugin) HandleBatchSearch(m *telegram.NewMessage) error {
	if p.appCtx == nil || p.appCtx.Batch == nil {
		_, err := m.Reply("⚠️ <b>Batch service is unavailable.</b>", &telegram.SendOptions{ParseMode: "html"})
		return err
	}

	text := strings.TrimSpace(m.Text())
	parts := strings.Fields(text)
	if len(parts) < 2 {
		_, err := m.Reply("ℹ️ <b>Usage:</b> <code>/batchsearch &lt;name&gt;</code>\n\n<i>Example:</i> <code>/batchsearch Marvel</code>", &telegram.SendOptions{ParseMode: "html"})
		return err
	}

	query := strings.TrimSpace(strings.TrimPrefix(text, parts[0]))
	return p.renderSearchPage(m, nil, query, 0)
}

func (p *Plugin) renderSearchPage(m *telegram.NewMessage, cb *telegram.CallbackQuery, query string, offset int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	results, total, err := p.appCtx.Batch.SearchBatches(ctx, query, searchPageSize, offset)
	if err != nil {
		msg := fmt.Sprintf("❌ <b>Search Error:</b> %s", commands.EscapeHTML(err.Error()))
		if cb != nil {
			_, _ = cb.Answer("Error searching batches")
			_, editErr := cb.Edit(msg, &telegram.SendOptions{ParseMode: "html"})
			return editErr
		}
		_, replyErr := m.Reply(msg, &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}

	if total == 0 {
		msg := fmt.Sprintf("🔍 <b>No batches found matching:</b> <code>%s</code>", commands.EscapeHTML(query))
		if cb != nil {
			_, _ = cb.Answer("No results found")
			_, editErr := cb.Edit(msg, &telegram.SendOptions{ParseMode: "html"})
			return editErr
		}
		_, replyErr := m.Reply(msg, &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}

	botUsername := "bot"
	if p.appCtx.BotUser != nil && p.appCtx.BotUser.Username != "" {
		botUsername = p.appCtx.BotUser.Username
	}

	currentPage := (offset / searchPageSize) + 1
	totalPages := (total + searchPageSize - 1) / searchPageSize

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔍 <b>Batch Search Results for:</b> <code>%s</code>\n", commands.EscapeHTML(query)))
	sb.WriteString(fmt.Sprintf("<i>Showing %d-%d of %d batches (Page %d/%d)</i>\n\n",
		offset+1, min(offset+len(results), total), total, currentPage, totalPages))

	kb := telegram.NewKeyboard()

	for i, b := range results {
		createdStr := b.CreatedAt.Format("02 Jan 2006")
		batchLink := fmt.Sprintf("https://t.me/%s?start=batch_%s", botUsername, b.BatchID)
		sb.WriteString(fmt.Sprintf("<b>%d. %s</b>\n", offset+i+1, commands.EscapeHTML(b.BatchName)))
		sb.WriteString(fmt.Sprintf("   • 🆔 <code>%s</code> | 📁 %d files | 📝 %d texts\n", b.BatchID, b.TotalFiles, b.TotalTexts))
		sb.WriteString(fmt.Sprintf("   • 📅 Created: %s\n", createdStr))
		sb.WriteString(fmt.Sprintf("   • 🔗 <a href=\"%s\">Open Collection</a>\n\n", batchLink))

		btnText := fmt.Sprintf("📦 %d. %s", offset+i+1, truncateString(b.BatchName, 25))
		kb.AddRow(telegram.Button.Data(btnText, "batch_view:"+b.BatchID))
	}

	// Pagination buttons
	var navButtons []telegram.KeyboardButton
	if offset > 0 {
		prevOffset := offset - searchPageSize
		if prevOffset < 0 {
			prevOffset = 0
		}
		navButtons = append(navButtons, telegram.Button.Data("◀️ Prev", fmt.Sprintf("batch_spage:%s:%d", query, prevOffset)))
	}
	navButtons = append(navButtons, telegram.Button.Data(fmt.Sprintf("📄 %d/%d", currentPage, totalPages), "batch_noop"))
	if offset+searchPageSize < total {
		nextOffset := offset + searchPageSize
		navButtons = append(navButtons, telegram.Button.Data("Next ▶️", fmt.Sprintf("batch_spage:%s:%d", query, nextOffset)))
	}
	if len(navButtons) > 0 {
		kb.AddRow(navButtons...)
	}

	content := strings.TrimSpace(sb.String())
	markup := kb.Build()

	if cb != nil {
		_, _ = cb.Answer("")
		_, err := cb.Edit(content, &telegram.SendOptions{
			ParseMode:   "html",
			ReplyMarkup: markup,
		})
		return err
	}

	_, err = m.Reply(content, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: markup,
	})
	return err
}

// HandleSearchPageCallback handles pagination for search results.
func (p *Plugin) HandleSearchPageCallback(cb *telegram.CallbackQuery) error {
	if cb == nil || p.appCtx == nil || p.appCtx.Batch == nil {
		return nil
	}

	raw := strings.TrimPrefix(cb.DataString(), "batch_spage:")
	parts := strings.Split(raw, ":")
	if len(parts) < 2 {
		return nil
	}
	query := parts[0]
	offset, _ := strconv.Atoi(parts[1])

	return p.renderSearchPage(nil, cb, query, offset)
}

// HandleBatchList renders a paginated list of all finalized batches.
func (p *Plugin) HandleBatchList(m *telegram.NewMessage) error {
	if p.appCtx == nil || p.appCtx.Batch == nil {
		_, err := m.Reply("⚠️ <b>Batch service is unavailable.</b>", &telegram.SendOptions{ParseMode: "html"})
		return err
	}

	return p.renderListPage(m, nil, 0)
}

func (p *Plugin) renderListPage(m *telegram.NewMessage, cb *telegram.CallbackQuery, offset int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	batches, total, err := p.appCtx.Batch.ListBatches(ctx, 0, listPageSize, offset)
	if err != nil {
		msg := fmt.Sprintf("❌ <b>Database Error:</b> %s", commands.EscapeHTML(err.Error()))
		if cb != nil {
			_, _ = cb.Answer("Error loading batches")
			_, editErr := cb.Edit(msg, &telegram.SendOptions{ParseMode: "html"})
			return editErr
		}
		_, replyErr := m.Reply(msg, &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}

	if total == 0 {
		msg := "📦 <b>No batches saved yet.</b>\n\nUse /batch to create your first collection!"
		if cb != nil {
			_, _ = cb.Answer("No batches found")
			_, editErr := cb.Edit(msg, &telegram.SendOptions{ParseMode: "html"})
			return editErr
		}
		_, replyErr := m.Reply(msg, &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}

	currentPage := (offset / listPageSize) + 1
	totalPages := (total + listPageSize - 1) / listPageSize

	botUsername := "bot"
	if p.appCtx.BotUser != nil && p.appCtx.BotUser.Username != "" {
		botUsername = p.appCtx.BotUser.Username
	}

	var sb strings.Builder
	sb.WriteString("📦 <b><u>SAVED BATCH COLLECTIONS</u></b>\n\n")
	sb.WriteString(fmt.Sprintf("<i>Showing %d-%d of %d batches (Page %d/%d)</i>\n\n",
		offset+1, min(offset+len(batches), total), total, currentPage, totalPages))

	kb := telegram.NewKeyboard()

	for i, b := range batches {
		createdStr := b.CreatedAt.Format("02 Jan 2006")
		batchLink := fmt.Sprintf("https://t.me/%s?start=batch_%s", botUsername, b.BatchID)
		sb.WriteString(fmt.Sprintf("<b>%d. %s</b>\n", offset+i+1, commands.EscapeHTML(b.BatchName)))
		sb.WriteString(fmt.Sprintf("   • 🆔 <code>%s</code> | 📁 %d files | 📝 %d texts\n", b.BatchID, b.TotalFiles, b.TotalTexts))
		sb.WriteString(fmt.Sprintf("   • 📅 Created: %s\n", createdStr))
		sb.WriteString(fmt.Sprintf("   • 🔗 <a href=\"%s\">Open Collection</a>\n\n", batchLink))

		btnText := fmt.Sprintf("📦 %d. %s", offset+i+1, truncateString(b.BatchName, 25))
		kb.AddRow(telegram.Button.Data(btnText, "batch_view:"+b.BatchID))
	}

	// Pagination buttons
	var navButtons []telegram.KeyboardButton
	if offset > 0 {
		prevOffset := offset - listPageSize
		if prevOffset < 0 {
			prevOffset = 0
		}
		navButtons = append(navButtons, telegram.Button.Data("◀️ Prev", fmt.Sprintf("batch_page:%d", prevOffset)))
	}
	navButtons = append(navButtons, telegram.Button.Data(fmt.Sprintf("📄 %d/%d", currentPage, totalPages), "batch_noop"))
	if offset+listPageSize < total {
		nextOffset := offset + listPageSize
		navButtons = append(navButtons, telegram.Button.Data("Next ▶️", fmt.Sprintf("batch_page:%d", nextOffset)))
	}
	if len(navButtons) > 0 {
		kb.AddRow(navButtons...)
	}

	content := strings.TrimSpace(sb.String())
	markup := kb.Build()

	if cb != nil {
		_, _ = cb.Answer("")
		_, err := cb.Edit(content, &telegram.SendOptions{
			ParseMode:   "html",
			ReplyMarkup: markup,
		})
		return err
	}

	_, err = m.Reply(content, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: markup,
	})
	return err
}

// HandleListPageCallback processes page changes in the batch list.
func (p *Plugin) HandleListPageCallback(cb *telegram.CallbackQuery) error {
	if cb == nil || p.appCtx == nil || p.appCtx.Batch == nil {
		return nil
	}

	offsetStr := strings.TrimPrefix(cb.DataString(), "batch_page:")
	offset, _ := strconv.Atoi(offsetStr)
	return p.renderListPage(nil, cb, offset)
}

// HandleViewCallback renders detailed information for a single batch.
func (p *Plugin) HandleViewCallback(cb *telegram.CallbackQuery) error {
	if cb == nil || p.appCtx == nil || p.appCtx.Batch == nil {
		return nil
	}

	batchID := strings.TrimPrefix(cb.DataString(), "batch_view:")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	meta, err := p.appCtx.Batch.GetBatch(ctx, batchID)
	if err != nil || meta == nil {
		_, _ = cb.Answer("Batch not found or removed.", &telegram.CallbackOptions{Alert: true})
		return nil
	}

	botUsername := "bot"
	if p.appCtx.BotUser != nil && p.appCtx.BotUser.Username != "" {
		botUsername = p.appCtx.BotUser.Username
	}
	batchLink := fmt.Sprintf("https://t.me/%s?start=batch_%s", botUsername, meta.BatchID)
	createdStr := meta.CreatedAt.Format("02 Jan 2006, 15:04 MST")

	card := fmt.Sprintf(
		"📦 <b><u>BATCH DETAILS</u></b>\n\n"+
			"🏷️ <b>Name:</b> %s\n"+
			"🆔 <b>Batch ID:</b> <code>%s</code>\n"+
			"📊 <b>Total Items:</b> %d\n"+
			"📁 <b>Total Files:</b> %d (🎬 %d vids, 🎵 %d audio, 🖼 %d photos, 📄 %d docs)\n"+
			"📝 <b>Total Texts:</b> %d\n"+
			"📅 <b>Created:</b> %s\n\n"+
			"🔗 <b>Shareable Link:</b>\n<code>%s</code>",
		commands.EscapeHTML(meta.BatchName),
		meta.BatchID,
		meta.TotalItems,
		meta.TotalFiles,
		meta.TotalVideos,
		meta.TotalAudio,
		meta.TotalPhotos,
		meta.TotalDocuments,
		meta.TotalTexts,
		createdStr,
		batchLink,
	)

	var userID int64
	if cb.Sender != nil && cb.Sender.ID != 0 {
		userID = cb.Sender.ID
	} else {
		userID = cb.SenderID
	}

	isAdmin := p.appCtx.Config != nil && (p.appCtx.Config.IsAdmin(userID) || p.appCtx.Config.IsOwner(userID))
	isOwner := meta.OwnerID == userID

	kb := telegram.NewKeyboard().
		AddRow(telegram.Button.URL("🔗 Open Batch Link", batchLink))

	if isOwner || isAdmin {
		kb.AddRow(
			telegram.Button.Data("🗑️ Delete Batch", "batch_del:"+meta.BatchID).Danger(),
			telegram.Button.Data("◀️ Back to List", "batch_page:0"),
		)
	} else {
		kb.AddRow(telegram.Button.Data("◀️ Back to List", "batch_page:0"))
	}

	_, _ = cb.Answer("")
	_, err = cb.Edit(card, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: kb.Build(),
	})
	return err
}

// HandleDeleteCallback deletes a batch from MongoDB and SQLite.
func (p *Plugin) HandleDeleteCallback(cb *telegram.CallbackQuery) error {
	if cb == nil || p.appCtx == nil || p.appCtx.Batch == nil {
		return nil
	}

	batchID := strings.TrimPrefix(cb.DataString(), "batch_del:")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	meta, err := p.appCtx.Batch.GetBatch(ctx, batchID)
	if err != nil || meta == nil {
		_, _ = cb.Answer("Batch not found.", &telegram.CallbackOptions{Alert: true})
		return nil
	}

	var userID int64
	if cb.Sender != nil && cb.Sender.ID != 0 {
		userID = cb.Sender.ID
	} else {
		userID = cb.SenderID
	}

	isAdmin := p.appCtx.Config != nil && (p.appCtx.Config.IsAdmin(userID) || p.appCtx.Config.IsOwner(userID))
	if meta.OwnerID != userID && !isAdmin {
		_, _ = cb.Answer("⛔ Only the batch creator or bot admins can delete this batch.", &telegram.CallbackOptions{Alert: true})
		return nil
	}

	if err := p.appCtx.Batch.DeleteBatch(ctx, batchID); err != nil {
		_, _ = cb.Answer(fmt.Sprintf("Error deleting batch: %s", err.Error()), &telegram.CallbackOptions{Alert: true})
		return err
	}

	_, _ = cb.Answer("🗑️ Batch deleted successfully.")
	return p.renderListPage(nil, cb, 0)
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
