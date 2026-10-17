package autodelete

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/amarnathcjd/gogram/telegram"
	coreAutoDel "github.com/itsafxal/MYTGBOTZ/internal/autodelete"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
)

// Plugin implements administrative commands for Auto-Delete (Filter & File) and Weekly Trash.
type Plugin struct {
	appCtx  *plugins.AppContext
	service coreAutoDel.AutoDeleteService
}

// New creates a new autodelete plugin instance.
func New(appCtx *plugins.AppContext, service coreAutoDel.AutoDeleteService) *Plugin {
	return &Plugin{
		appCtx:  appCtx,
		service: service,
	}
}

// Name returns the unique plugin name.
func (p *Plugin) Name() string {
	return "autodelete"
}

// Commands returns the administrative commands for auto-delete and trash management.
func (p *Plugin) Commands() []plugins.CommandDefinition {
	return []plugins.CommandDefinition{
		{
			Name:        "autodel",
			Aliases:     []string{"autodelete", "set_del"},
			Description: "View and configure auto-delete timers (filter & file) and weekly trash",
			Usage:       "/autodel [filter|file|weekly|retention] [value]",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.handleAutoDel,
		},
		{
			Name:        "set_filter_del",
			Aliases:     []string{"autodel_filter"},
			Description: "Set auto-delete timer for filter search results (e.g. 15m, 1h, 0 to disable)",
			Usage:       "/set_filter_del <duration>",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.handleSetFilterDel,
		},
		{
			Name:        "set_file_del",
			Aliases:     []string{"autodel_file"},
			Description: "Set auto-delete timer for delivered media files (e.g. 30m, 2h, 0 to disable)",
			Usage:       "/set_file_del <duration>",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.handleSetFileDel,
		},
		{
			Name:        "clean_trash",
			Aliases:     []string{"weekly_trash", "trash"},
			Description: "Manually trigger weekly trash cleanup to delete old bot messages and prevent copyright strikes",
			Usage:       "/clean_trash",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.handleCleanTrash,
		},
	}
}

