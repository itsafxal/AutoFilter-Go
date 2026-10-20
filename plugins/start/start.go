package start

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/MYTGBOTZ/internal/commands"
	"github.com/itsafxal/MYTGBOTZ/internal/delivery"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
)

// Plugin implements the /start command plugin and interactive menu navigation.
type Plugin struct {
	appCtx *plugins.AppContext
}

// New creates a new start plugin instance.
func New(appCtx *plugins.AppContext) *Plugin {
	return &Plugin{appCtx: appCtx}
}

// Name returns the unique plugin name.
func (p *Plugin) Name() string {
	return "start"
}

// Commands returns the command definitions for the start plugin.
func (p *Plugin) Commands() []plugins.CommandDefinition {
	return []plugins.CommandDefinition{
		{
			Name:        "start",
			Description: "Start the bot and get welcome guide or process deep-link",
			Usage:       "/start [payload]",
			Category:    "General",
			Handler:     p.HandleStart,
		},
	}
}

// CallbackHandlers registers interactive callback query handlers for start menu navigation.
func (p *Plugin) CallbackHandlers() []plugins.CallbackDefinition {
	return []plugins.CallbackDefinition{
		{Prefix: "start_help", Handler: p.HandleHelpCallback},
		{Prefix: "start_about", Handler: p.HandleAboutCallback},
		{Prefix: "start_home", Handler: p.HandleHomeCallback},
		{Prefix: "help", Handler: p.HandleHelpCallback},
		{Prefix: "about", Handler: p.HandleAboutCallback},
		{Prefix: "close_data", Handler: p.HandleCloseCallback},
	}
}

func (p *Plugin) buildWelcomeContent(user *telegram.UserObj, botUsername string) (string, *telegram.ReplyInlineMarkup) {
	userMention := "there"
	if user != nil {
		if user.FirstName != "" {
			userMention = commands.EscapeHTML(user.FirstName)
		} else if user.Username != "" {
			userMention = "@" + commands.EscapeHTML(user.Username)
		}
	}

	welcomeText := fmt.Sprintf(
		"👋 <b>Hey there, %s!</b>\n\n"+
			"I'm your auto-filter bot – advanced, powerful, and designed to turn your groups into movie request groups!\n\n"+
			"Let's get started! 🚀",
		userMention,
	)

	replyMarkup := telegram.NewKeyboard().
		AddRow(telegram.Button.URL("➕ ᴀᴅᴅ ᴍᴇ ᴛᴏ ʏᴏᴜʀ ɢʀᴏᴜᴘ ➕", fmt.Sprintf("https://t.me/%s?startgroup=true", botUsername))).
		AddRow(
			telegram.Button.Data("ℹ️ ʜᴇʟᴘ", "start_help"),
			telegram.Button.Data("✨ ᴀʙᴏᴜᴛ", "start_about"),
		).
		Build()

	return welcomeText, replyMarkup
}

func (p *Plugin) buildHelpContent() (string, *telegram.ReplyInlineMarkup) {
	helpText := "📖 <b><u>Help & Available Commands</u></b>\n\n" +
		"Here are the commands you can use:\n\n" +
		"• /start - Start the bot & check status\n" +
		"• /ping - Check bot response speed & latency\n" +
		"• /info - View bot info & user profile\n" +
		"• /id - Get chat ID and user Telegram ID\n" +
		"• /latest - View recently added media files\n" +
		"• /request - Request a movie or series title\n\n" +
		"💡 <b>How to get movies:</b>\n" +
		"Simply send any movie or series title in PM or in any connected group, and I will search and deliver the files instantly!"

	replyMarkup := telegram.NewKeyboard().
		AddRow(
			telegram.Button.Data("✨ ᴀʙᴏᴜᴛ", "start_about"),
			telegram.Button.Data("🏠 ʜᴏᴍᴇ", "start_home"),
		).
		Build()

	return helpText, replyMarkup
}

