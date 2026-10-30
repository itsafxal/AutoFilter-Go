package cmds

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/AutoFilter-Go/internal/plugins"
)

// CategoryMeta describes a command category and its UI properties.
type CategoryMeta struct {
	ID        string
	Name      string
	Label     string
	AdminOnly bool
}

// CanonicalCategories defines the ordered list of command categories.
var CanonicalCategories = []CategoryMeta{
	{ID: "gen", Name: "General", Label: "⚡ General", AdminOnly: false},
	{ID: "user", Name: "User Commands", Label: "👤 User Tools", AdminOnly: false},
	{ID: "media", Name: "File & Media", Label: "📁 File & Media", AdminOnly: false},
	{ID: "batch", Name: "Batch Management", Label: "📦 Batch System", AdminOnly: false},
	{ID: "fcb", Name: "FCB Commands", Label: "📢 FCB Controls", AdminOnly: true},
	{ID: "gfilter", Name: "Global Filters", Label: "🏷️ Global Filters", AdminOnly: true},
	{ID: "index", Name: "Indexing", Label: "🔍 Indexing", AdminOnly: true},
	{ID: "group", Name: "Group Management", Label: "👥 Groups", AdminOnly: true},
	{ID: "admin", Name: "Admin Commands", Label: "🛡️ Admin Controls", AdminOnly: true},
	{ID: "dev", Name: "Developer Tools", Label: "💻 Developer Tools", AdminOnly: true},
}

// Plugin implements the /cmds, /cmd, and /cmdc interactive categorized command browser.
type Plugin struct {
	appCtx *plugins.AppContext
}

// New creates a new cmds plugin instance.
func New(appCtx *plugins.AppContext) *Plugin {
	return &Plugin{appCtx: appCtx}
}

// Name returns the unique plugin name.
func (p *Plugin) Name() string {
	return "cmds"
}

// Commands returns the command definitions for cmds plugin.
func (p *Plugin) Commands() []plugins.CommandDefinition {
	return []plugins.CommandDefinition{
		{
			Name:        "cmds",
			Aliases:     []string{"help", "cmd"},
			Description: "Explore the categorized command center and usage guide",
			Usage:       "/cmd",
			Category:    "General",
			Handler:     p.HandleCmds,
		},
		{
			Name:        "cmdc",
			Description: "Open the admin command master center directly (Admin Only)",
			Usage:       "/cmdc",
			Category:    "Admin Commands",
			AdminOnly:   true,
			Handler:     p.HandleCmdc,
		},
	}
}

// CallbackHandlers returns the interactive callback query handlers for category navigation.
func (p *Plugin) CallbackHandlers() []plugins.CallbackDefinition {
	return []plugins.CallbackDefinition{
		{
			Prefix:  "cmd_cat#",
			Handler: p.HandleCategoryCallback,
		},
		{
			Prefix:  "cmdc_cat#",
			Handler: p.HandleLegacyCategoryCallback,
		},
		{
			Prefix:  "cmd_home",
			Handler: p.HandleHomeCallback,
		},
		{
			Prefix:  "cmdc_home",
			Handler: p.HandleHomeCallback,
		},
		{
			Prefix:  "close_data",
			Handler: p.HandleCloseCallback,
		},
		{
			Prefix:  "noop",
			Handler: p.HandleNoopCallback,
		},
	}
}

func (p *Plugin) isAdmin(userID int64) bool {
	if p.appCtx == nil || p.appCtx.Config == nil {
		return false
	}
	return p.appCtx.Config.IsAdmin(userID) || p.appCtx.Config.IsOwner(userID)
}

// findCategoryMeta finds category metadata by ID or Name.
func findCategoryMeta(idOrName string) *CategoryMeta {
	for i := range CanonicalCategories {
		c := &CanonicalCategories[i]
		if strings.EqualFold(c.ID, idOrName) || strings.EqualFold(c.Name, idOrName) {
			return c
		}
	}
	return nil
}

// HandleCmds renders the interactive command center.
func (p *Plugin) HandleCmds(m *telegram.NewMessage) error {
	var userID int64
	if m.Sender != nil && m.Sender.ID != 0 {
		userID = m.Sender.ID
	} else {
		userID = m.SenderID()
	}

	isAdmin := p.isAdmin(userID)
	text, markup := p.buildMainMenu(isAdmin)

	_, err := m.Reply(text, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: markup,
	})
	return err
}

// HandleCmdc renders the admin command center directly.
func (p *Plugin) HandleCmdc(m *telegram.NewMessage) error {
	var userID int64
	if m.Sender != nil && m.Sender.ID != 0 {
		userID = m.Sender.ID
	} else {
		userID = m.SenderID()
	}

	if !p.isAdmin(userID) {
		_, err := m.Reply("⛔ <b>Access Denied:</b> This command is restricted to bot administrators.", &telegram.SendOptions{
			ParseMode: "html",
		})
		return err
	}

	text, markup := p.buildMainMenu(true)
	_, err := m.Reply(text, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: markup,
	})
	return err
}