func (p *Plugin) handleAutoDel(m *telegram.NewMessage) error {
	if p.service == nil {
		_, err := m.Reply("⚠️ <b>Auto-Delete service is not available.</b>", &telegram.SendOptions{ParseMode: "html"})
		return err
	}

	args := strings.Fields(m.Text())
	ctx := context.Background()

	// If arguments are provided: /autodel filter <duration>, /autodel file <duration>, etc.
	if len(args) >= 3 {
		subCmd := strings.ToLower(args[1])
		val := args[2]

		switch subCmd {
		case "filter", "filters":
			d, err := coreAutoDel.ParseDuration(val)
			if err != nil {
				_, replyErr := m.Reply(fmt.Sprintf("❌ <b>Invalid Duration:</b> <code>%s</code>\n\nUse e.g. <code>10m</code>, <code>30m</code>, <code>1h</code>, or <code>0</code> to disable.", val), &telegram.SendOptions{ParseMode: "html"})
				return replyErr
			}
			if err := p.service.SetFilterDuration(ctx, d); err != nil {
				_, replyErr := m.Reply(fmt.Sprintf("❌ Error updating filter duration: <code>%s</code>", err.Error()), &telegram.SendOptions{ParseMode: "html"})
				return replyErr
			}
			_, replyErr := m.Reply(fmt.Sprintf("✅ <b>Filter Auto-Delete Updated:</b> <code>%s</code>", coreAutoDel.FormatDuration(d)), &telegram.SendOptions{ParseMode: "html"})
			return replyErr

		case "file", "files":
			d, err := coreAutoDel.ParseDuration(val)
			if err != nil {
				_, replyErr := m.Reply(fmt.Sprintf("❌ <b>Invalid Duration:</b> <code>%s</code>\n\nUse e.g. <code>15m</code>, <code>30m</code>, <code>1h</code>, or <code>0</code> to disable.", val), &telegram.SendOptions{ParseMode: "html"})
				return replyErr
			}
			if err := p.service.SetFileDuration(ctx, d); err != nil {
				_, replyErr := m.Reply(fmt.Sprintf("❌ Error updating file duration: <code>%s</code>", err.Error()), &telegram.SendOptions{ParseMode: "html"})
				return replyErr
			}
			_, replyErr := m.Reply(fmt.Sprintf("✅ <b>File Auto-Delete Updated:</b> <code>%s</code>", coreAutoDel.FormatDuration(d)), &telegram.SendOptions{ParseMode: "html"})
			return replyErr

		case "weekly", "trash":
			enabled := strings.EqualFold(val, "on") || strings.EqualFold(val, "true") || val == "1" || strings.EqualFold(val, "enable")
			if err := p.service.SetWeeklyTrash(ctx, enabled); err != nil {
				_, replyErr := m.Reply(fmt.Sprintf("❌ Error toggling weekly trash: <code>%s</code>", err.Error()), &telegram.SendOptions{ParseMode: "html"})
				return replyErr
			}
			statusStr := "Enabled"
			if !enabled {
				statusStr = "Disabled"
			}
			_, replyErr := m.Reply(fmt.Sprintf("✅ <b>Weekly Trash Cleanup:</b> <code>%s</code>", statusStr), &telegram.SendOptions{ParseMode: "html"})
			return replyErr

		case "retention", "days":
			days, err := strconv.Atoi(val)
			if err != nil || days <= 0 {
				_, replyErr := m.Reply("❌ <b>Invalid Retention Days:</b> Must be a positive integer (e.g. 7).", &telegram.SendOptions{ParseMode: "html"})
				return replyErr
			}
			if err := p.service.SetTrashRetention(ctx, days); err != nil {
				_, replyErr := m.Reply(fmt.Sprintf("❌ Error setting retention: <code>%s</code>", err.Error()), &telegram.SendOptions{ParseMode: "html"})
				return replyErr
			}
			_, replyErr := m.Reply(fmt.Sprintf("✅ <b>Weekly Trash Retention Updated:</b> <code>%d days</code>", days), &telegram.SendOptions{ParseMode: "html"})
			return replyErr
		}
	}

	// Default: Display full auto-delete dashboard
	settings := p.service.GetSettings()
	stats, _ := p.service.GetStats(ctx)

	weeklyStatus := "Enabled"
	if !settings.WeeklyTrashEnabled {
		weeklyStatus = "Disabled"
	}

	lastCleanedStr := "Never"
	if !stats.LastCleanedAt.IsZero() {
		lastCleanedStr = fmt.Sprintf("%s (%d messages deleted)", stats.LastCleanedAt.Format("02 Jan 15:04"), stats.LastDeletedCount)
	}

	dashboard := fmt.Sprintf(
		"🗑️ <b><u>AUTO-DELETE & WEEKLY TRASH CONTROLS</u></b> 🗑️\n\n"+
			"🔍 <b>Filter Auto-Delete:</b> <code>%s</code>\n"+
			"📁 <b>File Auto-Delete:</b> <code>%s</code>\n"+
			"♻️ <b>Weekly Trash:</b> <code>%s</code> (Retention: %d days)\n\n"+
			"📊 <b><u>Database & Operations Status:</u></b>\n"+
			"• <b>Pending Auto-Deletions:</b> <code>%d</code> (survives restarts)\n"+
			"• <b>Tracked Bot Messages:</b> <code>%d</code>\n"+
			"• <b>Last Trash Cleanup:</b> <code>%s</code>\n\n"+
			"⚙️ <b><u>Configuration Commands:</u></b>\n"+
			"• <code>/autodel filter &lt;time&gt;</code> - Set search results deletion timer (e.g. <code>10m</code>, <code>30m</code>, <code>0</code>)\n"+
			"• <code>/autodel file &lt;time&gt;</code> - Set delivered files deletion timer (e.g. <code>30m</code>, <code>1h</code>, <code>0</code>)\n"+
			"• <code>/autodel weekly &lt;on/off&gt;</code> - Toggle weekly trash cleaner\n"+
			"• <code>/autodel retention &lt;days&gt;</code> - Set trash retention days (e.g. <code>7</code>)\n"+
			"• <code>/clean_trash</code> - Manually trigger trash cleanup now",
		coreAutoDel.FormatDuration(settings.FilterDeleteDuration),
		coreAutoDel.FormatDuration(settings.FileDeleteDuration),
		weeklyStatus,
		settings.TrashRetentionDays,
		stats.PendingDeletionsCount,
		stats.TrackedMessagesCount,
		lastCleanedStr,
	)

	_, replyErr := m.Reply(dashboard, &telegram.SendOptions{
		ParseMode: "html",
	})
	return replyErr
}