func (p *Plugin) buildAboutContent() (string, *telegram.ReplyInlineMarkup) {
	botName := "AutoFilter Bot"
	botUsername := "bot"
	if p.appCtx != nil && p.appCtx.BotUser != nil {
		if p.appCtx.BotUser.FirstName != "" {
			botName = commands.EscapeHTML(p.appCtx.BotUser.FirstName)
		}
		if p.appCtx.BotUser.Username != "" {
			botUsername = commands.EscapeHTML(p.appCtx.BotUser.Username)
		}
	}

	ownerID := int64(0)
	if p.appCtx != nil && p.appCtx.Config != nil {
		ownerID = p.appCtx.Config.OwnerID
	}

	var startTime time.Time
	if p.appCtx != nil {
		startTime = p.appCtx.StartTime
	}
	uptime := commands.FormatUptime(startTime)
	goVer := runtime.Version()

	devLink := fmt.Sprintf("<a href=\"tg://user?id=%d\">Developer</a>", ownerID)
	if ownerID == 0 {
		devLink = "Bot Administrator"
	}

	aboutText := fmt.Sprintf(
		"🤖 <b><u>About This Bot</u></b>\n\n"+
			"• <b>My Name:</b> %s\n"+
			"• <b>Username:</b> @%s\n"+
			"• <b>Developer:</b> %s\n"+
			"• <b>Developer ID:</b> <code>%d</code>\n"+
			"• <b>Library:</b> <code>Gogram v1.7.71</code>\n"+
			"• <b>Language:</b> <code>Go %s</code>\n"+
			"• <b>Database:</b> <code>MongoDB & SQLite</code>\n"+
			"• <b>Bot Server:</b> <code>Telegram MTProto</code>\n"+
			"• <b>Uptime:</b> <code>%s</code>",
		botName,
		botUsername,
		devLink,
		ownerID,
		goVer,
		uptime,
	)

	replyMarkup := telegram.NewKeyboard().
		AddRow(
			telegram.Button.Data("ℹ️ ʜᴇʟᴘ", "start_help"),
			telegram.Button.Data("🏠 ʜᴏᴍᴇ", "start_home"),
		).
		Build()

	return aboutText, replyMarkup
}

// HandleHelpCallback renders the Help view in-place.
func (p *Plugin) HandleHelpCallback(cb *telegram.CallbackQuery) error {
	if cb == nil {
		return nil
	}
	text, markup := p.buildHelpContent()
	_, _ = cb.Edit(text, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: markup,
	})
	_, err := cb.Answer("")
	return err
}

// HandleAboutCallback renders the About view in-place.
func (p *Plugin) HandleAboutCallback(cb *telegram.CallbackQuery) error {
	if cb == nil {
		return nil
	}
	text, markup := p.buildAboutContent()
	_, _ = cb.Edit(text, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: markup,
	})
	_, err := cb.Answer("")
	return err
}

// HandleHomeCallback renders the Start welcome view in-place.
func (p *Plugin) HandleHomeCallback(cb *telegram.CallbackQuery) error {
	if cb == nil {
		return nil
	}
	botUsername := "bot"
	if p.appCtx != nil && p.appCtx.BotUser != nil && p.appCtx.BotUser.Username != "" {
		botUsername = p.appCtx.BotUser.Username
	}
	text, markup := p.buildWelcomeContent(cb.Sender, botUsername)
	_, _ = cb.Edit(text, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: markup,
	})
	_, err := cb.Answer("")
	return err
}

// HandleCloseCallback deletes the menu message.
func (p *Plugin) HandleCloseCallback(cb *telegram.CallbackQuery) error {
	if cb != nil {
		_, _ = cb.Delete()
		_, _ = cb.Answer("")
	}
	return nil
}

