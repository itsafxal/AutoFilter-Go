package start

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/MYTGBOTZ/internal/commands"
	"github.com/itsafxal/MYTGBOTZ/internal/delivery"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
)

// Plugin implements the /start command plugin.
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

// HandleStart handles the /start command in private and group chats.
func (p *Plugin) HandleStart(m *telegram.NewMessage) error {
	botUsername := "bot"
	if p.appCtx != nil && p.appCtx.BotUser != nil && p.appCtx.BotUser.Username != "" {
		botUsername = p.appCtx.BotUser.Username
	}

	// Handle Group and Supergroup chats
	if m.IsGroup() || m.IsChannel() {
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
			helpText := "Use /cmds to view available commands."
			if p.appCtx != nil && p.appCtx.Registry != nil {
				helpText = p.appCtx.Registry.GenerateHelpText()
			}
			_, err := m.Reply(helpText, &telegram.SendOptions{
				ParseMode: "html",
			})
			return err
		}

		// 2. Media delivery deep link: file_<id>, filep_<id>, or direct file ID
		if p.appCtx != nil && p.appCtx.Delivery != nil {
			var fileID string
			protectContent := false

			if strings.HasPrefix(payload, "filep_") {
				fileID = strings.TrimPrefix(payload, "filep_")
				protectContent = true
			} else if strings.HasPrefix(payload, "file_") {
				fileID = strings.TrimPrefix(payload, "file_")
			} else if payload != "about" && payload != "subscribe" && payload != "error" && payload != "okay" {
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

				_, err := p.appCtx.Delivery.Deliver(context.Background(), m.ChatID(), fileID, opts)
				if err == nil {
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

	// Standard private welcome
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
		AddRow(telegram.Button.URL("➜ ᴀᴅᴅ ᴍᴇ ᴛᴏ ʏᴏᴜʀ ɢʀᴏᴜᴘ ➜", fmt.Sprintf("https://t.me/%s?startgroup=true", botUsername))).
		AddRow(telegram.Button.URL("⭐ Cᴏɴᴛᴀᴄᴛ Aᴅᴍɪɴ ⭐", fmt.Sprintf("https://t.me/%s", botUsername))).
		Build()

	_, err := m.Reply(welcomeText, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: replyMarkup,
	})
	return err
}