func (p *Plugin) handleSetFilterDel(m *telegram.NewMessage) error {
	if p.service == nil {
		_, err := m.Reply("⚠️ <b>Auto-Delete service is not available.</b>", &telegram.SendOptions{ParseMode: "html"})
		return err
	}

	args := strings.Fields(m.Text())
	if len(args) < 2 {
		_, err := m.Reply("Please provide the filter auto-delete duration.\n\n<b>Usage:</b> <code>/set_filter_del 15m</code> (or <code>0</code> to disable)", &telegram.SendOptions{ParseMode: "html"})
		return err
	}

	d, err := coreAutoDel.ParseDuration(args[1])
	if err != nil {
		_, replyErr := m.Reply(fmt.Sprintf("❌ <b>Invalid Duration:</b> <code>%s</code>\n\nUse e.g. <code>10m</code>, <code>30m</code>, <code>1h</code>, or <code>0</code> to disable.", args[1]), &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}

	if err := p.service.SetFilterDuration(context.Background(), d); err != nil {
		_, replyErr := m.Reply(fmt.Sprintf("❌ Error updating filter duration: <code>%s</code>", err.Error()), &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}

	text := fmt.Sprintf(
		"✅ <b>Filter Auto-Delete Timer Updated!</b>\n\n"+
			"⏱️ <b>New Timer:</b> <code>%s</code>\n"+
			"ℹ️ Search result messages will be automatically deleted after this duration across chats.",
		coreAutoDel.FormatDuration(d),
	)

	_, replyErr := m.Reply(text, &telegram.SendOptions{ParseMode: "html"})
	return replyErr
}

func (p *Plugin) handleSetFileDel(m *telegram.NewMessage) error {
	if p.service == nil {
		_, err := m.Reply("⚠️ <b>Auto-Delete service is not available.</b>", &telegram.SendOptions{ParseMode: "html"})
		return err
	}

	args := strings.Fields(m.Text())
	if len(args) < 2 {
		_, err := m.Reply("Please provide the file auto-delete duration.\n\n<b>Usage:</b> <code>/set_file_del 30m</code> (or <code>0</code> to disable)", &telegram.SendOptions{ParseMode: "html"})
		return err
	}

	d, err := coreAutoDel.ParseDuration(args[1])
	if err != nil {
		_, replyErr := m.Reply(fmt.Sprintf("❌ <b>Invalid Duration:</b> <code>%s</code>\n\nUse e.g. <code>15m</code>, <code>30m</code>, <code>1h</code>, or <code>0</code> to disable.", args[1]), &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}

	if err := p.service.SetFileDuration(context.Background(), d); err != nil {
		_, replyErr := m.Reply(fmt.Sprintf("❌ Error updating file duration: <code>%s</code>", err.Error()), &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}

	text := fmt.Sprintf(
		"✅ <b>File Auto-Delete Timer Updated!</b>\n\n"+
			"⏱️ <b>New Timer:</b> <code>%s</code>\n"+
			"ℹ️ Delivered media files will be automatically deleted after this duration to prevent copyright strikes.",
		coreAutoDel.FormatDuration(d),
	)

	_, replyErr := m.Reply(text, &telegram.SendOptions{ParseMode: "html"})
	return replyErr
}

func (p *Plugin) handleCleanTrash(m *telegram.NewMessage) error {
	if p.service == nil {
		_, err := m.Reply("⚠️ <b>Auto-Delete service is not available.</b>", &telegram.SendOptions{ParseMode: "html"})
		return err
	}

	statusMsg, _ := m.Reply("♻️ <b>Running Weekly Trash Cleanup...</b>\n<i>Scanning and purging messages older than retention window to protect from copyright strikes...</i>", &telegram.SendOptions{ParseMode: "html"})

	deletedCount, err := p.service.RunWeeklyTrash(context.Background())
	if err != nil {
		_, replyErr := m.Reply(fmt.Sprintf("❌ <b>Trash Cleanup Error:</b> <code>%s</code>", err.Error()), &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}

	replyText := fmt.Sprintf(
		"🗑️ <b>Weekly Trash Cleanup Completed!</b>\n\n"+
			"• <b>Messages Purged:</b> <code>%d</code>\n"+
			"• <b>Status:</b> All old bot messages older than %d days have been deleted across Telegram chats.\n"+
			"• <b>Protection:</b> Active against copyright strikes 🛡️",
		deletedCount,
		p.service.GetSettings().TrashRetentionDays,
	)

	if statusMsg != nil {
		_, err = statusMsg.Edit(replyText, &telegram.SendOptions{ParseMode: "html"})
		return err
	}

	_, err = m.Reply(replyText, &telegram.SendOptions{ParseMode: "html"})
	return err
}
