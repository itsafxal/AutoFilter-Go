package info

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/AutoFilter-Go/internal/commands"
)

// UserCardData represents resolved and formatted data for a user info card.
type UserCardData struct {
	User           *telegram.UserObj
	ProfilePhotoDC int32
	BotSessionDC   int
	ChatID         int64
	ChatTitle      string
	IsGroup        bool
	GroupStatus    string
	GroupJoinDate  int32
	RepliedMsgID   int32
}

// ChannelCardData represents resolved data for a channel or anonymous admin info card.
type ChannelCardData struct {
	Channel      *telegram.Channel
	Chat         *telegram.ChatObj
	CurrentChat  int64
	CurrentTitle string
	RepliedMsgID int32
}

// FormatUserInfoCard generates the modern Telegram-style HTML information card for a user.
func FormatUserInfoCard(data *UserCardData) string {
	if data == nil || data.User == nil {
		return "⚠️ <b>User Information:</b> <i>Target user not found or unavailable.</i>"
	}

	u := data.User

	// Check for deleted account
	if u.Deleted {
		var sb strings.Builder
		sb.WriteString("👤 <b>USER INFORMATION</b>\n\n")
		sb.WriteString("━━━━━━━━━━━━━━━━━━\n\n")
		sb.WriteString("📝 <b>Name:</b> <i>Deleted Account</i>\n")
		sb.WriteString(fmt.Sprintf("🆔 <b>User ID:</b> <code>%d</code>\n\n", u.ID))
		sb.WriteString("━━━━━━━━━━━━━━━━━━\n\n")
		sb.WriteString("📊 <b>ACCOUNT DETAILS</b>\n\n")
		sb.WriteString("⚠️ <b>Status:</b> Deleted Account\n")

		if data.IsGroup {
			appendGroupDetails(&sb, data)
		}
		return sb.String()
	}

	// 1. User Information section
	fullName := strings.TrimSpace(u.FirstName + " " + u.LastName)
	if fullName == "" {
		fullName = "Unknown"
	}
	escapedName := commands.EscapeHTML(fullName)

	usernameStr := "<i>Not Set</i>"
	if u.Username != "" {
		usernameStr = "@" + commands.EscapeHTML(u.Username)
	}

	profileLink := fmt.Sprintf("<a href=\"tg://user?id=%d\">Open Profile</a>", u.ID)
	if u.Username != "" {
		profileLink = fmt.Sprintf("<a href=\"https://t.me/%s\">Open Profile</a>", commands.EscapeHTML(u.Username))
	}

	var sb strings.Builder
	sb.WriteString("👤 <b>USER INFORMATION</b>\n\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━\n\n")
	sb.WriteString(fmt.Sprintf("📝 <b>Name:</b> %s\n", escapedName))
	sb.WriteString(fmt.Sprintf("✨ <b>Username:</b> %s\n", usernameStr))
	sb.WriteString(fmt.Sprintf("🆔 <b>User ID:</b> <code>%d</code>\n", u.ID))
	sb.WriteString(fmt.Sprintf("🔗 <b>Profile:</b> %s\n\n", profileLink))

	// 2. Account Details section
	sb.WriteString("━━━━━━━━━━━━━━━━━━\n\n")
	sb.WriteString("📊 <b>ACCOUNT DETAILS</b>\n\n")
	sb.WriteString(fmt.Sprintf("🤖 <b>Bot:</b> %s\n", boolToYesNo(u.Bot)))
	sb.WriteString(fmt.Sprintf("⭐ <b>Premium:</b> %s\n", boolToYesNo(u.Premium)))
	sb.WriteString(fmt.Sprintf("☑️ <b>Verified:</b> %s\n", boolToYesNo(u.Verified)))
	sb.WriteString(fmt.Sprintf("🚫 <b>Scam:</b> %s\n", boolToYesNo(u.Scam)))
	sb.WriteString(fmt.Sprintf("👁 <b>Fake:</b> %s\n", boolToYesNo(u.Fake)))

	if u.Restricted {
		sb.WriteString("⚠️ <b>Restricted:</b> Yes\n")
	}

	langCode := "Not Available"
	if u.LangCode != "" {
		langCode = fmt.Sprintf("<code>%s</code>", commands.EscapeHTML(u.LangCode))
	}
	sb.WriteString(fmt.Sprintf("🌐 <b>Language:</b> %s\n\n", langCode))

	// 3. Activity section
	lastSeen := formatLastSeen(u.Status)
	sb.WriteString("━━━━━━━━━━━━━━━━━━\n\n")
	sb.WriteString("🟢 <b>ACTIVITY</b>\n\n")
	sb.WriteString(fmt.Sprintf("📡 <b>Last Seen:</b> %s\n", lastSeen))

	if data.IsGroup && data.GroupJoinDate > 0 {
		joinDateStr := time.Unix(int64(data.GroupJoinDate), 0).UTC().Format("02 Jan 2006, 15:04 UTC")
		sb.WriteString(fmt.Sprintf("📅 <b>Group Joined:</b> %s\n", joinDateStr))
	}

	// 4. Group Context (if command was executed in a group/supergroup)
	if data.IsGroup {
		appendGroupDetails(&sb, data)
	}

	// 5. Network Details section (only when reliably available)
	var networkDetails []string
	if data.ProfilePhotoDC > 0 {
		networkDetails = append(networkDetails, fmt.Sprintf("📍 <b>Profile Photo DC:</b> DC %d", data.ProfilePhotoDC))
	}
	if data.BotSessionDC > 0 {
		networkDetails = append(networkDetails, fmt.Sprintf("📍 <b>Bot Session DC:</b> DC %d", data.BotSessionDC))
	}

	if len(networkDetails) > 0 {
		sb.WriteString("\n━━━━━━━━━━━━━━━━━━\n\n")
		sb.WriteString("🌐 <b>NETWORK DETAILS</b>\n\n")
		for _, nd := range networkDetails {
			sb.WriteString(nd + "\n")
		}
	}

	return sb.String()
}