// HandleCategoryCallback renders the commands for a selected category with pagination.
func (p *Plugin) HandleCategoryCallback(cb *telegram.CallbackQuery) error {
	if cb == nil {
		return nil
	}

	parts := strings.Split(cb.DataString(), "#")
	if len(parts) < 2 {
		_, err := cb.Answer("Invalid category selection", &telegram.CallbackOptions{Alert: true})
		return err
	}

	catID := parts[1]
	page := 1
	if len(parts) >= 3 {
		if parsed, err := strconv.Atoi(parts[2]); err == nil && parsed > 0 {
			page = parsed
		}
	}

	meta := findCategoryMeta(catID)
	if meta == nil {
		_, err := cb.Answer("Category not found", &telegram.CallbackOptions{Alert: true})
		return err
	}

	isAdmin := p.isAdmin(cb.SenderID)
	if meta.AdminOnly && !isAdmin {
		_, err := cb.Answer("⛔ Access Denied: Administrator privileges required.", &telegram.CallbackOptions{Alert: true})
		return err
	}

	text, markup := p.buildCategoryView(meta, page, isAdmin)
	_, _ = cb.Edit(text, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: markup,
	})
	_, _ = cb.Answer("")
	return nil
}

// HandleLegacyCategoryCallback maps legacy category IDs to modern categories.
func (p *Plugin) HandleLegacyCategoryCallback(cb *telegram.CallbackQuery) error {
	if cb == nil {
		return nil
	}
	parts := strings.SplitN(cb.DataString(), "#", 2)
	if len(parts) < 2 {
		_, err := cb.Answer("Invalid category", &telegram.CallbackOptions{Alert: true})
		return err
	}

	legacyCat := parts[1]
	mappedID := legacyCat
	switch legacyCat {
	case "fsub":
		mappedID = "fcb"
	case "autodel", "caption":
		mappedID = "admin"
	case "media":
		mappedID = "media"
	case "general":
		mappedID = "gen"
	}

	meta := findCategoryMeta(mappedID)
	if meta == nil {
		meta = findCategoryMeta("admin")
	}

	isAdmin := p.isAdmin(cb.SenderID)
	if meta.AdminOnly && !isAdmin {
		_, err := cb.Answer("⛔ Access Denied: Administrator privileges required.", &telegram.CallbackOptions{Alert: true})
		return err
	}

	text, markup := p.buildCategoryView(meta, 1, isAdmin)
	_, _ = cb.Edit(text, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: markup,
	})
	_, _ = cb.Answer("")
	return nil
}

// HandleHomeCallback returns to the main categorized command menu in-place.
func (p *Plugin) HandleHomeCallback(cb *telegram.CallbackQuery) error {
	if cb == nil {
		return nil
	}

	isAdmin := p.isAdmin(cb.SenderID)
	text, markup := p.buildMainMenu(isAdmin)
	_, _ = cb.Edit(text, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: markup,
	})
	_, _ = cb.Answer("")
	return nil
}

// HandleCloseCallback deletes the message.
func (p *Plugin) HandleCloseCallback(cb *telegram.CallbackQuery) error {
	if cb != nil {
		_, _ = cb.Delete()
		_, _ = cb.Answer("")
	}
	return nil
}

// HandleNoopCallback answers empty string for page indicator clicks.
func (p *Plugin) HandleNoopCallback(cb *telegram.CallbackQuery) error {
	if cb != nil {
		_, _ = cb.Answer("")
	}
	return nil
}

