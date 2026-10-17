package caption

import (
	"context"
	"fmt"
	"strings"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/MYTGBOTZ/internal/commands"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
)

// Plugin implements the custom file caption management commands.
type Plugin struct {
	appCtx *plugins.AppContext
}

// New creates a new caption plugin instance.
func New(appCtx *plugins.AppContext) *Plugin {
	return &Plugin{appCtx: appCtx}
}

// Name returns the unique plugin name.
func (p *Plugin) Name() string {
	return "caption"
}

// Commands returns the administrative commands for managing custom media captions.
func (p *Plugin) Commands() []plugins.CommandDefinition {
	return []plugins.CommandDefinition{
		{
			Name:        "set_caption",
			Aliases:     []string{"setcaption", "caption"},
			Description: "Set custom media file caption template (Admin only)",
			Usage:       "/set_caption <template> or reply to a message",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.HandleSetCaption,
		},
		{
			Name:        "view_caption",
			Aliases:     []string{"viewcaption", "get_caption", "show_caption"},
			Description: "View active media caption template and preview (Admin only)",
			Usage:       "/view_caption",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.HandleViewCaption,
		},
		{
			Name:        "del_caption",
			Aliases:     []string{"delcaption", "reset_caption", "rmcaption"},
			Description: "Reset custom media caption to default (Admin only)",
			Usage:       "/del_caption",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.HandleDelCaption,
		},
	}
}

// HandleSetCaption sets a new custom media caption template in bot DM.
func (p *Plugin) HandleSetCaption(m *telegram.NewMessage) error {
	if m == nil {
		return nil
	}

	// Encourage usage in private chat / DM
	if !m.IsPrivate() {
		_, err := m.Reply("⚠️ <b>Notice:</b> Please configure custom captions in private chat (DM) with the bot.", &telegram.SendOptions{
			ParseMode: "html",
		})
		return err
	}

	var templateText string

	// 1. Check if user replied to a message
	if m.IsReply() {
		reply, err := m.GetReplyMessage()
		if err == nil && reply != nil && strings.TrimSpace(reply.Text()) != "" {
			templateText = strings.TrimSpace(reply.Text())
		}
	}

	// 2. Otherwise extract text following the command
	if templateText == "" {
		text := m.Text()
		parts := strings.SplitN(text, " ", 2)
		if len(parts) > 1 {
			templateText = strings.TrimSpace(parts[1])
		}
	}

	// If no template was provided, display instructions and placeholder guide
	if templateText == "" {
		guide := "📝 <b><u>Custom File Caption Settings</u></b>\n\n" +
			"Configure a custom caption template applied to all delivered media files.\n\n" +
			"<b>Usage:</b>\n" +
			"• <code>/set_caption &lt;template&gt;</code>\n" +
			"• Or reply to any formatted message with <code>/set_caption</code>\n\n" +
			"<b>Supported Placeholders:</b>\n" +
			"• <code>{filename}</code> - Name of the file\n" +
			"• <code>{file sise}</code> - Size of the file (e.g. 1.45 GB)\n" +
			"• <code>{caption}</code> - Original caption\n\n" +
			"<b>Default Model:</b>\n" +
			"<code>CUSTOM CAPTION {filename} {file sise}</code>\n\n" +
			"<b>Example:</b>\n" +
			"<code>/set_caption 🎬 <b>{filename}</b>\n\n📁 <b>Size:</b> {file sise}</code>"

		_, err := m.Reply(guide, &telegram.SendOptions{
			ParseMode: "html",
		})
		return err
	}

	if p.appCtx == nil || p.appCtx.Caption == nil {
		_, err := m.Reply("⚠️ <b>Service Unavailable:</b> Caption service is not ready.", &telegram.SendOptions{
			ParseMode: "html",
		})
		return err
	}

	if err := p.appCtx.Caption.SetCaption(context.Background(), templateText); err != nil {
		_, replyErr := m.Reply(fmt.Sprintf("❌ <b>Failed to set caption:</b> %s", commands.EscapeHTML(err.Error())), &telegram.SendOptions{
			ParseMode: "html",
		})
		return replyErr
	}

	preview := p.appCtx.Caption.Format(templateText, "Inception.2010.1080p.mkv", "1.45 GB", "Inception Movie")
	resp := fmt.Sprintf(
		"✅ <b>Custom Caption Set Successfully!</b>\n\n"+
			"<b>Active Template:</b>\n<code>%s</code>\n\n"+
			"<b>Live Sample Preview:</b>\n%s",
		commands.EscapeHTML(templateText),
		preview,
	)

	_, err := m.Reply(resp, &telegram.SendOptions{
		ParseMode: "html",
	})
	return err
}

// HandleViewCaption displays the active caption template and live preview.
func (p *Plugin) HandleViewCaption(m *telegram.NewMessage) error {
	if m == nil {
		return nil
	}

	if p.appCtx == nil || p.appCtx.Caption == nil {
		_, err := m.Reply("⚠️ <b>Service Unavailable:</b> Caption service is not ready.", &telegram.SendOptions{
			ParseMode: "html",
		})
		return err
	}

	ctx := context.Background()
	currentTmpl := p.appCtx.Caption.GetCaption(ctx)
	isCustom := p.appCtx.Caption.IsCustom()

	status := "Default Template"
	if isCustom {
		status = "Custom Admin Override"
	}

	preview := p.appCtx.Caption.Format(currentTmpl, "Inception.2010.1080p.mkv", "1.45 GB", "Inception Movie")

	resp := fmt.Sprintf(
		"📋 <b><u>Active File Caption Template</u></b>\n\n"+
			"• <b>Status:</b> %s\n"+
			"• <b>Template:</b>\n<code>%s</code>\n\n"+
			"<b>Live Sample Preview:</b>\n%s\n\n"+
			"💡 <i>Use /set_caption to update or /del_caption to reset to default.</i>",
		status,
		commands.EscapeHTML(currentTmpl),
		preview,
	)

	_, err := m.Reply(resp, &telegram.SendOptions{
		ParseMode: "html",
	})
	return err
}

// HandleDelCaption resets the caption configuration back to default.
func (p *Plugin) HandleDelCaption(m *telegram.NewMessage) error {
	if m == nil {
		return nil
	}

	if p.appCtx == nil || p.appCtx.Caption == nil {
		_, err := m.Reply("⚠️ <b>Service Unavailable:</b> Caption service is not ready.", &telegram.SendOptions{
			ParseMode: "html",
		})
		return err
	}

	ctx := context.Background()
	if err := p.appCtx.Caption.DeleteCaption(ctx); err != nil {
		_, replyErr := m.Reply(fmt.Sprintf("❌ <b>Failed to reset caption:</b> %s", commands.EscapeHTML(err.Error())), &telegram.SendOptions{
			ParseMode: "html",
		})
		return replyErr
	}

	defaultTmpl := p.appCtx.Caption.GetCaption(ctx)
	resp := fmt.Sprintf(
		"✅ <b>Custom Caption Reset!</b>\n\n"+
			"Reverted to default template:\n<code>%s</code>",
		commands.EscapeHTML(defaultTmpl),
	)

	_, err := m.Reply(resp, &telegram.SendOptions{
		ParseMode: "html",
	})
	return err
}
