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
			Description: "Set primary Force Subscribe Channel 1 (or forward from channel)",
			Usage:       "/sub1 [channel_id]",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.handleSetSub1,
		},
		{
			Name:        "sub2",
			Aliases:     []string{"set_sub2"},
			Description: "Set primary Force Subscribe Channel 2 (or forward from channel)",
			Usage:       "/sub2 [channel_id]",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.handleSetSub2,
		},
		{
			Name:        "sub3",
			Aliases:     []string{"set_sub3"},
			Description: "Set primary Force Subscribe Channel 3 (or forward from channel)",
			Usage:       "/sub3 [channel_id]",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.handleSetSub3,
		},
		// Secondary Channels
		{
			Name:        "2sub1",
			Aliases:     []string{"setchat1_2"},
			Description: "Set secondary Force Subscribe Channel 1 (or forward from channel)",
			Usage:       "/2sub1 [channel_id]",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.handleSetSecondary1,
		},
		{
			Name:        "2sub2",
			Aliases:     []string{"setchat2_2"},
			Description: "Set secondary Force Subscribe Channel 2 (or forward from channel)",
			Usage:       "/2sub2 [channel_id]",
			Category:    "Admin",
			AdminOnly:   true,
			Handler:     p.handleSetSecondary2,
		},
		{
			Name:        "2sub3",
			Aliases:     []string{"setchat3_2"},
			Description: "Set secondary Force Subscribe Channel 3 (or forward from channel)",
			Usage:       "/2sub3 [channel_id]",
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

// CallbackHandlers registers interactive callback query handlers for fsub plugin.
func (p *Plugin) CallbackHandlers() []plugins.CallbackDefinition {
	return []plugins.CallbackDefinition{
		{
			Prefix:  "close_data",
			Handler: p.HandleCloseCallback,
		},
	}
}

// HandleCloseCallback deletes the prompt message.
func (p *Plugin) HandleCloseCallback(cb *telegram.CallbackQuery) error {
	if cb != nil {
		_, _ = cb.Delete()
		_, _ = cb.Answer("")
	}
	return nil
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
	var chatID int64

	if len(args) >= 2 {
		rawID := strings.TrimSpace(args[1])
		parsedID, err := strconv.ParseInt(rawID, 10, 64)
		if err != nil {
			_, replyErr := m.Reply("❌ <b>Invalid Channel ID!</b> Please provide a valid numeric channel ID or forward a message from the channel.", &telegram.SendOptions{ParseMode: "html"})
			return replyErr
		}
		chatID = parsedID
	} else {
		extractedID, ok := extractChannelIDFromMessage(m)
		if !ok {
			_, err := m.Reply(fmt.Sprintf(
				"⚠️ <b>Channel ID Not Found!</b>\n\n"+
					"You can set the Force Subscribe channel by:\n"+
					"1. Typing <code>/sub%d</code> inside your channel and forwarding that message here.\n"+
					"2. Forwarding any post from your channel here and replying with <code>/sub%d</code>.\n"+
					"3. Providing the numeric Channel ID: <code>/sub%d -1001234567890</code>",
				slot, slot, slot,
			), &telegram.SendOptions{ParseMode: "html"})
			return err
		}
		chatID = extractedID
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

	// Update channel description and report status
	descErr := p.service.UpdateChannelDescription(p.appCtx.Client, chatID, "")
	descStatus := ""
	if descErr != nil {
		descStatus = "\n<b>📋 Channel Description:</b> ⚠️ <i>Could not update description automatically (ensure bot has 'Change Channel Info' permission)</i>"
	} else {
		descStatus = fmt.Sprintf("\n<b>📋 Channel Description:</b> Updated to:\n<i>%s</i>", p.service.GetDefaultChannelDescription())
	}

	text := fmt.Sprintf(
		"✅ <b>Force Subscribe Channel %d Successfully Set!</b>\n\n"+
			"<b>🆔 Chat ID:</b> <code>%d</code>\n"+
			"<b>📝 Chat Name:</b> <code>%s</code>\n"+
			"<b>🔗 Invite Link:</b> <a href=\"%s\">Click Here</a>%s",
		slot, chatID, title, link, descStatus,
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
	var chatID int64

	if len(args) >= 2 {
		rawID := strings.TrimSpace(args[1])
		parsedID, err := strconv.ParseInt(rawID, 10, 64)
		if err != nil {
			_, replyErr := m.Reply("❌ <b>Invalid Channel ID!</b> Please provide a valid numeric channel ID or forward a message from the channel.", &telegram.SendOptions{ParseMode: "html"})
			return replyErr
		}
		chatID = parsedID
	} else {
		extractedID, ok := extractChannelIDFromMessage(m)
		if !ok {
			_, err := m.Reply(fmt.Sprintf(
				"⚠️ <b>Channel ID Not Found!</b>\n\n"+
					"You can set the Secondary Force Subscribe channel by:\n"+
					"1. Typing <code>/2sub%d</code> inside your channel and forwarding that message here.\n"+
					"2. Forwarding any post from your channel here and replying with <code>/2sub%d</code>.\n"+
					"3. Providing the numeric Channel ID: <code>/2sub%d -1001234567890</code>",
				slot, slot, slot,
			), &telegram.SendOptions{ParseMode: "html"})
			return err
		}
		chatID = extractedID
	}

	title, link, err := p.service.SetSecondaryChannel(context.Background(), p.appCtx.Client, slot, chatID)
	if err != nil {
		_, replyErr := m.Reply(fmt.Sprintf(
			"❌ <b>Error setting secondary channel:</b> <code>%s</code>",
			err.Error(),
		), &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}

	descErr := p.service.UpdateChannelDescription(p.appCtx.Client, chatID, "")
	descStatus := ""
	if descErr != nil {
		descStatus = "\n<b>📋 Channel Description:</b> ⚠️ <i>Could not update description automatically (ensure bot has 'Change Channel Info' permission)</i>"
	} else {
		descStatus = fmt.Sprintf("\n<b>📋 Channel Description:</b> Updated to:\n<i>%s</i>", p.service.GetDefaultChannelDescription())
	}

	text := fmt.Sprintf(
		"✅ <b>Secondary Force Subscribe Channel %d Set!</b>\n\n"+
			"<b>🆔 Chat ID:</b> <code>%d</code>\n"+
			"<b>📛 Chat Name:</b> <code>%s</code>\n"+
			"<b>🔗 Invite Link:</b> <a href=\"%s\">Click Here</a>%s",
		slot, chatID, title, link, descStatus,
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

// MessageHandlers registers listeners for forwarded sub commands in DM.
func (p *Plugin) MessageHandlers() []plugins.MessageDefinition {
	return []plugins.MessageDefinition{
		{
			Filter: func(m *telegram.NewMessage) bool {
				if m == nil || !m.IsPrivate() {
					return false
				}
				if p.appCtx == nil || p.appCtx.Config == nil || !p.appCtx.Config.IsAdmin(m.SenderID()) {
					return false
				}
				// Must be forwarded or have FwdFrom
				if !m.IsForward() && (m.Message == nil || m.Message.FwdFrom == nil) {
					return false
				}
				text := strings.TrimSpace(m.Text())
				// If it starts with / or !, command dispatcher handles it; this filter catches plain triggers (e.g. "sub1")
				if strings.HasPrefix(text, "/") || strings.HasPrefix(text, "!") {
					return false
				}
				_, ok := parseSubTrigger(text)
				return ok
			},
			Handler: p.handleForwardedSubCommand,
		},
	}
}

func (p *Plugin) handleForwardedSubCommand(m *telegram.NewMessage) error {
	sub, ok := parseSubTrigger(m.Text())
	if !ok {
		return nil
	}
	if sub.secondary {
		return p.processSetSecondary(m, sub.slot)
	}
	return p.processSetChannel(m, sub.slot)
}

type subCommandType struct {
	slot      int
	secondary bool
}

func parseSubTrigger(text string) (subCommandType, bool) {
	text = strings.ToLower(strings.TrimSpace(text))
	text = strings.TrimPrefix(text, "/")
	text = strings.TrimPrefix(text, "!")
	if idx := strings.Index(text, "@"); idx != -1 {
		text = text[:idx]
	}
	text = strings.TrimSpace(text)

	switch text {
	case "sub1", "set_sub1":
		return subCommandType{slot: 1, secondary: false}, true
	case "sub2", "set_sub2":
		return subCommandType{slot: 2, secondary: false}, true
	case "sub3", "set_sub3":
		return subCommandType{slot: 3, secondary: false}, true
	case "2sub1", "setchat1_2":
		return subCommandType{slot: 1, secondary: true}, true
	case "2sub2", "setchat2_2":
		return subCommandType{slot: 2, secondary: true}, true
	case "2sub3", "setchat3_2":
		return subCommandType{slot: 3, secondary: true}, true
	default:
		return subCommandType{}, false
	}
}

func formatChannelChatID(rawID int64) int64 {
	if rawID < 0 {
		return rawID
	}
	s := fmt.Sprintf("-100%d", rawID)
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return -rawID
	}
	return id
}

func extractChannelIDFromMessage(m *telegram.NewMessage) (int64, bool) {
	if m == nil {
		return 0, false
	}

	// 1. Check forward headers (from FwdFrom)
	if m.Message != nil && m.Message.FwdFrom != nil {
		fwd := m.Message.FwdFrom
		if peerCh, ok := fwd.FromID.(*telegram.PeerChannel); ok && peerCh.ChannelID != 0 {
			return formatChannelChatID(peerCh.ChannelID), true
		}
		if peerChat, ok := fwd.FromID.(*telegram.PeerChat); ok && peerChat.ChatID != 0 {
			return -peerChat.ChatID, true
		}
		if peerCh, ok := fwd.SavedFromPeer.(*telegram.PeerChannel); ok && peerCh.ChannelID != 0 {
			return formatChannelChatID(peerCh.ChannelID), true
		}
		if peerChat, ok := fwd.SavedFromPeer.(*telegram.PeerChat); ok && peerChat.ChatID != 0 {
			return -peerChat.ChatID, true
		}
		if peerCh, ok := fwd.SavedFromID.(*telegram.PeerChannel); ok && peerCh.ChannelID != 0 {
			return formatChannelChatID(peerCh.ChannelID), true
		}
		if peerChat, ok := fwd.SavedFromID.(*telegram.PeerChat); ok && peerChat.ChatID != 0 {
			return -peerChat.ChatID, true
		}
	}

	// 2. Check SenderChat or Channel if message came directly from or within a channel
	if m.SenderChat != nil && m.SenderChat.ID != 0 {
		return formatChannelChatID(m.SenderChat.ID), true
	}
	if m.Channel != nil && m.Channel.ID != 0 {
		return formatChannelChatID(m.Channel.ID), true
	}

	// 3. If replied to another message, extract from replied message
	if m.IsReply() {
		if replyMsg, err := m.GetReplyMessage(); err == nil && replyMsg != nil {
			if id, ok := extractChannelIDFromMessage(replyMsg); ok {
				return id, true
			}
		}
	}

	// 4. If executed inside a channel
	if m.ChannelID() != 0 {
		return formatChannelChatID(m.ChannelID()), true
	}
	if m.IsChannel() && m.ChatID() != 0 {
		return formatChannelChatID(m.ChatID()), true
	}

	return 0, false
}