// buildMainMenu constructs the interactive categorized command browser menu.
func (p *Plugin) buildMainMenu(isAdmin bool) (string, *telegram.ReplyInlineMarkup) {
	totalCmds := 0
	if p.appCtx != nil && p.appCtx.Registry != nil {
		if isAdmin {
			totalCmds = p.appCtx.Registry.CommandCount()
		} else {
			totalCmds = p.appCtx.Registry.PublicCommandCount()
		}
	}

	var sb strings.Builder
	sb.WriteString("╭━━━━━━━━━━━━━━━━━━━━╮\n")
	sb.WriteString("      🤖 <b>AutoFilter-Go COMMAND CENTER</b>\n")
	sb.WriteString("╰━━━━━━━━━━━━━━━━━━━━╯\n\n")
	sb.WriteString("Select a category below to explore available commands:\n\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━\n")
	if isAdmin {
		sb.WriteString(fmt.Sprintf("📊 <b>Total Commands:</b> <code>%d</code>", totalCmds))
	} else {
		sb.WriteString(fmt.Sprintf("📊 <b>Available Commands:</b> <code>%d</code>", totalCmds))
	}

	// Filter categories based on permissions
	var visibleCategories []CategoryMeta
	for _, cat := range CanonicalCategories {
		if !cat.AdminOnly || isAdmin {
			visibleCategories = append(visibleCategories, cat)
		}
	}

	kb := telegram.NewKeyboard()
	// Build 2-column inline buttons
	for i := 0; i < len(visibleCategories); i += 2 {
		btn1 := telegram.Button.Data(visibleCategories[i].Label, fmt.Sprintf("cmd_cat#%s#1", visibleCategories[i].ID)).Primary()
		if i+1 < len(visibleCategories) {
			btn2 := telegram.Button.Data(visibleCategories[i+1].Label, fmt.Sprintf("cmd_cat#%s#1", visibleCategories[i+1].ID)).Primary()
			kb.AddRow(btn1, btn2)
		} else {
			kb.AddRow(btn1)
		}
	}

	// Red Close button
	kb.AddRow(
		telegram.Button.Data("❌ Cʟᴏsᴇ", "close_data").Danger(),
	)

	return sb.String(), kb.Build()
}

const commandsPerPage = 6

// buildCategoryView renders commands for a category with pagination.
func (p *Plugin) buildCategoryView(meta *CategoryMeta, page int, isAdmin bool) (string, *telegram.ReplyInlineMarkup) {
	var cmds []plugins.CommandDefinition
	if p.appCtx != nil && p.appCtx.Registry != nil {
		allGrouped := p.appCtx.Registry.CommandsByCategory(isAdmin)
		cmds = allGrouped[meta.Name]
	}

	totalItems := len(cmds)
	totalPages := int(math.Ceil(float64(totalItems) / float64(commandsPerPage)))
	if totalPages < 1 {
		totalPages = 1
	}
	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}

	startIndex := (page - 1) * commandsPerPage
	endIndex := startIndex + commandsPerPage
	if endIndex > totalItems {
		endIndex = totalItems
	}

	var pageCmds []plugins.CommandDefinition
	if startIndex < totalItems {
		pageCmds = cmds[startIndex:endIndex]
	}

	var sb strings.Builder
	if totalPages > 1 {
		sb.WriteString(fmt.Sprintf("<b><u>%s (Page %d/%d)</u></b>\n\n", strings.ToUpper(meta.Label), page, totalPages))
	} else {
		sb.WriteString(fmt.Sprintf("<b><u>%s</u></b>\n\n", strings.ToUpper(meta.Label)))
	}

	if len(pageCmds) == 0 {
		sb.WriteString("<i>No commands currently registered in this category.</i>\n")
	} else {
		for _, cmd := range pageCmds {
			usageStr := cmd.Usage
			if usageStr == "" {
				usageStr = "/" + cmd.Name
			}

			aliasStr := ""
			if len(cmd.Aliases) > 0 {
				aliasStr = fmt.Sprintf(" (aliases: <code>/%s</code>)", strings.Join(cmd.Aliases, ", /"))
			}

			adminBadge := ""
			if cmd.AdminOnly {
				adminBadge = " <i>[Admin]</i>"
			}

			sb.WriteString(fmt.Sprintf("• <b><code>%s</code></b>%s%s\n", usageStr, aliasStr, adminBadge))
			sb.WriteString(fmt.Sprintf("  <i>%s</i>\n\n", cmd.Description))
		}
	}

	kb := telegram.NewKeyboard()

	// Add pagination row if multiple pages exist
	if totalPages > 1 {
		var prevBtn, pageIndicator, nextBtn telegram.KeyboardButton

		if page > 1 {
			prevBtn = telegram.Button.Data("◀️ Prev", fmt.Sprintf("cmd_cat#%s#%d", meta.ID, page-1)).Primary()
		} else {
			prevBtn = telegram.Button.Data("◀️", "noop")
		}

		pageIndicator = telegram.Button.Data(fmt.Sprintf("📄 %d/%d", page, totalPages), "noop")

		if page < totalPages {
			nextBtn = telegram.Button.Data("Next ▶️", fmt.Sprintf("cmd_cat#%s#%d", meta.ID, page+1)).Primary()
		} else {
			nextBtn = telegram.Button.Data("▶️", "noop")
		}

		kb.AddRow(prevBtn, pageIndicator, nextBtn)
	}

	// Navigation row: Back to Menu & Close
	kb.AddRow(
		telegram.Button.Data("◀️ Bᴀᴄᴋ ᴛᴏ Mᴇɴᴜ", "cmd_home").Primary(),
		telegram.Button.Data("❌ Cʟᴏsᴇ", "close_data").Danger(),
	)

	return strings.TrimSpace(sb.String()), kb.Build()
}
