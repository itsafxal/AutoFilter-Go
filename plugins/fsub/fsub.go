package fsub

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/MYTGBOTZ/internal/fsub"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
)

// Plugin implements the Force Subscribe management plugin and join request listener.
type Plugin struct {
	appCtx  *plugins.AppContext
	service *fsub.Service
}

// New creates a new FSub plugin instance.
func New(appCtx *plugins.AppContext, service *fsub.Service) *Plugin {
	return &Plugin{
		appCtx:  appCtx,
		service: service,
	}
}

// Name returns the unique plugin name.
func (p *Plugin) Name() string {
	return "fsub"
}

// Commands returns the administrative commands for managing Force Subscribe channels and thresholds.
func (p *Plugin) Commands() []plugins.CommandDefinition {
	return []plugins.CommandDefinition{
		// Primary Channels
		{
			Name:        "sub1",
			Aliases:     []string{"set_sub1"},
			Description: "Set primary Force Subscribe Channel 1",
			Usage:       "/sub1 <channel_id>",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.handleSetSub1,
		},
		{
			Name:        "sub2",
			Aliases:     []string{"set_sub2"},
			Description: "Set primary Force Subscribe Channel 2",
			Usage:       "/sub2 <channel_id>",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.handleSetSub2,
		},
		{
			Name:        "sub3",
			Aliases:     []string{"set_sub3"},
			Description: "Set primary Force Subscribe Channel 3",
			Usage:       "/sub3 <channel_id>",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.handleSetSub3,
		},
		// Secondary Channels
		{
			Name:        "2sub1",
			Aliases:     []string{"setchat1_2"},
			Description: "Set secondary Force Subscribe Channel 1 for threshold switching",
			Usage:       "/2sub1 <channel_id>",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.handleSetSecondary1,
		},
		{
			Name:        "2sub2",
			Aliases:     []string{"setchat2_2"},
			Description: "Set secondary Force Subscribe Channel 2 for threshold switching",
			Usage:       "/2sub2 <channel_id>",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.handleSetSecondary2,
		},
		{
			Name:        "2sub3",
			Aliases:     []string{"setchat3_2"},
			Description: "Set secondary Force Subscribe Channel 3 for threshold switching",
			Usage:       "/2sub3 <channel_id>",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.handleSetSecondary3,
		},
		// Thresholds
		{
			Name:        "setcount1",
			Description: "Set join request count threshold for Channel 1",
			Usage:       "/setcount1 <count>",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.handleSetCount1,
		},
		{
			Name:        "setcount2",
			Description: "Set join request count threshold for Channel 2",
			Usage:       "/setcount2 <count>",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.handleSetCount2,
		},
		{
			Name:        "setcount3",
			Description: "Set join request count threshold for Channel 3",
			Usage:       "/setcount3 <count>",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.handleSetCount3,
		},
		// Overview / Status
		{
			Name:        "view_sub",
			Aliases:     []string{"viewsub", "fsub_status"},
			Description: "View active Force Subscribe channels status and request counts",
			Usage:       "/view_sub",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.handleViewSub,
		},
		// Deletions - Primary
		{
			Name:        "delsub1",
			Description: "Remove primary Force Subscribe Channel 1",
			Usage:       "/delsub1",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.handleDelSub1,
		},
		{
			Name:        "delsub2",
			Description: "Remove primary Force Subscribe Channel 2",
			Usage:       "/delsub2",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.handleDelSub2,
		},
		{
			Name:        "delsub3",
			Description: "Remove primary Force Subscribe Channel 3",
			Usage:       "/delsub3",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.handleDelSub3,
		},
		// Deletions - Secondary
		{
			Name:        "2delsub1",
			Description: "Remove secondary Force Subscribe Channel 1",
			Usage:       "/2delsub1",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.handleDelSecondary1,
		},
		{
			Name:        "2delsub2",
			Description: "Remove secondary Force Subscribe Channel 2",
			Usage:       "/2delsub2",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.handleDelSecondary2,
		},
		{
			Name:        "2delsub3",
			Description: "Remove secondary Force Subscribe Channel 3",
			Usage:       "/2delsub3",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.handleDelSecondary3,
		},
	}
}

