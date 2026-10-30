package cmds

import (
	"strings"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/AutoFilter-Go/internal/plugins"
)

// Plugin implements the /cmds and /cmdc command plugin with interactive admin category navigation.
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
			Aliases:     []string{"help"},
			Description: "View list of available commands and usage guide",
			Usage:       "/cmds",
			Category:    "General",
			Handler:     p.HandleCmds,
		},
		{
			Name:        "cmdc",
			Description: "View all bot commands organized into interactive category menus (Admin Only)",
			Usage:       "/cmdc",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.HandleCmdc,
		},
	}
}

// CallbackHandlers returns the interactive callback query handlers for /cmdc navigation and close actions.
func (p *Plugin) CallbackHandlers() []plugins.CallbackDefinition {
	return []plugins.CallbackDefinition{
		{
			Prefix:  "cmdc_cat#",
			Handler: p.HandleCmdcCategoryCallback,
		},
		{
			Prefix:  "cmdc_home",
			Handler: p.HandleCmdcHomeCallback,
		},
		{
			Prefix:  "close_data",
			Handler: p.HandleCloseCallback,
		},
	}
}

func (p *Plugin) isAdmin(userID int64) bool {
	if p.appCtx == nil || p.appCtx.Config == nil {
		return false
	}
	return p.appCtx.Config.IsAdmin(userID) || p.appCtx.Config.IsOwner(userID)
}

// HandleCmds renders the user commands guide with a Red Close button.
func (p *Plugin) HandleCmds(m *telegram.NewMessage) error {
	helpText := "<b><u>🤖 BOT COMMANDS & USAGE GUIDE 🤖</u></b>\n\nNo commands registered."
	if p.appCtx != nil && p.appCtx.Registry != nil {
		helpText = p.appCtx.Registry.GenerateHelpText()
	}

	replyMarkup := telegram.NewKeyboard().
		AddRow(
			telegram.Button.Data("❌ Cʟᴏsᴇ", "close_data").Danger(),
		).
		Build()

	_, err := m.Reply(helpText, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: replyMarkup,
	})
	return err
}

// HandleCmdc renders the interactive Admin Commands Master Panel with category buttons.
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

	text, markup := p.buildCmdcMainMenu()
	_, err := m.Reply(text, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: markup,
	})
	return err
}

// HandleCmdcCategoryCallback renders the commands for a selected category in-place.
func (p *Plugin) HandleCmdcCategoryCallback(cb *telegram.CallbackQuery) error {
	if cb == nil {
		return nil
	}

	if !p.isAdmin(cb.SenderID) {
		_, err := cb.Answer("⛔ This admin panel is restricted.", &telegram.CallbackOptions{
			Alert: true,
		})
		return err
	}

	parts := strings.SplitN(cb.DataString(), "#", 2)
	if len(parts) < 2 {
		_, err := cb.Answer("Invalid category", &telegram.CallbackOptions{Alert: true})
		return err
	}

	cat := parts[1]
	text, markup := p.buildCmdcCategoryView(cat)
	_, editErr := cb.Edit(text, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: markup,
	})
	if editErr != nil {
		_, _ = cb.Edit(text, &telegram.SendOptions{
			ReplyMarkup: markup,
		})
	}
	_, _ = cb.Answer("")
	return nil
}

// HandleCmdcHomeCallback returns to the main /cmdc categories menu in-place.
func (p *Plugin) HandleCmdcHomeCallback(cb *telegram.CallbackQuery) error {
	if cb == nil {
		return nil
	}

	if !p.isAdmin(cb.SenderID) {
		_, err := cb.Answer("⛔ This admin panel is restricted.", &telegram.CallbackOptions{
			Alert: true,
		})
		return err
	}

	text, markup := p.buildCmdcMainMenu()
	_, editErr := cb.Edit(text, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: markup,
	})
	if editErr != nil {
		_, _ = cb.Edit(text, &telegram.SendOptions{
			ReplyMarkup: markup,
		})
	}
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

// buildCmdcMainMenu constructs the interactive admin categories menu.
func (p *Plugin) buildCmdcMainMenu() (string, *telegram.ReplyInlineMarkup) {
	text := "👑 <b><u>BOT COMMANDS MASTER PANEL (ADMIN)</u></b> 👑\n\n" +
		"Welcome, Administrator! Please choose a category below to view and manage commands:\n\n" +
		"• 📢 <b>Force Subscribe</b> — Primary & secondary channels, join thresholds\n" +
		"• ⏳ <b>Auto-Delete</b> — Search & file auto-delete timers, weekly trash cleanup\n" +
		"• 📝 <b>Custom Caption</b> — Media file caption template & formatting tags\n" +
		"• 📦 <b>Media & Delivery</b> — Direct retrieval, fast download & latest files\n" +
		"• ⚙️ <b>General & System</b> — Ping, latency, ID tools & diagnostics\n\n" +
		"<i>Select any button below to view detailed command usages:</i>"

	markup := telegram.NewKeyboard().
		AddRow(
			telegram.Button.Data("📢 Force Subscribe", "cmdc_cat#fsub").Primary(),
			telegram.Button.Data("⏳ Auto-Delete", "cmdc_cat#autodel").Primary(),
		).
		AddRow(
			telegram.Button.Data("📝 Custom Caption", "cmdc_cat#caption").Primary(),
			telegram.Button.Data("📦 Media & Files", "cmdc_cat#media").Primary(),
		).
		AddRow(
			telegram.Button.Data("⚙️ General & System", "cmdc_cat#general").Primary(),
		).
		AddRow(
			telegram.Button.Data("❌ Cʟᴏsᴇ", "close_data").Danger(),
		).
		Build()

	return text, markup
}