// HandleStart handles the /start command in private and group chats.
func (p *Plugin) HandleStart(m *telegram.NewMessage) error {
	botUsername := "bot"
	if p.appCtx != nil && p.appCtx.BotUser != nil && p.appCtx.BotUser.Username != "" {
		botUsername = p.appCtx.BotUser.Username
	}

	// Ignore broadcast channels completely
	if m.IsChannel() || m.IsChannelPost() {
		return nil
	}

	// Handle Group and Supergroup chats
	if m.IsGroup() {
		chatTitle := "Chat"
		if m.Channel != nil && m.Channel.Title != "" {
			chatTitle = m.Channel.Title
		} else if m.Chat != nil && m.Chat.Title != "" {
			chatTitle = m.Chat.Title
		}
		if p.appCtx != nil && p.appCtx.DB != nil && p.appCtx.DB.GroupRepo() != nil {
			_ = p.appCtx.DB.GroupRepo().AddGroup(context.Background(), m.ChatID(), chatTitle)
		}
		escapedTitle := commands.EscapeHTML(chatTitle)
		text := fmt.Sprintf(
			"👋 <b>Hey there!</b>\n\n"+
				"I'm ready to serve <b>%s</b>!\n\n"+
				"Use /cmds to view available commands.",
			escapedTitle,
		)

		replyMarkup := telegram.NewKeyboard().
			AddRow(telegram.Button.URL("➜ ᴀᴅᴅ ᴍᴇ ᴛᴏ ʏᴏᴜʀ ɢʀᴏᴜᴘ ➜", fmt.Sprintf("https://t.me/%s?startgroup=true", botUsername))).
			AddRow(telegram.Button.URL("✜ ᴄᴏᴍᴍᴀɴᴅs", fmt.Sprintf("https://t.me/%s?start=help", botUsername))).
			Build()

		_, err := m.Reply(text, &telegram.SendOptions{
			ParseMode:   "html",
			ReplyMarkup: replyMarkup,
		})
		return err
	}

	// Handle Private chats
	user := m.Sender
	if user != nil && p.appCtx != nil && p.appCtx.DB != nil && p.appCtx.DB.UserRepo() != nil {
		userName := user.FirstName
		if user.LastName != "" {
			userName += " " + user.LastName
		}
		_ = p.appCtx.DB.UserRepo().AddUser(context.Background(), user.ID, userName)
	}

	args := strings.Fields(m.Text())
	payloadRes := commands.ValidateStartPayload(args)

	if payloadRes.HasPayload {
		if !payloadRes.IsValid {
			_, err := m.Reply(fmt.Sprintf("⚠️ <b>Invalid Start Payload:</b> %s", commands.EscapeHTML(payloadRes.ErrorReason)), &telegram.SendOptions{
				ParseMode: "html",
			})
			return err
		}

		payload := payloadRes.Payload

		// 1. Help/commands deep link
		if payload == "help" {
			text, markup := p.buildHelpContent()
			_, err := m.Reply(text, &telegram.SendOptions{
				ParseMode:   "html",
				ReplyMarkup: markup,
			})
			return err
		}

		// 2. About deep link
		if payload == "about" {
			text, markup := p.buildAboutContent()
			_, err := m.Reply(text, &telegram.SendOptions{
				ParseMode:   "html",
				ReplyMarkup: markup,
			})
			return err
		}

		// 3. Media delivery deep link: file_<id>, filep_<id>, or direct file ID
		if p.appCtx != nil && p.appCtx.Delivery != nil {
			var fileID string
			protectContent := false

			if strings.HasPrefix(payload, "filep_") {
				fileID = strings.TrimPrefix(payload, "filep_")
				protectContent = true
			} else if strings.HasPrefix(payload, "file_") {
				fileID = strings.TrimPrefix(payload, "file_")
			} else if payload != "subscribe" && payload != "error" && payload != "okay" {
				fileID = payload
			}

			if fileID != "" {
				var userID int64
				if user != nil {
					userID = user.ID
				} else {
					userID = m.SenderID()
				}

				// Force Subscribe Verification Flow matching legacy TGxCyberBot
				if p.appCtx != nil && p.appCtx.FSub != nil {
					isAdmin := false
					if p.appCtx.Config != nil {
						isAdmin = p.appCtx.Config.IsAdmin(userID) || p.appCtx.Config.IsOwner(userID)
					}

					if !isAdmin {
						markup, unjoined := p.appCtx.FSub.BuildDynamicFSubKeyboard(context.Background(), p.appCtx.Client, userID, payload)
						if len(unjoined) > 0 && markup != nil {
							promptMsg, err := m.Reply("<b>Pʟᴇᴀꜱᴇ Jᴏɪɴ Oᴜʀ Cʜᴀɴɴᴇʟs Tᴏ Gᴇᴛ Yᴏᴜʀ Rᴇǫᴜᴇꜱᴛᴇᴅ Mᴏᴠɪᴇ Fɪʟᴇ 🚀</b>", &telegram.SendOptions{
								ParseMode:   "html",
								ReplyMarkup: markup,
							})
							if err != nil {
								return err
							}

							var promptMsgID int32
							if promptMsg != nil {
								promptMsgID = promptMsg.ID
							}

							// Seamless refresh background loop: dynamically updates markup & auto-delivers once joined without needing a Try Again button
							checkSuccess := p.appCtx.FSub.CheckLoopSubDynamic(context.Background(), p.appCtx.Client, userID, m.ChatID(), promptMsgID, payload)
							if checkSuccess {
								if p.appCtx.Client != nil && promptMsgID != 0 {
									_, _ = p.appCtx.Client.DeleteMessages(m.ChatID(), []int32{promptMsgID})
								}
							} else {
								// User hasn't joined within the verification window; stop delivery
								return nil
							}
						}
					}
				}

				opts := delivery.DeliveryOptions{
					UserID:         userID,
					ProtectContent: protectContent,
				}

				res, err := p.appCtx.Delivery.Deliver(context.Background(), m.ChatID(), fileID, opts)
				if err == nil {
					if res != nil && res.MessageID != 0 && p.appCtx != nil && p.appCtx.AutoDelete != nil {
						_ = p.appCtx.AutoDelete.ScheduleFileDelete(m.ChatID(), int32(res.MessageID))
					}
					return nil
				}

				// Safe user-friendly error messages without internal data exposure
				if errors.Is(err, delivery.ErrMediaNotFound) {
					_, replyErr := m.Reply("⚠️ <b>File not found or has been removed.</b>", &telegram.SendOptions{ParseMode: "html"})
					return replyErr
				}
				if errors.Is(err, delivery.ErrUserBanned) {
					_, replyErr := m.Reply("⛔ <b>Access Denied:</b> Your account is suspended from receiving files.", &telegram.SendOptions{ParseMode: "html"})
					return replyErr
				}
				if errors.Is(err, delivery.ErrChatDisabled) {
					_, replyErr := m.Reply("⛔ <b>Access Denied:</b> This chat is disabled for media delivery.", &telegram.SendOptions{ParseMode: "html"})
					return replyErr
				}
				if errors.Is(err, delivery.ErrDuplicateRequest) {
					_, replyErr := m.Reply("⏳ <b>Please wait:</b> A delivery request for this file is already in progress.", &telegram.SendOptions{ParseMode: "html"})
					return replyErr
				}
				if errors.Is(err, delivery.ErrRateLimited) {
					_, replyErr := m.Reply("⚠️ <b>Flood Control:</b> Telegram rate limit reached. Please wait a moment before trying again.", &telegram.SendOptions{ParseMode: "html"})
					return replyErr
				}
				if errors.Is(err, delivery.ErrBlockedByUser) {
					return nil
				}

				_, replyErr := m.Reply("❌ <b>Delivery Error:</b> Failed to deliver requested file. Please try again later.", &telegram.SendOptions{ParseMode: "html"})
				return replyErr
			}
		}

		// Fallback for non-file payload
		text := fmt.Sprintf(
			"📦 <b>Deep-Link Payload Received:</b> <code>%s</code>\n\n"+
				"ℹ️ <i>Use /cmds to view available commands.</i>",
			commands.EscapeHTML(payloadRes.Payload),
		)
		_, err := m.Reply(text, &telegram.SendOptions{
			ParseMode: "html",
		})
		return err
	}

	// Standard private welcome with 3 buttons: Add Me To Group, Help, About
	welcomeText, replyMarkup := p.buildWelcomeContent(user, botUsername)

	sentMsg, err := m.Reply(welcomeText, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: replyMarkup,
	})
	if err == nil && sentMsg != nil && p.appCtx != nil && p.appCtx.AutoDelete != nil {
		_ = p.appCtx.AutoDelete.TrackSentMessage(sentMsg.ChatID(), sentMsg.ID, "start")
	}
	return err
}

