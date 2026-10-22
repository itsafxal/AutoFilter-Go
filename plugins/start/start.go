package start

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/MYTGBOTZ/internal/commands"
	"github.com/itsafxal/MYTGBOTZ/internal/delivery"
	"github.com/itsafxal/MYTGBOTZ/internal/oldbot"
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
	userMention := "friend"
	if user != nil {
		if user.FirstName != "" {
			userMention = fmt.Sprintf("<a href=\"tg://user?id=%d\">%s</a>", user.ID, commands.EscapeHTML(user.FirstName))
		} else if user.Username != "" {
			userMention = "@" + commands.EscapeHTML(user.Username)
		}
	}

	welcomeText := fmt.Sprintf(
		"👋 𝙃𝙚𝙮 𝙩𝙝𝙚𝙧𝙚, %s!\n\n"+
			"𝙄'𝙢 𝙮𝙤𝙪𝙧 𝙖𝙪𝙩𝙤-𝙛𝙞𝙡𝙩𝙚𝙧 𝙗𝙤𝙩 – 𝙖𝙙𝙫𝙖𝙣𝙘𝙚𝙙, 𝙥𝙤𝙬𝙚𝙧𝙛𝙪𝙡, 𝙖𝙣𝙙 𝙙𝙚𝙨𝙞𝙜𝙣𝙚𝙙 𝙩𝙤 𝙢𝙖𝙠𝙚 𝙮𝙤𝙪𝙧 𝙜𝙧𝙤𝙪𝙥𝙨 𝙩𝙤 𝙢𝙤𝙫𝙞𝙚 𝙧𝙚𝙦𝙪𝙚𝙨𝙩 𝙜𝙧𝙤𝙪𝙥!\n\n"+
			"𝙇𝙚𝙩'𝙨 𝙜𝙚𝙩 𝙨𝙩𝙖𝙧𝙩𝙚𝙙! 🚀",
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
	helpText := "<b>Iᴛ ɪs ᴇᴀsʏ ᴛᴏ ᴜsᴇ ᴍᴇ Jᴜsᴛ ᴀᴅᴅ ᴍᴇ ᴛᴏ ʏᴏᴜʀ ɢʀᴏᴜᴘ ᴏʀ Sᴇᴀʀᴄʜ ʏᴏᴜʀ ᴍᴏᴠɪᴇs ᴏɴ ʙᴏᴛ ᴘᴍ (Hᴇʀᴇ) 😉\n\n" +
		"<blockquote><u>Sᴏᴍᴇ Oғ Mʏ  Cᴏᴍᴍᴀɴᴅs</u>\n" +
		"‣/latest - Gᴇᴛ ʟɪsᴛ ᴏғ ʟᴀᴛᴇsᴛ ᴍᴏᴠɪᴇs ʙʏ ʟᴀɴɢᴜᴀɢᴇ\n" +
		"‣/request &lt;movie name&gt; - Rᴇᴏ̨ᴜᴇsᴛ ᴀ ᴍᴏᴠɪᴇ ғʀᴏᴍ ᴀᴅᴍɪɴs\n" +
		"‣/id - Gᴇᴛ ʏᴏᴜʀ Tᴇʟᴇɢʀᴀᴍ ID ᴏʀ ᴍᴇssᴀɢᴇ/ғɪʟᴇ ID\n" +
		"‣/info - Fᴇᴛᴄʜ ᴅᴇᴛᴀɪʟᴇᴅ ɪɴғᴏ ᴏғ ᴀ ᴜsᴇʀ (ʀᴇᴘʟʏ ᴏʀ @ᴜsᴇʀɴᴀᴍᴇ)\n" +
		"‣/ping - Cʜᴇᴄᴋ Sᴇʀᴠᴇʀ ᴘɪɴɢ\n" +
		"‣/broadcast - Rᴇᴘʟʏ ᴛᴏ ᴀ ᴍᴇssᴀɢᴇ ᴛᴏ sᴇɴᴅ ᴛʜᴀᴛ ᴛᴏ ᴀʟʟ ʙᴏᴛ ᴜsᴇʀs (ᴀᴅᴍɪɴ ᴏɴʟʏ)</blockquote></b>"

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
			botUsername = strings.TrimPrefix(commands.EscapeHTML(p.appCtx.BotUser.Username), "@")
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

func (p *Plugin) isMaintenanceMode() bool {
	return p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.MaintenanceMode
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
		if p.isMaintenanceMode() {
			var userID int64
			if m.Sender != nil && m.Sender.ID != 0 {
				userID = m.Sender.ID
			} else {
				userID = m.SenderID()
			}
			if userID == 0 {
				userID = m.ChatID()
			}
			isAdmin := p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.IsAdmin(userID)
			if !isAdmin {
				return nil
			}
		}

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
	if (user == nil || user.ID == 0) && m.Client != nil {
		if sender, err := m.GetSender(); err == nil && sender != nil && sender.ID != 0 {
			user = sender
			m.Sender = sender
		}
	}
	var privateUserID int64
	if user != nil && user.ID != 0 {
		privateUserID = user.ID
	} else {
		privateUserID = m.SenderID()
		if privateUserID == 0 {
			privateUserID = m.ChatID()
		}
	}

	if privateUserID != 0 && p.appCtx != nil && p.appCtx.DB != nil && p.appCtx.DB.UserRepo() != nil {
		userName := ""
		if user != nil && user.FirstName != "" {
			userName = user.FirstName
			if user.LastName != "" {
				userName += " " + user.LastName
			}
		}
		_ = p.appCtx.DB.UserRepo().AddUser(context.Background(), privateUserID, userName)
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

		// 3. Welcome redirect from old bots
		if payload == "from_oldbot" || strings.HasPrefix(payload, "from_") {
			welcomeText := fmt.Sprintf(
				"👋 <b>Welcome to our Main Bot!</b>\n\n"+
					"🎉 You have successfully switched from our old bot!\n\n"+
					"✨ <b>Here you can enjoy:</b>\n"+
					"• 🌐 Multi-language filters (Malayalam, Tamil, English, Hindi, Telugu, Kannada, Multi)\n"+
					"• 🎞 Quality filters (480p, 720p, 1080p, 4K)\n"+
					"• ⚡ Ultra-fast direct download links\n"+
					"• 📦 Auto-deleting links for storage safety\n\n"+
					"💡 <i>Send any movie or series title now to start searching!</i>",
			)
			_, replyMarkup := p.buildWelcomeContent(user, botUsername)
			_, err := m.Reply(welcomeText, &telegram.SendOptions{
				ParseMode:   "html",
				ReplyMarkup: replyMarkup,
			})
			return err
		}

		// 4. Search deep-link from old bots or external buttons (q_<b64> or search_<query>)
		if query, ok := oldbot.DecodeSearchDeepLinkPayload(payload); ok && query != "" {
			var userID int64
			if user != nil && user.ID != 0 {
				userID = user.ID
			} else {
				userID = m.SenderID()
				if userID == 0 {
					userID = m.ChatID()
				}
			}

			// Force Subscribe Verification Flow if enabled
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

						checkSuccess := p.appCtx.FSub.CheckLoopSubDynamic(context.Background(), p.appCtx.Client, userID, m.ChatID(), promptMsgID, payload, unjoined...)
						if checkSuccess {
							if p.appCtx.Client != nil && promptMsgID != 0 {
								_, _ = p.appCtx.Client.DeleteMessages(m.ChatID(), []int32{promptMsgID})
							}
						} else {
							return nil
						}
					}
				}
			}

			// Execute search through autofilter plugin
			if p.appCtx != nil && p.appCtx.Registry != nil {
				if plug := p.appCtx.Registry.GetPlugin("autofilter"); plug != nil {
					if af, ok := plug.(interface {
						ExecuteSearch(m *telegram.NewMessage, query string) error
					}); ok {
						return af.ExecuteSearch(m, query)
					}
				}
			}
		}

		// 5. Media delivery deep link: file_<id>, filep_<id>, or direct file ID
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
				if user != nil && user.ID != 0 {
					userID = user.ID
				} else {
					userID = m.SenderID()
					if userID == 0 {
						userID = m.ChatID()
					}
				}

				if p.isMaintenanceMode() {
					isAdmin := p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.IsAdmin(userID)
					if !isAdmin {
						welcomeText, replyMarkup := p.buildWelcomeContent(user, botUsername)
						_, err := m.Reply("⚠️ <b>Bot is currently under maintenance. File delivery is temporarily paused.</b>\n\n"+welcomeText, &telegram.SendOptions{
							ParseMode:   "html",
							ReplyMarkup: replyMarkup,
						})
						return err
					}
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
							checkSuccess := p.appCtx.FSub.CheckLoopSubDynamic(context.Background(), p.appCtx.Client, userID, m.ChatID(), promptMsgID, payload, unjoined...)
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
					RecipientPeer:  m.Peer,
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

	startPic := ""
	if p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Config.StartPic != "" {
		startPic = p.appCtx.Config.StartPic
	}
	if startPic == "" {
		if _, err := os.Stat("assets/start.jpg"); err == nil {
			startPic = "assets/start.jpg"
		}
	}

	var sentMsg *telegram.NewMessage
	var err error
	if startPic != "" {
		sentMsg, err = m.ReplyMedia(startPic, &telegram.MediaOptions{
			Caption:     welcomeText,
			ParseMode:   "html",
			ReplyMarkup: replyMarkup,
		})
	}
	if err != nil || sentMsg == nil {
		sentMsg, err = m.Reply(welcomeText, &telegram.SendOptions{
			ParseMode:   "html",
			ReplyMarkup: replyMarkup,
		})
	}
	if err != nil && m.Client != nil && m.Peer != nil {
		sentMsg, err = m.Client.SendMessage(m.Peer, welcomeText, &telegram.SendOptions{
			ParseMode:   "html",
			ReplyMarkup: replyMarkup,
		})
	}
	if err == nil && sentMsg != nil && p.appCtx != nil && p.appCtx.AutoDelete != nil {
		_ = p.appCtx.AutoDelete.TrackSentMessage(sentMsg.ChatID(), sentMsg.ID, "start")
	}
	return err
}