// buildCmdcCategoryView renders the detailed commands for a chosen category.
func (p *Plugin) buildCmdcCategoryView(category string) (string, *telegram.ReplyInlineMarkup) {
	var text string

	switch category {
	case "fsub":
		text = "👑 <b><u>📢 Force Subscribe (FSub) Controls</u></b>\n\n" +
			"<b>⚡ Status & Overview:</b>\n" +
			"• <code>/view_sub</code> — View active channels, IDs & request counts\n\n" +
			"<b>⚡ Primary Channels:</b>\n" +
			"• <code>/sub1 &lt;channel_id&gt;</code> — Set primary Channel 1 (or forward post)\n" +
			"• <code>/sub2 &lt;channel_id&gt;</code> — Set primary Channel 2 (or forward post)\n" +
			"• <code>/sub3 &lt;channel_id&gt;</code> — Set primary Channel 3 (or forward post)\n\n" +
			"<b>⚡ Secondary Channels (Alternates):</b>\n" +
			"• <code>/2sub1 &lt;channel_id&gt;</code> — Set secondary Channel 1\n" +
			"• <code>/2sub2 &lt;channel_id&gt;</code> — Set secondary Channel 2\n" +
			"• <code>/2sub3 &lt;channel_id&gt;</code> — Set secondary Channel 3\n\n" +
			"<b>⚡ Thresholds (Auto-Switch):</b>\n" +
			"• <code>/setcount1 &lt;count&gt;</code> — Request threshold for Channel 1\n" +
			"• <code>/setcount2 &lt;count&gt;</code> — Request threshold for Channel 2\n" +
			"• <code>/setcount3 &lt;count&gt;</code> — Request threshold for Channel 3\n\n" +
			"<b>⚡ Removals:</b>\n" +
			"• <code>/delsub1</code>, <code>/delsub2</code>, <code>/delsub3</code> — Remove primary channels\n" +
			"• <code>/2delsub1</code>, <code>/2delsub2</code>, <code>/2delsub3</code> — Remove secondary channels\n\n" +
			"💡 <i>Tip: Forward any message from your target channel directly to the bot with /sub1 to bind instantly.</i>"

	case "autodel":
		text = "👑 <b><u>⏳ Auto-Delete & Trash Controls</u></b>\n\n" +
			"<b>⚡ Auto-Delete Timers:</b>\n" +
			"• <code>/autodel</code> — View active auto-delete status & all timers\n" +
			"• <code>/set_filter_del &lt;time&gt;</code> — Search results timer (e.g. <code>15m</code>, <code>1h</code>, <code>0</code>)\n" +
			"• <code>/set_file_del &lt;time&gt;</code> — Delivered files timer (e.g. <code>30m</code>, <code>2h</code>, <code>0</code>)\n\n" +
			"<b>⚡ Trash Cleanup:</b>\n" +
			"• <code>/clean_trash</code> — Manually trigger weekly trash cleanup to delete old bot messages and protect from copyright strikes\n\n" +
			"💡 <i>Duration formats: <code>15m</code> (15 mins), <code>1h</code> (1 hour), <code>1d</code> (1 day). Use <code>0</code> to disable.</i>"

	case "caption":
		text = "👑 <b><u>📝 Custom Caption Controls</u></b>\n\n" +
			"<b>⚡ Template Management:</b>\n" +
			"• <code>/view_caption</code> — View active caption template and live preview\n" +
			"• <code>/set_caption &lt;template&gt;</code> — Set custom caption (or reply to text)\n" +
			"• <code>/del_caption</code> — Reset custom caption back to system default\n\n" +
			"💡 <i>Supported placeholders:</i>\n" +
			"• <code>{filename}</code> — Original file name\n" +
			"• <code>{size}</code> — Formatted file size\n" +
			"• <code>{ext}</code> — File extension\n" +
			"• <code>{quality}</code> — Detected resolution (e.g. 1080p, 720p)\n" +
			"• <code>{year}</code> — Movie release year"

	case "media":
		text = "👑 <b><u>📦 Media & Delivery Commands</u></b>\n\n" +
			"<b>⚡ File Retrieval & Direct Links:</b>\n" +
			"• <code>/fastdl &lt;file_id&gt;</code> — Generate direct high-speed download link (or reply to file)\n" +
			"• <code>/getfile &lt;file_id&gt;</code> — Retrieve and deliver media file by identifier\n" +
			"• <code>/latest</code> — View recently added media files in database"

	case "general":
		text = "👑 <b><u>⚙️ General & System Commands</u></b>\n\n" +
			"<b>⚡ Bot Diagnostics:</b>\n" +
			"• <code>/start</code> — Welcome guide & deep-link processor\n" +
			"• <code>/ping</code> — Check bot response latency, DB ping & system uptime\n" +
			"• <code>/info</code> — Bot specifications & user inspection (or reply to user)\n" +
			"• <code>/id</code> — Show chat ID, sender ID & replied message ID\n" +
			"• <code>/cmds</code> — User-facing command directory & guide\n" +
			"• <code>/cmdc</code> — Interactive admin commands master panel (Admin only)"

	default:
		text = "👑 <b><u>👑 Admin Commands</u></b>\n\nSelect a category from the menu."
	}

	markup := telegram.NewKeyboard().
		AddRow(
			telegram.Button.Data("◀️ Bᴀᴄᴋ ᴛᴏ Mᴇɴᴜ", "cmdc_home").Primary(),
			telegram.Button.Data("❌ Cʟᴏsᴇ", "close_data").Danger(),
		).
		Build()

	return text, markup
}
