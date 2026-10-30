package gfilter

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/MYTGBOTZ/internal/commands"
	coreGFilter "github.com/itsafxal/MYTGBOTZ/internal/gfilter"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
)

const (
	pageSize = 10
)

// Plugin implements the administrator-managed Global Filter plugin.
type Plugin struct {
	appCtx *plugins.AppContext
}

// New creates a new Global Filter plugin instance.
func New(appCtx *plugins.AppContext) *Plugin {
	return &Plugin{appCtx: appCtx}
}

// Name returns the unique plugin name.
func (p *Plugin) Name() string {
	return "gfilter"
}

// Commands returns the admin command definitions for the Global Filter system.
func (p *Plugin) Commands() []plugins.CommandDefinition {
	return []plugins.CommandDefinition{
		{
			Name:        "gfilter",
			Aliases:     []string{"addgfilter", "setgfilter"},
			Description: "Add or update a Global Filter across all groups",
			Usage:       "/gfilter <keyword> (or reply to text/media with /gfilter <keyword>)",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.HandleGFilter,
		},
		{
			Name:        "gfilters",
			Aliases:     []string{"allgfilters", "listgfilters"},
			Description: "Display all active Global Filters with pagination",
			Usage:       "/gfilters",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.HandleGFilters,
		},
		{
			Name:        "viewgfilter",
			Aliases:     []string{"gfilterinfo"},
			Description: "Display details and preview of a specific Global Filter",
			Usage:       "/viewgfilter <keyword>",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.HandleViewGFilter,
		},
		{
			Name:        "delg",
			Aliases:     []string{"delgfilter", "deletegfilter"},
			Description: "Delete a specific Global Filter",
			Usage:       "/delg <keyword>",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.HandleDelGFilter,
		},
		{
			Name:        "delallg",
			Aliases:     []string{"delallgfilters", "cleargfilters"},
			Description: "Delete all Global Filters with confirmation",
			Usage:       "/delallg",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.HandleDelAllGFilter,
		},
	}
}

// CallbackHandlers returns the interactive callback definitions for Global Filter navigation.
func (p *Plugin) CallbackHandlers() []plugins.CallbackDefinition {
	return []plugins.CallbackDefinition{
		{Prefix: "gfilter_page#", Handler: p.HandlePaginationCallback},
		{Prefix: "gfilter_delall_confirm", Handler: p.HandleDelAllConfirmCallback},
		{Prefix: "gfilter_delall_cancel", Handler: p.HandleDelAllCancelCallback},
		{Prefix: "gfilter_noop", Handler: p.HandleNoopCallback},
	}
}