// FormatChannelCard generates a clean information card when target is a channel or anonymous sender chat.
func FormatChannelCard(data *ChannelCardData) string {
	if data == nil {
		return "⚠️ <i>Channel information unavailable.</i>"
	}

	var sb strings.Builder
	sb.WriteString("📢 <b>CHANNEL / SENDER CHAT</b>\n\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━\n\n")

	title := "Unknown"
	username := "<i>Not Set</i>"
	var chatID int64
	link := "<i>Private</i>"

	if data.Channel != nil {
		title = commands.EscapeHTML(data.Channel.Title)
		chatID = data.Channel.ID
		if !strings.HasPrefix(strconv.FormatInt(chatID, 10), "-100") && chatID > 0 {
			chatID = -1000000000000 - chatID // ensure standard Telegram supergroup/channel ID format
		}
		if data.Channel.Username != "" {
			username = "@" + commands.EscapeHTML(data.Channel.Username)
			link = fmt.Sprintf("<a href=\"https://t.me/%s\">Open Channel</a>", commands.EscapeHTML(data.Channel.Username))
		}
	} else if data.Chat != nil {
		title = commands.EscapeHTML(data.Chat.Title)
		chatID = data.Chat.ID
		if chatID > 0 {
			chatID = -chatID
		}
	}

	sb.WriteString(fmt.Sprintf("🏷️ <b>Title:</b> %s\n", title))
	sb.WriteString(fmt.Sprintf("✨ <b>Username:</b> %s\n", username))
	sb.WriteString(fmt.Sprintf("🆔 <b>Chat ID:</b> <code>%d</code>\n", chatID))
	sb.WriteString(fmt.Sprintf("🔗 <b>Link:</b> %s\n\n", link))

	if data.CurrentChat != 0 {
		sb.WriteString("━━━━━━━━━━━━━━━━━━\n\n")
		sb.WriteString("👥 <b>GROUP CONTEXT</b>\n\n")
		sb.WriteString(fmt.Sprintf("💬 <b>Group ID:</b> <code>%d</code>\n", data.CurrentChat))
		if data.CurrentTitle != "" {
			sb.WriteString(fmt.Sprintf("🏷️ <b>Group Title:</b> %s\n", commands.EscapeHTML(data.CurrentTitle)))
		}
		if data.RepliedMsgID > 0 {
			sb.WriteString(fmt.Sprintf("📨 <b>Replied Message ID:</b> <code>%d</code>\n", data.RepliedMsgID))
		}
	}

	return sb.String()
}

// appendGroupDetails writes group context rows into the string builder.
func appendGroupDetails(sb *strings.Builder, data *UserCardData) {
	sb.WriteString("\n━━━━━━━━━━━━━━━━━━\n\n")
	sb.WriteString("👥 <b>GROUP CONTEXT</b>\n\n")
	sb.WriteString(fmt.Sprintf("💬 <b>Group ID:</b> <code>%d</code>\n", data.ChatID))
	if data.ChatTitle != "" {
		sb.WriteString(fmt.Sprintf("🏷️ <b>Group Title:</b> %s\n", commands.EscapeHTML(data.ChatTitle)))
	}
	if data.GroupStatus != "" {
		sb.WriteString(fmt.Sprintf("🎖️ <b>Membership Status:</b> %s\n", commands.EscapeHTML(data.GroupStatus)))
	}
	if data.RepliedMsgID > 0 {
		sb.WriteString(fmt.Sprintf("📨 <b>Replied Message ID:</b> <code>%d</code>\n", data.RepliedMsgID))
	}
}

