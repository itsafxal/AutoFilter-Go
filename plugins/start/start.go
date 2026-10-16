package start

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
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
func (p *Plugin) HandleStart(b *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	chat := ctx.EffectiveChat
	user := ctx.EffectiveSender.User

	botUsername := b.User.Username
	if botUsername == "" {
		botUsername = "bot"
	}

	// Handle Group and Supergroup chats
	if chat.Type == "group" || chat.Type == "supergroup" {
		if p.appCtx != nil && p.appCtx.DB != nil && p.appCtx.DB.GroupRepo() != nil {
			_ = p.appCtx.DB.GroupRepo().AddGroup(context.Background(), chat.Id, chat.Title)
		}
		chatTitle := commands.EscapeHTML(chat.Title)
		text := fmt.Sprintf(
			"👋 <b>Hey there!</b>\n\n"+
				"I'm ready to serve <b>%s</b>!\n\n"+
				"Use /cmds to view available commands.",
			chatTitle,
		)

		replyMarkup := gotgbot.InlineKeyboardMarkup{
			InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
				{
					{
						Text: "➜ ᴀᴅᴅ ᴍᴇ ᴛᴏ ʏᴏᴜʀ ɢʀᴏᴜᴘ ➜",
						Url:  fmt.Sprintf("https://t.me/%s?startgroup=true", botUsername),
					},
				},
				{
					{
						Text: "✜ ᴄᴏᴍᴍᴀɴᴅs",
						Url:  fmt.Sprintf("https://t.me/%s?start=help", botUsername),
					},
				},
			},
		}

		_, err := msg.Reply(b, text, &gotgbot.SendMessageOpts{
			ParseMode:   "HTML",
			ReplyMarkup: replyMarkup,
		})
		return err
	}

	// Handle Private chats
	if user != nil && p.appCtx != nil && p.appCtx.DB != nil && p.appCtx.DB.UserRepo() != nil {
		userName := user.FirstName
		if user.LastName != "" {
			userName += " " + user.LastName
		}
		_ = p.appCtx.DB.UserRepo().AddUser(context.Background(), user.Id, userName)
	}

	args := strings.Fields(msg.Text)
	payloadRes := commands.ValidateStartPayload(args)

	if payloadRes.HasPayload {
		if !payloadRes.IsValid {
			_, err := msg.Reply(b, fmt.Sprintf("⚠️ <b>Invalid Start Payload:</b> %s", commands.EscapeHTML(payloadRes.ErrorReason)), &gotgbot.SendMessageOpts{
				ParseMode: "HTML",
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
			_, err := msg.Reply(b, helpText, &gotgbot.SendMessageOpts{
				ParseMode: "HTML",
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
					userID = user.Id
				}

				opts := delivery.DeliveryOptions{
					UserID:         userID,
					ProtectContent: protectContent,
				}

				_, err := p.appCtx.Delivery.Deliver(context.Background(), chat.Id, fileID, opts)
				if err == nil {
					return nil
				}

				// Safe user-friendly error messages without internal data exposure
				if errors.Is(err, delivery.ErrMediaNotFound) {
					_, replyErr := msg.Reply(b, "⚠️ <b>File not found or has been removed.</b>", &gotgbot.SendMessageOpts{ParseMode: "HTML"})
					return replyErr
				}
				if errors.Is(err, delivery.ErrUserBanned) {
					_, replyErr := msg.Reply(b, "⛔ <b>Access Denied:</b> Your account is suspended from receiving files.", &gotgbot.SendMessageOpts{ParseMode: "HTML"})
					return replyErr
				}
				if errors.Is(err, delivery.ErrChatDisabled) {
					_, replyErr := msg.Reply(b, "⛔ <b>Access Denied:</b> This chat is disabled for media delivery.", &gotgbot.SendMessageOpts{ParseMode: "HTML"})
					return replyErr
				}
				if errors.Is(err, delivery.ErrDuplicateRequest) {
					_, replyErr := msg.Reply(b, "⏳ <b>Please wait:</b> A delivery request for this file is already in progress.", &gotgbot.SendMessageOpts{ParseMode: "HTML"})
					return replyErr
				}
				if errors.Is(err, delivery.ErrRateLimited) {
					_, replyErr := msg.Reply(b, "⚠️ <b>Flood Control:</b> Telegram rate limit reached. Please wait a moment before trying again.", &gotgbot.SendMessageOpts{ParseMode: "HTML"})
					return replyErr
				}
				if errors.Is(err, delivery.ErrBlockedByUser) {
					return nil
				}

				_, replyErr := msg.Reply(b, "❌ <b>Delivery Error:</b> Failed to deliver requested file. Please try again later.", &gotgbot.SendMessageOpts{ParseMode: "HTML"})
				return replyErr
			}
		}

		// Fallback for non-file payload
		text := fmt.Sprintf(
			"📦 <b>Deep-Link Payload Received:</b> <code>%s</code>\n\n"+
				"ℹ️ <i>Use /cmds to view available commands.</i>",
			commands.EscapeHTML(payloadRes.Payload),
		)
		_, err := msg.Reply(b, text, &gotgbot.SendMessageOpts{
			ParseMode: "HTML",
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

	replyMarkup := gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
			{
				{
					Text: "➜ ᴀᴅᴅ ᴍᴇ ᴛᴏ ʏᴏᴜʀ ɢʀᴏᴜᴘ ➜",
					Url:  fmt.Sprintf("https://t.me/%s?startgroup=true", botUsername),
				},
			},
			{
				{
					Text: "⭐ Cᴏɴᴛᴀᴄᴛ Aᴅᴍɪɴ ⭐",
					Url:  fmt.Sprintf("https://t.me/%s", botUsername),
				},
			},
		},
	}

	_, err := msg.Reply(b, welcomeText, &gotgbot.SendMessageOpts{
		ParseMode:   "HTML",
		ReplyMarkup: replyMarkup,
	})
	return err
}