// HandleGFilter creates or updates a Global Filter.
func (p *Plugin) HandleGFilter(m *telegram.NewMessage) error {
	if p.appCtx == nil || p.appCtx.GFilter == nil {
		_, err := m.Reply("⚠️ <b>Global Filter service is unavailable.</b>", &telegram.SendOptions{ParseMode: "html"})
		return err
	}

	text := m.Text()
	cmdParts := strings.Fields(text)
	if len(cmdParts) < 2 && !m.IsReply() {
		guide := "ℹ️ <b><u>Global Filter Setup Guide</u></b>\n\n" +
			"Global Filters match incoming messages across all groups with exact whole-message matching.\n\n" +
			"<b>Usage 1 (Reply Method):</b>\n" +
			"• Reply to any message (text, photo, video, document, audio) with:\n" +
			"  <code>/gfilter &lt;keyword&gt;</code>\n" +
			"  <i>Example:</i> <code>/gfilter Vikram</code>\n\n" +
			"<b>Usage 2 (Direct Text Method):</b>\n" +
			"• <code>/gfilter &lt;keyword&gt; &lt;reply text&gt;</code>\n" +
			"  <i>Example:</i> <code>/gfilter Vikram Watch the full movie here!</code>\n\n" +
			"<b>Button Syntax (Optional):</b>\n" +
			"• <code>[Button Name](buttonurl:https://t.me/example)</code>\n" +
			"• <code>[Same Row Button](buttonurl:https://t.me/example:same)</code>"

		_, err := m.Reply(guide, &telegram.SendOptions{ParseMode: "html"})
		return err
	}

	var keyword string
	var replyText string
	var mediaType coreGFilter.MediaType = coreGFilter.MediaTypeText
	var fileID string
	var fileRef string
	var buttons []coreGFilter.ButtonRow

	// 1. Reply-based configuration
	if m.IsReply() {
		replyMsg, err := m.GetReplyMessage()
		if err != nil || replyMsg == nil {
			_, replyErr := m.Reply("❌ Failed to resolve replied message.", &telegram.SendOptions{ParseMode: "html"})
			return replyErr
		}

		// Keyword is all arguments after /gfilter
		argStr := strings.TrimSpace(text[len(cmdParts[0]):])
		if argStr == "" {
			_, replyErr := m.Reply("❌ Please provide a filter keyword.\n\n<b>Usage:</b> Reply with <code>/gfilter &lt;keyword&gt;</code>", &telegram.SendOptions{ParseMode: "html"})
			return replyErr
		}
		keyword = argStr

		// Detect replied media
		if v := replyMsg.Video(); v != nil {
			mediaType = coreGFilter.MediaTypeVideo
			fileID = telegram.PackBotFileID(v)
			if len(v.FileReference) > 0 {
				fileRef = base64.RawURLEncoding.EncodeToString(v.FileReference)
			}
		} else if pht := replyMsg.Photo(); pht != nil {
			mediaType = coreGFilter.MediaTypePhoto
			fileID = telegram.PackBotFileID(pht)
			if len(pht.FileReference) > 0 {
				fileRef = base64.RawURLEncoding.EncodeToString(pht.FileReference)
			}
		} else if d := replyMsg.Document(); d != nil {
			mediaType = coreGFilter.MediaTypeDocument
			if d.MimeType == "image/gif" {
				mediaType = coreGFilter.MediaTypeAnimation
			} else if strings.Contains(d.MimeType, "sticker") {
				mediaType = coreGFilter.MediaTypeSticker
			}
			fileID = telegram.PackBotFileID(d)
			if len(d.FileReference) > 0 {
				fileRef = base64.RawURLEncoding.EncodeToString(d.FileReference)
			}
		} else if a := replyMsg.Audio(); a != nil {
			mediaType = coreGFilter.MediaTypeAudio
			fileID = telegram.PackBotFileID(a)
			if len(a.FileReference) > 0 {
				fileRef = base64.RawURLEncoding.EncodeToString(a.FileReference)
			}
		} else if vc := replyMsg.Voice(); vc != nil {
			mediaType = coreGFilter.MediaTypeVoice
			fileID = telegram.PackBotFileID(vc)
			if len(vc.FileReference) > 0 {
				fileRef = base64.RawURLEncoding.EncodeToString(vc.FileReference)
			}
		} else if stk := replyMsg.Sticker(); stk != nil {
			mediaType = coreGFilter.MediaTypeSticker
			fileID = telegram.PackBotFileID(stk)
			if len(stk.FileReference) > 0 {
				fileRef = base64.RawURLEncoding.EncodeToString(stk.FileReference)
			}
		}

		replyText = replyMsg.Text()
		if replyText == "" && replyMsg.Message != nil {
			replyText = replyMsg.Message.Message
		}

		// Extract buttons from replied message markup
		if replyMsg.Message != nil && replyMsg.Message.ReplyMarkup != nil {
			buttons = coreGFilter.ExtractButtonsFromMarkup(replyMsg.Message.ReplyMarkup)
		}

		// If no markup on message, check for text button syntax in caption
		if len(buttons) == 0 && replyText != "" {
			cleanText, textBtns := coreGFilter.ParseButtonsFromText(replyText)
			if len(textBtns) > 0 {
				replyText = cleanText
				buttons = textBtns
			}
		}
	} else {
		// 2. Direct argument-based text configuration: /gfilter <keyword> <response>
		rawArgs := strings.TrimSpace(text[len(cmdParts[0]):])
		kw, resp, ok := parseKeywordAndResponse(rawArgs)
		if !ok || kw == "" || resp == "" {
			_, err := m.Reply("❌ <b>Invalid Syntax:</b> Please provide both keyword and response.\n\n<b>Usage:</b> <code>/gfilter &lt;keyword&gt; &lt;reply text&gt;</code>\n<i>Or for multi-word keywords:</i> <code>/gfilter \"The One\" &lt;reply text&gt;</code>", &telegram.SendOptions{ParseMode: "html"})
			return err
		}
		keyword = kw

		cleanText, textBtns := coreGFilter.ParseButtonsFromText(resp)
		replyText = cleanText
		buttons = textBtns
		mediaType = coreGFilter.MediaTypeText
	}

	filter := &coreGFilter.GFilter{
		Keyword:   keyword,
		ReplyText: replyText,
		MediaType: mediaType,
		FileID:    fileID,
		FileRef:   fileRef,
		Buttons:   buttons,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := p.appCtx.GFilter.SaveFilter(ctx, filter); err != nil {
		_, replyErr := m.Reply(fmt.Sprintf("❌ Error saving Global Filter: <code>%s</code>", commands.EscapeHTML(err.Error())), &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}

	btnCount := 0
	for _, r := range buttons {
		btnCount += len(r)
	}

	respMsg := fmt.Sprintf(
		"✅ <b>Global Filter Successfully Saved!</b>\n\n"+
			"• <b>Keyword:</b> <code>%s</code>\n"+
			"• <b>Type:</b> <i>%s</i>\n"+
			"• <b>Buttons:</b> <code>%d</code> configured\n"+
			"• <b>Status:</b> Active across all groups",
		commands.EscapeHTML(keyword),
		string(mediaType),
		btnCount,
	)

	_, err := m.Reply(respMsg, &telegram.SendOptions{ParseMode: "html"})
	return err
}

func parseKeywordAndResponse(input string) (string, string, bool) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", "", false
	}

	// Quoted keyword: "The One" response text
	if strings.HasPrefix(trimmed, "\"") {
		idx := strings.Index(trimmed[1:], "\"")
		if idx != -1 {
			keyword := strings.TrimSpace(trimmed[1 : idx+1])
			resp := strings.TrimSpace(trimmed[idx+2:])
			return keyword, resp, true
		}
	}

	parts := strings.SplitN(trimmed, " ", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
	}
	return parts[0], "", false
}