// formatLastSeen turns a Telegram UserStatus object into a human-readable string.
func formatLastSeen(status telegram.UserStatus) string {
	if status == nil {
		return "Not Available"
	}

	switch s := status.(type) {
	case *telegram.UserStatusOnline:
		return "Online"
	case *telegram.UserStatusOffline:
		if s.WasOnline > 0 {
			return time.Unix(int64(s.WasOnline), 0).UTC().Format("02 Jan 2006, 15:04 UTC")
		}
		return "Offline"
	case *telegram.UserStatusRecently:
		return "Recently"
	case *telegram.UserStatusLastWeek:
		return "Within a week"
	case *telegram.UserStatusLastMonth:
		return "Within a month"
	case *telegram.UserStatusEmpty:
		return "Hidden / Unknown"
	default:
		return "Not Available"
	}
}

// boolToYesNo converts boolean values to Yes / No string representations.
func boolToYesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

// BuildUserKeyboard creates inline buttons: Open Profile, Copy User ID, and Close.
func BuildUserKeyboard(userID int64, username string, requesterID int64) *telegram.ReplyInlineMarkup {
	profileURL := fmt.Sprintf("tg://user?id=%d", userID)
	if username != "" {
		profileURL = fmt.Sprintf("https://t.me/%s", strings.TrimPrefix(username, "@"))
	}

	return telegram.NewKeyboard().
		AddRow(
			telegram.Button.URL("🔗 Open Profile", profileURL),
			telegram.Button.Copy("📋 Copy ID", fmt.Sprintf("%d", userID)),
		).
		AddRow(
			telegram.Button.Data("❌ Close", fmt.Sprintf("info_close:%d", requesterID)),
		).
		Build()
}

// BuildChannelKeyboard creates inline buttons for channel or sender chat cards.
func BuildChannelKeyboard(chatID int64, username string, requesterID int64) *telegram.ReplyInlineMarkup {
	kb := telegram.NewKeyboard()

	if username != "" {
		kb.AddRow(
			telegram.Button.URL("🔗 Open Channel", fmt.Sprintf("https://t.me/%s", strings.TrimPrefix(username, "@"))),
			telegram.Button.Copy("📋 Copy ID", fmt.Sprintf("%d", chatID)),
		)
	} else {
		kb.AddRow(
			telegram.Button.Copy("📋 Copy ID", fmt.Sprintf("%d", chatID)),
		)
	}

	kb.AddRow(
		telegram.Button.Data("❌ Close", fmt.Sprintf("info_close:%d", requesterID)),
	)
	return kb.Build()
}

// ExtractProfilePhotoDC reliably extracts the DC ID from a UserProfilePhoto or UserPhoto object.
func ExtractProfilePhotoDC(photo any) int32 {
	if photo == nil {
		return 0
	}
	switch p := photo.(type) {
	case *telegram.UserPhoto:
		return p.DcID()
	case *telegram.UserProfilePhotoObj:
		return p.DcID
	case *telegram.PhotoObj:
		return p.DcID
	default:
		return 0
	}
}

// ParseGroupMemberStatus parses participant info into human-readable status, rank, and join date.
func ParseGroupMemberStatus(participant *telegram.Participant) (status string, joinDate int32) {
	if participant == nil {
		return "Member", 0
	}

	rawStatus := strings.ToLower(participant.Status)
	switch rawStatus {
	case "creator":
		status = "Owner / Creator"
	case "admin":
		status = "Administrator"
	case "restricted":
		status = "Restricted"
	case "banned", "kicked":
		status = "Banned"
	case "left":
		status = "Left Chat"
	default:
		status = "Member"
	}

	if participant.Rank != "" {
		status = fmt.Sprintf("%s (%s)", status, participant.Rank)
	}

	if participant.Participant != nil {
		switch cp := participant.Participant.(type) {
		case *telegram.ChannelParticipantObj:
			joinDate = cp.Date
		case *telegram.ChannelParticipantAdmin:
			joinDate = cp.Date
		case *telegram.ChannelParticipantBanned:
			joinDate = cp.Date
		case *telegram.ChannelParticipantSelf:
			joinDate = cp.Date
		}
	}

	return status, joinDate
}
