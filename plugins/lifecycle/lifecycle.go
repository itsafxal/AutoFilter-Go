package lifecycle

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/AutoFilter-Go/internal/commands"
	"github.com/itsafxal/AutoFilter-Go/internal/plugins"
)

// Plugin implements the Bot Lifecycle, Group Welcome, and Admin Notification system.
type Plugin struct {
	appCtx   *plugins.AppContext
	dedupMap sync.Map
}

// New creates a new lifecycle plugin instance.
func New(appCtx *plugins.AppContext) *Plugin {
	return &Plugin{
		appCtx: appCtx,
	}
}

// Name returns the unique plugin identifier.
func (p *Plugin) Name() string {
	return "lifecycle"
}

// Commands registers administrative lifecycle management commands.
func (p *Plugin) Commands() []plugins.CommandDefinition {
	return []plugins.CommandDefinition{
		{
			Name:        "leave",
			Aliases:     []string{"leave_chat"},
			Description: "Instruct the bot to leave the current group or a specified chat ID",
			Usage:       "/leave [chat_id]",
			Category:    "Group Management",
			AdminOnly:   true,
			Handler:     p.HandleLeave,
		},
		{
			Name:        "disable",
			Aliases:     []string{"disable_chat"},
			Description: "Disable search and media delivery in the current group or specified chat ID",
			Usage:       "/disable [chat_id] [reason]",
			Category:    "Group Management",
			AdminOnly:   true,
			Handler:     p.HandleDisable,
		},
		{
			Name:        "enable",
			Aliases:     []string{"enable_chat", "re_enable"},
			Description: "Re-enable search and media delivery in the current group or specified chat ID",
			Usage:       "/enable [chat_id]",
			Category:    "Group Management",
			AdminOnly:   true,
			Handler:     p.HandleEnable,
		},
		{
			Name:        "groups",
			Aliases:     []string{"chats"},
			Description: "View total registered and active connected group counts",
			Usage:       "/groups",
			Category:    "Group Management",
			AdminOnly:   true,
			Handler:     p.HandleGroups,
		},
	}
}

// ParticipantHandlers registers participant update listeners (for supergroups and channels).
func (p *Plugin) ParticipantHandlers() []plugins.ParticipantUpdateHandlerFunc {
	return []plugins.ParticipantUpdateHandlerFunc{
		p.HandleParticipantUpdate,
	}
}

// MessageHandlers registers action message listeners (for basic groups).
func (p *Plugin) MessageHandlers() []plugins.MessageDefinition {
	return []plugins.MessageDefinition{
		{
			Filter: func(m *telegram.NewMessage) bool {
				if m == nil || m.Action == nil {
					return false
				}
				switch m.Action.(type) {
				case *telegram.MessageActionChatAddUser, *telegram.MessageActionChatDeleteUser:
					return true
				default:
					return false
				}
			},
			Handler: p.HandleChatAction,
		},
	}
}

// HandleParticipantUpdate intercepts member status changes in supergroups and channels.
func (p *Plugin) HandleParticipantUpdate(upd *telegram.ParticipantUpdate) error {
	if upd == nil || p.appCtx == nil {
		return nil
	}

	botID := p.getBotID()
	if botID == 0 {
		return nil
	}

	// Only process events where the affected target is this bot
	if upd.UserID() != botID {
		return nil
	}

	chatID := upd.ChannelID()
	if chatID == 0 {
		chatID = -1000000000000 - upd.ChatID()
	}

	chatTitle := "Group"
	if upd.Channel != nil && upd.Channel.Title != "" {
		chatTitle = upd.Channel.Title
	}

	chatUsername := ""
	if upd.Channel != nil && upd.Channel.Username != "" {
		chatUsername = upd.Channel.Username
	}

	// 1. Bot Added / Promoted in Group
	if upd.IsAdded() || upd.IsJoined() || upd.IsPromoted() {
		// Verify transition was not an existing member
		if upd.Old == nil || upd.IsPromoted() {
			p.OnBotAdded(chatID, chatTitle, chatUsername, upd.Actor, upd.ActorID())
			return nil
		}
	}

	// 2. Bot Removed / Banned from Group
	if upd.IsLeft() || upd.IsBanned() || upd.IsKicked() {
		p.OnBotRemoved(chatID, chatTitle, upd.Actor, upd.ActorID())
		return nil
	}

	return nil
}