// HandleGFilters displays the paginated list of all active Global Filters.
func (p *Plugin) HandleGFilters(m *telegram.NewMessage) error {
	if p.appCtx == nil || p.appCtx.GFilter == nil {
		_, err := m.Reply("⚠️ <b>Global Filter service is unavailable.</b>", &telegram.SendOptions{ParseMode: "html"})
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	filters, total, err := p.appCtx.GFilter.ListFilters(ctx, pageSize, 0)
	if err != nil {
		_, replyErr := m.Reply(fmt.Sprintf("❌ Error listing filters: <code>%s</code>", commands.EscapeHTML(err.Error())), &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}

	if total == 0 {
		_, err := m.Reply("📭 <b>No Global Filters Configured!</b>\n\nUse <code>/gfilter &lt;keyword&gt;</code> to create your first global filter.", &telegram.SendOptions{ParseMode: "html"})
		return err
	}

	text, markup := p.buildFilterListContent(filters, total, 0)
	_, err = m.Reply(text, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: markup,
	})
	return err
}

func (p *Plugin) buildFilterListContent(filters []*coreGFilter.GFilter, total int64, offset int) (string, *telegram.ReplyInlineMarkup) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📋 <b><u>Global Filters Directory</u></b> (Total: <code>%d</code>)\n\n", total))

	for i, f := range filters {
		num := offset + i + 1
		mediaBadge := string(f.MediaType)
		if mediaBadge == "" {
			mediaBadge = "text"
		}
		sb.WriteString(fmt.Sprintf("<b>%d.</b> <code>%s</code> [<i>%s</i>]\n", num, commands.EscapeHTML(f.Keyword), mediaBadge))
	}

	totalPages := int((total + pageSize - 1) / pageSize)
	currentPage := (offset / pageSize) + 1
	sb.WriteString(fmt.Sprintf("\n<i>Page %d of %d</i>", currentPage, totalPages))

	// Pagination keyboard
	builder := telegram.NewKeyboard()
	if totalPages > 1 {
		var navBtns []telegram.KeyboardButton
		if offset > 0 {
			prevOffset := offset - pageSize
			if prevOffset < 0 {
				prevOffset = 0
			}
			navBtns = append(navBtns, telegram.Button.Data("◀️ Previous", fmt.Sprintf("gfilter_page#%d", prevOffset)).Primary())
		}

		navBtns = append(navBtns, telegram.Button.Data(fmt.Sprintf("%d / %d", currentPage, totalPages), "gfilter_noop"))

		if int64(offset+pageSize) < total {
			nextOffset := offset + pageSize
			navBtns = append(navBtns, telegram.Button.Data("Next ▶️", fmt.Sprintf("gfilter_page#%d", nextOffset)).Primary())
		}

		builder.AddRow(navBtns...)
	}

	builder.AddRow(telegram.Button.Data("❌ Cʟᴏsᴇ", "close_data").Danger())
	return sb.String(), builder.Build()
}