// JoinRequestHandlers registers the MTProto chat join request listener.
func (p *Plugin) JoinRequestHandlers() []plugins.JoinRequestHandlerFunc {
	return []plugins.JoinRequestHandlerFunc{
		func(req *telegram.JoinRequestUpdate) error {
			if p.service != nil && p.appCtx != nil {
				return p.service.HandleJoinRequest(p.appCtx.Client, req)
			}
			return nil
		},
	}
}

// Handlers for Primary Channels

func (p *Plugin) handleSetSub1(m *telegram.NewMessage) error {
	return p.processSetChannel(m, 1)
}

func (p *Plugin) handleSetSub2(m *telegram.NewMessage) error {
	return p.processSetChannel(m, 2)
}

func (p *Plugin) handleSetSub3(m *telegram.NewMessage) error {
	return p.processSetChannel(m, 3)
}

func (p *Plugin) processSetChannel(m *telegram.NewMessage, slot int) error {
	args := strings.Fields(m.Text())
	if len(args) < 2 {
		_, err := m.Reply(fmt.Sprintf(
			"Please provide the Force Subscribe Channel ID.\n\n<b>Usage:</b> <code>/sub%d -100123456789</code>",
			slot,
		), &telegram.SendOptions{ParseMode: "html"})
		return err
	}

	rawID := strings.TrimSpace(args[1])
	chatID, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		_, replyErr := m.Reply("❌ <b>Invalid Channel ID!</b> Please provide a valid numeric channel ID.", &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}

	title, link, err := p.service.SetChannel(context.Background(), p.appCtx.Client, slot, chatID)
	if err != nil {
		_, replyErr := m.Reply(fmt.Sprintf(
			"❌ <b>Access Denied / Error:</b>\n\n<code>%s</code>\n\n"+
				"Please ensure the bot is an admin in the channel with invite link creation permissions.",
			err.Error(),
		), &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}

	text := fmt.Sprintf(
		"✅ <b>Force Subscribe Channel %d Successfully Set!</b>\n\n"+
			"<b>🆔 Chat ID:</b> <code>%d</code>\n"+
			"<b>📝 Chat Name:</b> <code>%s</code>\n"+
			"<b>🔗 Invite Link:</b> <a href=\"%s\">Click Here</a>",
		slot, chatID, title, link,
	)

	_, replyErr := m.Reply(text, &telegram.SendOptions{
		ParseMode: "html",
	})
	return replyErr
}

// Handlers for Secondary Channels

func (p *Plugin) handleSetSecondary1(m *telegram.NewMessage) error {
	return p.processSetSecondary(m, 1)
}

func (p *Plugin) handleSetSecondary2(m *telegram.NewMessage) error {
	return p.processSetSecondary(m, 2)
}

func (p *Plugin) handleSetSecondary3(m *telegram.NewMessage) error {
	return p.processSetSecondary(m, 3)
}

func (p *Plugin) processSetSecondary(m *telegram.NewMessage, slot int) error {
	args := strings.Fields(m.Text())
	if len(args) < 2 {
		_, err := m.Reply(fmt.Sprintf(
			"Please provide the Secondary Force Subscribe Channel ID for Channel %d.\n\n<b>Usage:</b> <code>/2sub%d -100123456789</code>",
			slot, slot,
		), &telegram.SendOptions{ParseMode: "html"})
		return err
	}

	rawID := strings.TrimSpace(args[1])
	chatID, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		_, replyErr := m.Reply("❌ <b>Invalid Channel ID!</b> Please provide a valid numeric channel ID.", &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}

	title, link, err := p.service.SetSecondaryChannel(context.Background(), p.appCtx.Client, slot, chatID)
	if err != nil {
		_, replyErr := m.Reply(fmt.Sprintf(
			"❌ <b>Error setting secondary channel:</b> <code>%s</code>",
			err.Error(),
		), &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}

	text := fmt.Sprintf(
		"✅ <b>Secondary Force Subscribe Channel %d Set!</b>\n\n"+
			"🆔 <code>%d</code>\n"+
			"📛 <code>%s</code>\n"+
			"🔗 <a href=\"%s\">Invite Link</a>",
		slot, chatID, title, link,
	)

	_, replyErr := m.Reply(text, &telegram.SendOptions{
		ParseMode: "html",
	})
	return replyErr
}

// Handlers for Thresholds

func (p *Plugin) handleSetCount1(m *telegram.NewMessage) error {
	return p.processSetCount(m, 1)
}

func (p *Plugin) handleSetCount2(m *telegram.NewMessage) error {
	return p.processSetCount(m, 2)
}

func (p *Plugin) handleSetCount3(m *telegram.NewMessage) error {
	return p.processSetCount(m, 3)
}

func (p *Plugin) processSetCount(m *telegram.NewMessage, slot int) error {
	args := strings.Fields(m.Text())
	if len(args) < 2 {
		_, err := m.Reply(fmt.Sprintf(
			"Please provide the join request count threshold for Channel %d.\n\n<b>Usage:</b> <code>/setcount%d 100</code>",
			slot, slot,
		), &telegram.SendOptions{ParseMode: "html"})
		return err
	}

	count, err := strconv.Atoi(strings.TrimSpace(args[1]))
	if err != nil || count <= 0 {
		_, replyErr := m.Reply("❌ <b>Count must be a positive number!</b>", &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}

	if err := p.service.SetCountThreshold(context.Background(), slot, count); err != nil {
		_, replyErr := m.Reply(fmt.Sprintf("❌ Error setting threshold: <code>%s</code>", err.Error()), &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}

	text := fmt.Sprintf(
		"✅ <b>Count Threshold for Channel %d Successfully Set!</b>\n\n"+
			"<b>📊 Threshold:</b> <code>%d</code> join requests\n"+
			"<b>📝 Note:</b> When this threshold is exceeded, the system will switch to the secondary channel (if set).",
		slot, count,
	)

	_, replyErr := m.Reply(text, &telegram.SendOptions{
		ParseMode: "html",
	})
	return replyErr
}

// Handler for /view_sub Status Overview

func (p *Plugin) handleViewSub(m *telegram.NewMessage) error {
	statusText, err := p.service.GetStatus(p.appCtx.Client)
	if err != nil {
		_, replyErr := m.Reply(fmt.Sprintf("❌ An error occurred while fetching channels: <code>%s</code>", err.Error()), &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}

	_, replyErr := m.Reply(statusText, &telegram.SendOptions{
		ParseMode: "html",
	})
	return replyErr
}

// Handlers for Deleting Channels

func (p *Plugin) handleDelSub1(m *telegram.NewMessage) error {
	return p.processDelSub(m, 1)
}

func (p *Plugin) handleDelSub2(m *telegram.NewMessage) error {
	return p.processDelSub(m, 2)
}

func (p *Plugin) handleDelSub3(m *telegram.NewMessage) error {
	return p.processDelSub(m, 3)
}

func (p *Plugin) processDelSub(m *telegram.NewMessage, slot int) error {
	oldID, err := p.service.DeleteChannel(context.Background(), slot)
	if err != nil {
		_, replyErr := m.Reply(fmt.Sprintf("⚠️ <b>%s</b>", err.Error()), &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}

	text := fmt.Sprintf(
		"🗑️ <b>Force Subscribe Channel %d Successfully Removed!</b>\n\n"+
			"<b>🆔 Chat ID:</b> <code>%d</code>",
		slot, oldID,
	)

	_, replyErr := m.Reply(text, &telegram.SendOptions{
		ParseMode: "html",
	})
	return replyErr
}

func (p *Plugin) handleDelSecondary1(m *telegram.NewMessage) error {
	return p.processDelSecondary(m, 1)
}

func (p *Plugin) handleDelSecondary2(m *telegram.NewMessage) error {
	return p.processDelSecondary(m, 2)
}

func (p *Plugin) handleDelSecondary3(m *telegram.NewMessage) error {
	return p.processDelSecondary(m, 3)
}

func (p *Plugin) processDelSecondary(m *telegram.NewMessage, slot int) error {
	oldID, err := p.service.DeleteSecondaryChannel(context.Background(), slot)
	if err != nil {
		_, replyErr := m.Reply(fmt.Sprintf("⚠️ <b>%s</b>", err.Error()), &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}

	text := fmt.Sprintf(
		"🗑️ <b>Secondary Force Subscribe Channel %d Successfully Removed!</b>\n\n"+
			"<b>🆔 Chat ID:</b> <code>%d</code>",
		slot, oldID,
	)

	_, replyErr := m.Reply(text, &telegram.SendOptions{
		ParseMode: "html",
	})
	return replyErr
}
