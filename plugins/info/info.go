package info

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/MYTGBOTZ/internal/commands"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
)

// Plugin implements the /info and /about command plugin.
type Plugin struct {
	appCtx *plugins.AppContext
}

// New creates a new info plugin instance.
func New(appCtx *plugins.AppContext) *Plugin {
	return &Plugin{appCtx: appCtx}
}

// Name returns the unique plugin name.
func (p *Plugin) Name() string {
	return "info"
}

// Commands returns the command definitions for info plugin.
func (p *Plugin) Commands() []plugins.CommandDefinition {
	return []plugins.CommandDefinition{
		{
			Name:        "info",
			Aliases:     []string{"about"},
			Description: "Display available bot information and user profile details",
			Usage:       "/info",
			Category:    "General",
			Handler:     p.HandleInfo,
		},
	}
}

// BuildBotInfoReport builds a safe bot overview without exposing secrets or fake stats.
func BuildBotInfoReport(botUser *telegram.UserObj, startTime time.Time) string {
	botName := "Bot"
	botUsername := "N/A"
	botID := int64(0)
	if botUser != nil {
		botName = commands.EscapeHTML(botUser.FirstName)
		if botUser.Username != "" {
			botUsername = "@" + commands.EscapeHTML(botUser.Username)
		}
		botID = botUser.ID
	}

	uptime := commands.FormatUptime(startTime)
	goVer := runtime.Version()

	return fmt.Sprintf(
		"🤖 <b>Bot Information</b>\n\n"+
			"────────────────────\n"+
			"📛 <b>Bot Name:</b> %s\n"+
			"✨ <b>Username:</b> %s\n"+
			"🆔 <b>Bot ID:</b> <code>%d</code>\n"+
			"📚 <b>Library:</b> <code>Gogram v1.7.71</code>\n"+
			"💻 <b>Language:</b> <code>%s</code>\n"+
			"🗄️ <b>Database:</b> <code>MongoDB & SQLite</code>\n"+
			"🖥️ <b>Bot Server:</b> <code>Telegram MTProto</code>\n"+
			"⏱️ <b>Uptime:</b> <code>%s</code>\n"+
			"────────────────────",
		botName,
		botUsername,
		botID,
		goVer,
		uptime,
	)

}

// BuildUserInfoReport builds safe user profile details.
func BuildUserInfoReport(user *telegram.UserObj) string {
	if user == nil || user.ID == 0 {
		return "⚠️ <b>User Profile Details:</b> <i>User information not available.</i>"
	}

	firstName := commands.EscapeHTML(user.FirstName)
	lastName := "<i>Not Set</i>"
	if user.LastName != "" {
		lastName = commands.EscapeHTML(user.LastName)
	}

	username := "<i>Not Set</i>"
	if user.Username != "" {
		username = "@" + commands.EscapeHTML(user.Username)
	}

	return fmt.Sprintf(
		"✨ <b>User Profile Details</b> ✨\n\n"+
			"────────────────────\n"+
			"📝 <b>First Name:</b> %s\n"+
			"📝 <b>Last Name:</b> %s\n"+
			"🆔 <b>Telegram ID:</b> <code>%d</code>\n"+
			"✨ <b>Username:</b> %s\n"+
			"🔗 <b>User Link:</b> <a href=\"tg://user?id=%d\">Click Here</a>\n"+
			"────────────────────",
		firstName,
		lastName,
		user.ID,
		username,
		user.ID,
	)
}

// HandleInfo handles the /info and /about command.
func (p *Plugin) HandleInfo(m *telegram.NewMessage) error {
	// If replying to a user's message, display target user info
	if m.IsReply() {
		reply, err := m.GetReplyMessage()
		if err == nil && reply != nil && reply.Sender != nil {
			userReport := BuildUserInfoReport(reply.Sender)
			_, err := m.Reply(userReport, &telegram.SendOptions{
				ParseMode: "html",
			})
			return err
		}
	}

	var startTime time.Time
	var botUser *telegram.UserObj
	if p.appCtx != nil {
		startTime = p.appCtx.StartTime
		botUser = p.appCtx.BotUser
	}

	// If command is /about, render the custom about card
	if m != nil && strings.HasPrefix(strings.ToLower(strings.TrimSpace(m.Text())), "/about") {
		botName := "AutoFilter Bot"
		botUsername := "bot"
		if botUser != nil {
			if botUser.FirstName != "" {
				botName = commands.EscapeHTML(botUser.FirstName)
			}
			if botUser.Username != "" {
				botUsername = strings.TrimPrefix(commands.EscapeHTML(botUser.Username), "@")
			}
		}

		tgSettings := "tg://" + "settings"
		aboutText := fmt.Sprintf(
			"<blockquote><b>\n"+
				"‣ ᴍʏ ɴᴀᴍᴇ : <a href='https://t.me/%s'>%s</a>\n"+
				"‣ ᴄʀᴇᴀᴛᴏʀ : <a href='%s'>ᴛʜɪs ᴘᴇʀsᴏɴ</a>\n"+
				"‣ ᴅᴇᴠᴇʟᴏᴘᴇʀ : <a href='https://cybertg.vercel.app'>ᴄʏʙᴇʀ ᠰ ᴛɢ</a>\n"+
				"‣ ʟɪʙʀᴀʀʏ : <a href='https://www.gogram.sbs/'>ᴘʏʀᴏɢʀᴀᴍ</a>\n"+
				"‣ ʟᴀɴɢᴜᴀɢᴇ : <a href='https://go.dev/'>Go </a>\n"+
				"‣ ᴅᴀᴛᴀ ʙᴀsᴇ : <a href='https://www.mongodb.com/'>ᴍᴏɴɢᴏ ᴅʙ</a>\n"+
				"‣ ʙᴏᴛ sᴇʀᴠᴇʀ : <a href='https://t.me/quickfastt'>ǫᴜɪᴄᴋ ꜰᴀsᴛ</a></b></blockquote>",
			botUsername,
			botName,
			tgSettings,
		)

		replyMarkup := telegram.NewKeyboard().
			AddRow(
				telegram.Button.Data("ℹ️ ʜᴇʟᴘ", "start_help"),
				telegram.Button.Data("🏠 ʜᴏᴍᴇ", "start_home"),
			).
			Build()

		_, err := m.Reply(aboutText, &telegram.SendOptions{
			ParseMode:   "html",
			ReplyMarkup: replyMarkup,
		})
		return err
	}

	botReport := BuildBotInfoReport(botUser, startTime)
	var sb strings.Builder
	sb.WriteString(botReport)

	// If in private chat, append requesting user details
	if m.IsPrivate() && m.Sender != nil && m.Sender.ID != 0 {
		sb.WriteString("\n\n")
		sb.WriteString(BuildUserInfoReport(m.Sender))
	}

	_, err := m.Reply(sb.String(), &telegram.SendOptions{
		ParseMode: "html",
	})
	return err
}