// HandlePaginationCallback updates the /gfilters page interactively.
func (p *Plugin) HandlePaginationCallback(cb *telegram.CallbackQuery) error {
	if cb == nil || p.appCtx == nil || p.appCtx.GFilter == nil {
		return nil
	}

	// Verify administrator
	var userID int64
	if cb.Sender != nil && cb.Sender.ID != 0 {
		userID = cb.Sender.ID
	} else {
		userID = cb.SenderID
	}
	if p.appCtx.Config != nil && !p.appCtx.Config.IsAdmin(userID) {
		_, err := cb.Answer("⛔ Only bot administrators can browse global filters.", &telegram.CallbackOptions{Alert: true})
		return err
	}

	data := cb.DataString()
	offsetStr := strings.TrimPrefix(data, "gfilter_page#")
	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	filters, total, err := p.appCtx.GFilter.ListFilters(ctx, pageSize, offset)
	if err != nil {
		_, aErr := cb.Answer("❌ Error loading page.", &telegram.CallbackOptions{Alert: true})
		return aErr
	}

	text, markup := p.buildFilterListContent(filters, total, offset)
	_, _ = cb.Edit(text, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: markup,
	})
	return nil
}

// HandleViewGFilter shows the details and preview of an individual filter.
func (p *Plugin) HandleViewGFilter(m *telegram.NewMessage) error {
	if p.appCtx == nil || p.appCtx.GFilter == nil {
		_, err := m.Reply("⚠️ <b>Global Filter service is unavailable.</b>", &telegram.SendOptions{ParseMode: "html"})
		return err
	}

	cmdParts := strings.Fields(m.Text())
	if len(cmdParts) < 2 {
		_, err := m.Reply("ℹ️ <b>Usage:</b> <code>/viewgfilter &lt;keyword&gt;</code>", &telegram.SendOptions{ParseMode: "html"})
		return err
	}

	keyword := strings.TrimSpace(m.Text()[len(cmdParts[0]):])
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	f, err := p.appCtx.GFilter.GetFilter(ctx, keyword)
	if err != nil || f == nil {
		_, replyErr := m.Reply(fmt.Sprintf("❌ <b>Global Filter Not Found:</b> <code>%s</code>", commands.EscapeHTML(keyword)), &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}

	btnCount := 0
	for _, r := range f.Buttons {
		btnCount += len(r)
	}

	fileIdInfo := "None"
	if f.FileID != "" {
		fileIdInfo = f.FileID
		if len(fileIdInfo) > 30 {
			fileIdInfo = fileIdInfo[:30] + "..."
		}
	}

	captionPreview := f.ReplyText
	if captionPreview == "" {
		captionPreview = "<i>None</i>"
	} else if len(captionPreview) > 300 {
		captionPreview = captionPreview[:300] + "..."
	}

	info := fmt.Sprintf(
		"🔍 <b><u>Global Filter Details</u></b>\n\n"+
			"• <b>Keyword:</b> <code>%s</code>\n"+
			"• <b>Type:</b> <i>%s</i>\n"+
			"• <b>File ID:</b> <code>%s</code>\n"+
			"• <b>Buttons:</b> <code>%d</code> configured\n"+
			"• <b>Created:</b> <code>%s</code>\n"+
			"• <b>Updated:</b> <code>%s</code>\n\n"+
			"<b>Configured Response Preview:</b>\n<blockquote>%s</blockquote>",
		commands.EscapeHTML(f.Keyword),
		string(f.MediaType),
		commands.EscapeHTML(fileIdInfo),
		btnCount,
		f.CreatedAt.UTC().Format("2006-01-02 15:04:05 UTC"),
		f.UpdatedAt.UTC().Format("2006-01-02 15:04:05 UTC"),
		commands.EscapeHTML(captionPreview),
	)

	_, err = m.Reply(info, &telegram.SendOptions{ParseMode: "html"})
	return err
}

// HandleDelGFilter deletes a single Global Filter by keyword.
func (p *Plugin) HandleDelGFilter(m *telegram.NewMessage) error {
	if p.appCtx == nil || p.appCtx.GFilter == nil {
		_, err := m.Reply("⚠️ <b>Global Filter service is unavailable.</b>", &telegram.SendOptions{ParseMode: "html"})
		return err
	}

	cmdParts := strings.Fields(m.Text())
	if len(cmdParts) < 2 {
		_, err := m.Reply("ℹ️ <b>Usage:</b> <code>/delg &lt;keyword&gt;</code>", &telegram.SendOptions{ParseMode: "html"})
		return err
	}

	keyword := strings.TrimSpace(m.Text()[len(cmdParts[0]):])
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	deleted, err := p.appCtx.GFilter.DeleteFilter(ctx, keyword)
	if err != nil {
		_, replyErr := m.Reply(fmt.Sprintf("❌ Error deleting global filter: <code>%s</code>", commands.EscapeHTML(err.Error())), &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}

	if !deleted {
		_, replyErr := m.Reply(fmt.Sprintf("❌ <b>Global Filter Not Found:</b> <code>%s</code>", commands.EscapeHTML(keyword)), &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}

	_, replyErr := m.Reply(fmt.Sprintf("🗑️ <b>Global Filter Deleted:</b> <code>%s</code>\n<i>Removed from MongoDB, SQLite, and active runtime cache.</i>", commands.EscapeHTML(keyword)), &telegram.SendOptions{ParseMode: "html"})
	return replyErr
}

// HandleDelAllGFilter prompts for confirmation before purging all Global Filters.
func (p *Plugin) HandleDelAllGFilter(m *telegram.NewMessage) error {
	prompt := "⚠️ <b><u>Delete ALL Global Filters?</u></b>\n\n" +
		"Are you sure you want to permanently delete <b>all Global Filters</b> from MongoDB and SQLite?\n\n" +
		"<i>This action cannot be undone. Active filters will immediately stop responding.</i>"

	builder := telegram.NewKeyboard().
		AddRow(
			telegram.Button.Data("⚠️ Yes, Delete All", "gfilter_delall_confirm").Danger(),
			telegram.Button.Data("❌ Cancel", "gfilter_delall_cancel").Primary(),
		)

	_, err := m.Reply(prompt, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: builder.Build(),
	})
	return err
}

// HandleDelAllConfirmCallback processes the confirmation of bulk deletion.
func (p *Plugin) HandleDelAllConfirmCallback(cb *telegram.CallbackQuery) error {
	if cb == nil || p.appCtx == nil || p.appCtx.GFilter == nil {
		return nil
	}

	var userID int64
	if cb.Sender != nil && cb.Sender.ID != 0 {
		userID = cb.Sender.ID
	} else {
		userID = cb.SenderID
	}
	if p.appCtx.Config != nil && !p.appCtx.Config.IsAdmin(userID) {
		_, err := cb.Answer("⛔ Only bot administrators can execute bulk deletion.", &telegram.CallbackOptions{Alert: true})
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	deletedCount, err := p.appCtx.GFilter.DeleteAllFilters(ctx)
	if err != nil {
		_, _ = cb.Answer("❌ Error deleting all global filters.", &telegram.CallbackOptions{Alert: true})
		return err
	}

	_, _ = cb.Answer("✅ All global filters deleted.", &telegram.CallbackOptions{Alert: false})
	_, _ = cb.Edit(fmt.Sprintf("🗑️ <b>All Global Filters Deleted!</b>\n\n• <b>Count purged:</b> <code>%d</code> records\n• <b>Status:</b> Cleared from MongoDB and SQLite.", deletedCount), &telegram.SendOptions{
		ParseMode: "html",
	})
	return nil
}

// HandleDelAllCancelCallback cancels bulk deletion.
func (p *Plugin) HandleDelAllCancelCallback(cb *telegram.CallbackQuery) error {
	if cb == nil {
		return nil
	}
	_, _ = cb.Answer("Deletion cancelled.", &telegram.CallbackOptions{Alert: false})
	_, _ = cb.Edit("❌ <b>Global Filter bulk deletion cancelled.</b>", &telegram.SendOptions{
		ParseMode: "html",
	})
	return nil
}

// HandleNoopCallback is a no-op handler for non-clickable pagination indicator buttons.
func (p *Plugin) HandleNoopCallback(cb *telegram.CallbackQuery) error {
	if cb == nil {
		return nil
	}
	_, err := cb.Answer("", &telegram.CallbackOptions{})
	return err
}