// HandleChatAction intercepts add/delete member actions in basic group chats.
func (p *Plugin) HandleChatAction(m *telegram.NewMessage) error {
	if m == nil || m.Action == nil || p.appCtx == nil {
		return nil
	}

	botID := p.getBotID()
	if botID == 0 {
		return nil
	}

	chatID := m.ChatID()
	chatTitle := "Group"
	if m.Chat != nil && m.Chat.Title != "" {
		chatTitle = m.Chat.Title
	}

	switch act := m.Action.(type) {
	case *telegram.MessageActionChatAddUser:
		isBotAdded := false
		for _, uID := range act.Users {
			if uID == botID {
				isBotAdded = true
				break
			}
		}
		if isBotAdded {
			p.OnBotAdded(chatID, chatTitle, "", m.Sender, m.SenderID())
		}
	case *telegram.MessageActionChatDeleteUser:
		if act.UserID == botID {
			p.OnBotRemoved(chatID, chatTitle, m.Sender, m.SenderID())
		}
	}

	return nil
}

// OnBotAdded executes the registration, welcome banner, and admin notification workflow.
func (p *Plugin) OnBotAdded(chatID int64, chatTitle string, chatUsername string, actor *telegram.UserObj, actorID int64) {
	if p.isDuplicateEvent("add", chatID) {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Register group in authoritative MongoDB database
	if p.appCtx.DB != nil && p.appCtx.DB.GroupRepo() != nil {
		_ = p.appCtx.DB.GroupRepo().AddGroup(ctx, chatID, chatTitle)
		// Ensure group is re-enabled if it was previously marked disabled
		_ = p.appCtx.DB.GroupRepo().EnableGroup(ctx, chatID)
	}

	botUsername := p.getBotUsername()
	repoURL := "https://github.com/itsafxal/AutoFilter-Go"
	enableWelcome := true
	var deleteDelay time.Duration

	if p.appCtx.Config != nil {
		enableWelcome = p.appCtx.Config.EnableGroupWelcome
		deleteDelay = p.appCtx.Config.WelcomeDeleteDelay
		if p.appCtx.Config.RepoURL != "" {
			repoURL = p.appCtx.Config.RepoURL
		}
	}

	// 2. Format & Deliver Group Welcome Banner
	if enableWelcome && p.appCtx.Client != nil {
		actorName := "Group Admin"
		if actor != nil && actor.FirstName != "" {
			actorName = strings.TrimSpace(actor.FirstName + " " + actor.LastName)
		}

		welcomeText := FormatGroupWelcomeText(botUsername, chatTitle, chatID, actorName, actorID)
		replyMarkup := BuildGroupWelcomeKeyboard(botUsername, repoURL)

		sentMsg, err := p.appCtx.Client.SendMessage(chatID, welcomeText, &telegram.SendOptions{
			ParseMode:   "html",
			ReplyMarkup: replyMarkup,
		})

		// Schedule safe welcome message auto-deletion if configured
		if err == nil && sentMsg != nil && deleteDelay > 0 {
			msgID := sentMsg.ID
			client := p.appCtx.Client
			time.AfterFunc(deleteDelay, func() {
				_, _ = client.DeleteMessages(chatID, []int32{msgID})
			})
		}
	}

	// 3. Dispatch Admin DM Notifications
	p.notifyAdminsNewGroup(chatID, chatTitle, chatUsername, actor, actorID)
}

// OnBotRemoved executes database status updates and admin removal notifications.
func (p *Plugin) OnBotRemoved(chatID int64, chatTitle string, actor *telegram.UserObj, actorID int64) {
	if p.isDuplicateEvent("remove", chatID) {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Mark group disabled in database
	if p.appCtx.DB != nil && p.appCtx.DB.GroupRepo() != nil {
		_ = p.appCtx.DB.GroupRepo().DisableGroup(ctx, chatID, "Bot removed from group")
	}

	// 2. Dispatch Admin Removal Notifications
	p.notifyAdminsBotRemoved(chatID, chatTitle, actor, actorID)
}

// notifyAdminsNewGroup sends a structured DM notification to all configured administrators.
func (p *Plugin) notifyAdminsNewGroup(chatID int64, chatTitle string, chatUsername string, actor *telegram.UserObj, actorID int64) {
	if p.appCtx == nil || p.appCtx.Client == nil || p.appCtx.Config == nil {
		return
	}

	actorName := "Admin"
	actorUsername := "None"
	if actor != nil {
		if actor.FirstName != "" {
			actorName = strings.TrimSpace(actor.FirstName + " " + actor.LastName)
		}
		if actor.Username != "" {
			actorUsername = "@" + actor.Username
		}
	}

	botUsername := p.getBotUsername()
	now := time.Now().UTC().Format("02 Jan 2006, 15:04 UTC")
	groupLink := "Private"
	if chatUsername != "" {
		groupLink = "@" + chatUsername
	}

	notifText := FormatAdminNewGroupNotification(
		chatTitle,
		chatID,
		groupLink,
		actorName,
		actorID,
		actorUsername,
		now,
		botUsername,
	)

	adminRecipients := p.getAdminRecipients()
	for _, adminID := range adminRecipients {
		adminTarget := adminID
		go func(targetID int64) {
			_, err := p.appCtx.Client.SendMessage(targetID, notifText, &telegram.SendOptions{
				ParseMode: "html",
			})
			if err != nil && p.appCtx.Logger != nil {
				p.appCtx.Logger.Debug("admin new group DM failed (admin may have not started bot)", "admin_id", targetID, "error", err)
			}
		}(adminTarget)
	}
}

// notifyAdminsBotRemoved sends a structured DM removal notice to all configured administrators.
func (p *Plugin) notifyAdminsBotRemoved(chatID int64, chatTitle string, actor *telegram.UserObj, actorID int64) {
	if p.appCtx == nil || p.appCtx.Client == nil || p.appCtx.Config == nil {
		return
	}

	actionByName := "Admin"
	if actor != nil && actor.FirstName != "" {
		actionByName = strings.TrimSpace(actor.FirstName + " " + actor.LastName)
	}

	botUsername := p.getBotUsername()
	now := time.Now().UTC().Format("02 Jan 2006, 15:04 UTC")

	notifText := FormatAdminRemovedNotification(
		chatTitle,
		chatID,
		actionByName,
		actorID,
		now,
		botUsername,
	)

	adminRecipients := p.getAdminRecipients()
	for _, adminID := range adminRecipients {
		adminTarget := adminID
		go func(targetID int64) {
			_, err := p.appCtx.Client.SendMessage(targetID, notifText, &telegram.SendOptions{
				ParseMode: "html",
			})
			if err != nil && p.appCtx.Logger != nil {
				p.appCtx.Logger.Debug("admin bot removed DM failed", "admin_id", targetID, "error", err)
			}
		}(adminTarget)
	}
}

// HandleLeave processes the /leave command.
func (p *Plugin) HandleLeave(m *telegram.NewMessage) error {
	if m == nil || p.appCtx == nil || p.appCtx.Client == nil {
		return nil
	}

	args := strings.TrimSpace(m.Args())
	var targetChatID int64

	if args != "" {
		// Specific chat ID provided
		parsedID, err := strconv.ParseInt(strings.Fields(args)[0], 10, 64)
		if err != nil {
			_, replyErr := m.Reply("⚠️ <b>Invalid Chat ID:</b> Please specify a numerical group ID (e.g. <code>/leave -1001234567890</code>).", &telegram.SendOptions{ParseMode: "html"})
			return replyErr
		}
		targetChatID = parsedID
	} else {
		// No argument provided: must be executed inside a group
		if m.IsPrivate() {
			_, replyErr := m.Reply("⚠️ <b>Usage:</b> In private chat, specify the group ID: <code>/leave -100XXXXXXXXXX</code>\nOr execute <code>/leave</code> directly inside the target group.", &telegram.SendOptions{ParseMode: "html"})
			return replyErr
		}
		targetChatID = m.ChatID()
		_, _ = m.Reply("👋 <b>Goodbye!</b>\n\nI was instructed by an administrator to leave this group.", &telegram.SendOptions{ParseMode: "html"})
	}

	err := p.appCtx.Client.LeaveChannel(targetChatID)
	if err != nil {
		_, replyErr := m.Reply(fmt.Sprintf("⚠️ <b>Failed to leave chat:</b> <code>%s</code>", commands.EscapeHTML(err.Error())), &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}

	// Update DB and notify
	if p.appCtx.DB != nil && p.appCtx.DB.GroupRepo() != nil {
		_ = p.appCtx.DB.GroupRepo().DisableGroup(context.Background(), targetChatID, "Admin issued /leave")
	}

	_, replyErr := m.Reply(fmt.Sprintf("✅ <b>Successfully left chat:</b> <code>%d</code>", targetChatID), &telegram.SendOptions{ParseMode: "html"})
	return replyErr
}

// HandleDisable processes the /disable command.
func (p *Plugin) HandleDisable(m *telegram.NewMessage) error {
	if m == nil || p.appCtx == nil || p.appCtx.DB == nil || p.appCtx.DB.GroupRepo() == nil {
		return nil
	}

	args := strings.Fields(strings.TrimSpace(m.Args()))
	var targetChatID int64
	reason := "Disabled by administrator"

	if len(args) > 0 {
		if id, err := strconv.ParseInt(args[0], 10, 64); err == nil {
			targetChatID = id
			if len(args) > 1 {
				reason = strings.Join(args[1:], " ")
			}
		} else {
			// Argument is a reason text in current group
			if m.IsPrivate() {
				_, replyErr := m.Reply("⚠️ In private chat, please specify the group ID: <code>/disable &lt;chat_id&gt; [reason]</code>", &telegram.SendOptions{ParseMode: "html"})
				return replyErr
			}
			targetChatID = m.ChatID()
			reason = strings.Join(args, " ")
		}
	} else {
		if m.IsPrivate() {
			_, replyErr := m.Reply("⚠️ In private chat, please specify the group ID: <code>/disable &lt;chat_id&gt; [reason]</code>", &telegram.SendOptions{ParseMode: "html"})
			return replyErr
		}
		targetChatID = m.ChatID()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := p.appCtx.DB.GroupRepo().DisableGroup(ctx, targetChatID, reason)
	if err != nil {
		_, replyErr := m.Reply(fmt.Sprintf("⚠️ <b>Failed to disable group:</b> <code>%s</code>", commands.EscapeHTML(err.Error())), &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}

	respText := fmt.Sprintf(
		"🚫 <b>Group Disabled</b>\n\n"+
			"• <b>Group ID:</b> <code>%d</code>\n"+
			"• <b>Status:</b> Disabled\n"+
			"• <b>Reason:</b> %s\n\n"+
			"<i>Search queries and file deliveries are now paused for this group.</i>",
		targetChatID,
		commands.EscapeHTML(reason),
	)

	_, replyErr := m.Reply(respText, &telegram.SendOptions{ParseMode: "html"})
	return replyErr
}

// HandleEnable processes the /enable command.
func (p *Plugin) HandleEnable(m *telegram.NewMessage) error {
	if m == nil || p.appCtx == nil || p.appCtx.DB == nil || p.appCtx.DB.GroupRepo() == nil {
		return nil
	}

	args := strings.Fields(strings.TrimSpace(m.Args()))
	var targetChatID int64

	if len(args) > 0 {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			_, replyErr := m.Reply("⚠️ <b>Invalid Chat ID:</b> Please specify a numerical ID: <code>/enable &lt;chat_id&gt;</code>", &telegram.SendOptions{ParseMode: "html"})
			return replyErr
		}
		targetChatID = id
	} else {
		if m.IsPrivate() {
			_, replyErr := m.Reply("⚠️ In private chat, please specify the group ID: <code>/enable &lt;chat_id&gt;</code>", &telegram.SendOptions{ParseMode: "html"})
			return replyErr
		}
		targetChatID = m.ChatID()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := p.appCtx.DB.GroupRepo().EnableGroup(ctx, targetChatID)
	if err != nil {
		_, replyErr := m.Reply(fmt.Sprintf("⚠️ <b>Failed to enable group:</b> <code>%s</code>", commands.EscapeHTML(err.Error())), &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}

	respText := fmt.Sprintf(
		"✅ <b>Group Enabled</b>\n\n"+
			"• <b>Group ID:</b> <code>%d</code>\n"+
			"• <b>Status:</b> Active\n\n"+
			"<i>Search queries and file deliveries have been re-enabled for this group.</i>",
		targetChatID,
	)

	_, replyErr := m.Reply(respText, &telegram.SendOptions{ParseMode: "html"})
	return replyErr
}

// HandleGroups displays total connected group statistics.
func (p *Plugin) HandleGroups(m *telegram.NewMessage) error {
	if m == nil || p.appCtx == nil || p.appCtx.DB == nil || p.appCtx.DB.GroupRepo() == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	totalGroups, _ := p.appCtx.DB.GroupRepo().TotalGroupsCount(ctx)
	activeGroups, _ := p.appCtx.DB.GroupRepo().ActiveGroupsCount(ctx)
	disabledCount := totalGroups - activeGroups
	if disabledCount < 0 {
		disabledCount = 0
	}

	report := fmt.Sprintf(
		"📊 <b>Connected Groups Overview</b>\n\n"+
			"━━━━━━━━━━━━━━━━━━\n\n"+
			"• <b>Total Connected Groups:</b> <code>%d</code>\n"+
			"• <b>Active Groups:</b> <code>%d</code>\n"+
			"• <b>Disabled Groups:</b> <code>%d</code>\n\n"+
			"<i>Use /disable &lt;id&gt; to pause or /enable &lt;id&gt; to resume group services.</i>",
		totalGroups,
		activeGroups,
		disabledCount,
	)

	_, replyErr := m.Reply(report, &telegram.SendOptions{ParseMode: "html"})
	return replyErr
}

// FormatGroupWelcomeText formats the professional text-only group welcome banner.
func FormatGroupWelcomeText(botUsername string, groupName string, groupID int64, addedByName string, addedByID int64) string {
	escapedGroup := commands.EscapeHTML(groupName)
	escapedActor := commands.EscapeHTML(addedByName)
	cleanBotUsername := strings.TrimPrefix(botUsername, "@")

	var addedByInfo string
	if addedByID != 0 {
		addedByInfo = fmt.Sprintf("📝 <b>Name:</b> %s\n🆔 <b>ID:</b> <code>%d</code>", escapedActor, addedByID)
	} else {
		addedByInfo = fmt.Sprintf("📝 <b>Name:</b> %s", escapedActor)
	}

	return fmt.Sprintf(
		"🎉 <b>BOT SUCCESSFULLY ADDED!</b>\n\n"+
			"━━━━━━━━━━━━━━━━━━\n\n"+
			"👋 Hello, everyone!\n\n"+
			"Thanks for adding <b>@%s</b> to your group.\n\n"+
			"📌 <b>GROUP INFORMATION</b>\n\n"+
			"🏷 <b>Group:</b> %s\n"+
			"🆔 <b>Group ID:</b> <code>%d</code>\n\n"+
			"👤 <b>ADDED BY</b>\n\n"+
			"%s\n\n"+
			"━━━━━━━━━━━━━━━━━━\n\n"+
			"✨ Your bot is now ready to use!\n\n"+
			"Use the buttons below to explore more.",
		cleanBotUsername,
		escapedGroup,
		groupID,
		addedByInfo,
	)
}

// BuildGroupWelcomeKeyboard builds the interactive buttons for group welcome banners.
func BuildGroupWelcomeKeyboard(botUsername string, repoURL string) *telegram.ReplyInlineMarkup {
	cleanBotUsername := strings.TrimPrefix(botUsername, "@")
	openBotURL := fmt.Sprintf("https://t.me/%s?start=start", cleanBotUsername)
	if repoURL == "" {
		repoURL = "https://github.com/itsafxal/AutoFilter-Go"
	}

	return telegram.NewKeyboard().
		AddRow(
			telegram.Button.URL(fmt.Sprintf("🤖 @%s", cleanBotUsername), openBotURL),
		).
		AddRow(
			telegram.Button.URL("💻 Source Code", repoURL),
		).
		Build()
}

// FormatAdminNewGroupNotification formats the structured DM notification sent to administrators.
func FormatAdminNewGroupNotification(groupName string, groupID int64, groupLink string, addedByName string, addedByID int64, addedByUsername string, dateStr string, botUsername string) string {
	cleanBot := strings.TrimPrefix(botUsername, "@")
	escapedGroup := commands.EscapeHTML(groupName)
	escapedActor := commands.EscapeHTML(addedByName)

	return fmt.Sprintf(
		"📢 <b>#NEW_GROUP_ADDED</b>\n\n"+
			"━━━━━━━━━━━━━━━━━━\n\n"+
			"🏷️ <b>Group:</b> %s\n"+
			"🆔 <b>Group ID:</b> <code>%d</code>\n"+
			"🔗 <b>Group Link:</b> %s\n\n"+
			"👤 <b>ADDED BY:</b>\n"+
			"📝 <b>Name:</b> %s\n"+
			"🆔 <b>User ID:</b> <code>%d</code>\n"+
			"✨ <b>Username:</b> %s\n\n"+
			"📅 <b>Added On:</b> %s\n"+
			"🤖 <b>Bot:</b> @%s\n"+
			"━━━━━━━━━━━━━━━━━━",
		escapedGroup,
		groupID,
		groupLink,
		escapedActor,
		addedByID,
		addedByUsername,
		dateStr,
		cleanBot,
	)
}

// FormatAdminRemovedNotification formats the structured DM notice sent when bot is removed from a group.
func FormatAdminRemovedNotification(groupName string, groupID int64, actionByName string, actionByID int64, dateStr string, botUsername string) string {
	cleanBot := strings.TrimPrefix(botUsername, "@")
	escapedGroup := commands.EscapeHTML(groupName)
	escapedActor := commands.EscapeHTML(actionByName)

	var actorInfo string
	if actionByID != 0 {
		actorInfo = fmt.Sprintf("%s (<code>%d</code>)", escapedActor, actionByID)
	} else {
		actorInfo = escapedActor
	}

	return fmt.Sprintf(
		"🚪 <b>#BOT_REMOVED_FROM_GROUP</b>\n\n"+
			"━━━━━━━━━━━━━━━━━━\n\n"+
			"🏷️ <b>Group:</b> %s\n"+
			"🆔 <b>Group ID:</b> <code>%d</code>\n"+
			"👤 <b>Action By:</b> %s\n"+
			"📅 <b>Date:</b> %s\n"+
			"🤖 <b>Bot:</b> @%s\n"+
			"━━━━━━━━━━━━━━━━━━",
		escapedGroup,
		groupID,
		actorInfo,
		dateStr,
		cleanBot,
	)
}

// isDuplicateEvent prevents rapid repeated event processing for the same chat within 30 seconds.
func (p *Plugin) isDuplicateEvent(eventType string, chatID int64) bool {
	key := fmt.Sprintf("%s:%d", eventType, chatID)
	now := time.Now().Unix()

	if val, ok := p.dedupMap.Load(key); ok {
		if prevTimestamp, ok := val.(int64); ok && now-prevTimestamp < 30 {
			return true
		}
	}

	p.dedupMap.Store(key, now)
	return false
}

// getBotID safely resolves the bot's user ID.
func (p *Plugin) getBotID() int64 {
	if p.appCtx != nil && p.appCtx.BotUser != nil && p.appCtx.BotUser.ID != 0 {
		return p.appCtx.BotUser.ID
	}
	if p.appCtx != nil && p.appCtx.Client != nil {
		if me, err := p.appCtx.Client.GetMe(); err == nil && me != nil {
			return me.ID
		}
	}
	return 0
}

// getBotUsername safely resolves the bot's username without leading @.
func (p *Plugin) getBotUsername() string {
	if p.appCtx != nil && p.appCtx.BotUser != nil && p.appCtx.BotUser.Username != "" {
		return strings.TrimPrefix(p.appCtx.BotUser.Username, "@")
	}
	return "Bot"
}

// getAdminRecipients returns a deduplicated list of all owner and admin user IDs.
func (p *Plugin) getAdminRecipients() []int64 {
	if p.appCtx == nil || p.appCtx.Config == nil {
		return nil
	}

	seen := make(map[int64]bool)
	var list []int64

	if p.appCtx.Config.OwnerID != 0 {
		seen[p.appCtx.Config.OwnerID] = true
		list = append(list, p.appCtx.Config.OwnerID)
	}

	for _, adminID := range p.appCtx.Config.Admins {
		if adminID != 0 && !seen[adminID] {
			seen[adminID] = true
			list = append(list, adminID)
		}
	}

	return list
}
