package info

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/AutoFilter-Go/internal/commands"
	"github.com/itsafxal/AutoFilter-Go/internal/plugins"
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
			Description: "Retrieve comprehensive user, group, and Telegram profile information",
			Usage:       "/info [@username | user_id]",
			Category:    "User Commands",
			Handler:     p.HandleInfo,
		},
	}
}

// CallbackHandlers returns the callback definitions for info plugin.
func (p *Plugin) CallbackHandlers() []plugins.CallbackDefinition {
	return []plugins.CallbackDefinition{
		{
			Prefix:  "info_close:",
			Handler: p.HandleCloseCallback,
		},
	}
}

// HandleCloseCallback handles dismissing the information card.
func (p *Plugin) HandleCloseCallback(cb *telegram.CallbackQuery) error {
	data := cb.DataString()
	parts := strings.Split(data, ":")
	if len(parts) != 2 {
		_, err := cb.Answer("Invalid request", &telegram.CallbackOptions{Alert: true})
		return err
	}

	requesterID, _ := strconv.ParseInt(parts[1], 10, 64)
	callerID := cb.SenderID

	// Allow requester, bot owner, or bot admin to close the card
	isAdmin := false
	if p.appCtx != nil && p.appCtx.Config != nil {
		isAdmin = p.appCtx.Config.IsAdmin(callerID)
	}

	if requesterID != 0 && callerID != requesterID && !isAdmin {
		_, err := cb.Answer("⚠️ Only the person who requested this information card can close it.", &telegram.CallbackOptions{Alert: true})
		return err
	}

	_, _ = cb.Answer("Information card closed.", &telegram.CallbackOptions{})
	_, err := cb.Delete()
	return err
}

// HandleInfo handles /info and /about commands.
func (p *Plugin) HandleInfo(m *telegram.NewMessage) error {
	if m == nil {
		return nil
	}

	// 1. Backward-compatible /about command handler
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(m.Text())), "/about") {
		return p.handleAbout(m)
	}

	// 2. Resolve target user, channel, or replied chat
	userTarget, channelTarget, chatTarget, repliedMsgID, err := p.resolveTarget(m)
	if err != nil {
		_, replyErr := m.Reply(err.Error(), &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}

	requesterID := m.SenderID()
	if requesterID == 0 && m.Sender != nil {
		requesterID = m.Sender.ID
	}

	// Handle Channel / Anonymous Admin Target
	if channelTarget != nil || chatTarget != nil {
		cardData := &ChannelCardData{
			Channel:      channelTarget,
			Chat:         chatTarget,
			CurrentChat:  m.ChatID(),
			RepliedMsgID: repliedMsgID,
		}
		if m.Channel != nil && m.Channel.Title != "" {
			cardData.CurrentTitle = m.Channel.Title
		} else if m.Chat != nil && m.Chat.Title != "" {
			cardData.CurrentTitle = m.Chat.Title
		}

		cardText := FormatChannelCard(cardData)
		var targetID int64
		var targetUsername string
		if channelTarget != nil {
			targetID = channelTarget.ID
			targetUsername = channelTarget.Username
		} else if chatTarget != nil {
			targetID = chatTarget.ID
		}

		markup := BuildChannelKeyboard(targetID, targetUsername, requesterID)
		_, replyErr := m.Reply(cardText, &telegram.SendOptions{
			ParseMode:   "html",
			ReplyMarkup: markup,
		})
		return replyErr
	}

	if userTarget == nil {
		_, replyErr := m.Reply("⚠️ <i>User information not available.</i>", &telegram.SendOptions{ParseMode: "html"})
		return replyErr
	}

	// 3. Collect User Profile Details & Network DCs
	var profilePhotoDC int32
	var botSessionDC int
	if p.appCtx != nil && p.appCtx.Client != nil {
		botSessionDC = p.appCtx.Client.GetDC()
	}

	// Retrieve user profile photo if available
	var userPhoto *telegram.UserPhoto
	if p.appCtx != nil && p.appCtx.Client != nil && !userTarget.Deleted {
		photos, err := p.appCtx.Client.GetProfilePhotos(userTarget.ID, &telegram.PhotosOptions{Limit: 1})
		if err == nil && len(photos) > 0 {
			userPhoto = &photos[0]
			profilePhotoDC = ExtractProfilePhotoDC(userPhoto)
		}
	}
	if profilePhotoDC == 0 && userTarget.Photo != nil {
		profilePhotoDC = ExtractProfilePhotoDC(userTarget.Photo)
	}

	// 4. Collect Group Context if executed in a group or supergroup
	isGroup := !m.IsPrivate()
	var groupStatus string
	var groupJoinDate int32
	chatTitle := "Unknown"

	if isGroup {
		if m.Channel != nil && m.Channel.Title != "" {
			chatTitle = m.Channel.Title
		} else if m.Chat != nil && m.Chat.Title != "" {
			chatTitle = m.Chat.Title
		}

		// Retrieve participant status in current group
		if p.appCtx != nil && p.appCtx.Client != nil {
			participant, partErr := p.appCtx.Client.GetChatMember(m.ChatID(), userTarget.ID)
			if partErr == nil && participant != nil {
				groupStatus, groupJoinDate = ParseGroupMemberStatus(participant)
			}
		}
	}

	cardData := &UserCardData{
		User:           userTarget,
		ProfilePhotoDC: profilePhotoDC,
		BotSessionDC:   botSessionDC,
		ChatID:         m.ChatID(),
		ChatTitle:      chatTitle,
		IsGroup:        isGroup,
		GroupStatus:    groupStatus,
		GroupJoinDate:  groupJoinDate,
		RepliedMsgID:   repliedMsgID,
	}

	cardText := FormatUserInfoCard(cardData)
	markup := BuildUserKeyboard(userTarget.ID, userTarget.Username, requesterID)

	// 5. Deliver with Profile Photo or Clean Text Fallback
	if userPhoto != nil && userPhoto.Photo != nil && p.appCtx != nil && p.appCtx.Client != nil {
		// Attempt direct MTProto photo delivery first (zero disk I/O, zero download bandwidth)
		_, err := m.ReplyMedia(userPhoto.Photo, &telegram.MediaOptions{
			Caption:     cardText,
			ParseMode:   "html",
			ReplyMarkup: markup,
		})
		if err == nil {
			return nil
		}

		// Fallback: download to temporary file, upload as media, and clean up
		tempPath := filepath.Join(os.TempDir(), fmt.Sprintf("tg_photo_%d_%d.jpg", userTarget.ID, time.Now().UnixNano()))
		downloadedPath, downloadErr := p.appCtx.Client.DownloadMedia(userPhoto.Photo, &telegram.DownloadOptions{
			FileName: tempPath,
		})
		if downloadErr == nil {
			defer os.Remove(downloadedPath)
			_, sendErr := m.ReplyMedia(downloadedPath, &telegram.MediaOptions{
				Caption:     cardText,
				ParseMode:   "html",
				ReplyMarkup: markup,
			})
			if sendErr == nil {
				return nil
			}
		}
	}

	// Clean text card fallback
	_, err = m.Reply(cardText, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: markup,
	})
	return err
}

// resolveTarget resolves the target peer from command arguments, replied message, or message sender.
func (p *Plugin) resolveTarget(m *telegram.NewMessage) (*telegram.UserObj, *telegram.Channel, *telegram.ChatObj, int32, error) {
	args := strings.TrimSpace(m.Args())

	// 1. Arguments specified: /info @username or /info user_id
	if args != "" {
		firstToken := strings.Fields(args)[0]

		// Lookup by username
		if strings.HasPrefix(firstToken, "@") || (!isNumeric(firstToken) && len(firstToken) >= 3) {
			username := strings.TrimPrefix(firstToken, "@")
			if len(username) < 3 {
				return nil, nil, nil, 0, fmt.Errorf("⚠️ <b>Invalid username:</b> Username must be at least 3 characters.")
			}
			if p.appCtx == nil || p.appCtx.Client == nil {
				return nil, nil, nil, 0, fmt.Errorf("⚠️ Telegram client connection unavailable.")
			}

			resolved, err := p.appCtx.Client.ResolveUsername(username)
			if err != nil {
				return nil, nil, nil, 0, fmt.Errorf("⚠️ <b>User Not Found:</b> Could not find user with username @%s. The username may be invalid, changed, or does not exist.", commands.EscapeHTML(username))
			}

			switch peer := resolved.(type) {
			case *telegram.UserObj:
				return peer, nil, nil, 0, nil
			case *telegram.Channel:
				return nil, peer, nil, 0, nil
			case *telegram.ChatObj:
				return nil, nil, peer, 0, nil
			default:
				return nil, nil, nil, 0, fmt.Errorf("⚠️ <b>Lookup Failed:</b> Username @%s resolved to an unsupported peer type.", commands.EscapeHTML(username))
			}
		}

		// Lookup by numerical ID
		if isNumeric(firstToken) {
			id, parseErr := strconv.ParseInt(firstToken, 10, 64)
			if parseErr != nil {
				return nil, nil, nil, 0, fmt.Errorf("⚠️ <b>Invalid ID:</b> Please specify a valid numerical Telegram ID.")
			}

			if p.appCtx != nil && p.appCtx.Client != nil {
				// Check Gogram in-memory cache
				if user, err := p.appCtx.Client.GetUser(id); err == nil && user != nil && user.ID != 0 {
					return user, nil, nil, 0, nil
				}

				// Check sendable peer resolution
				if sendable, err := p.appCtx.Client.GetSendableUser(id); err == nil && sendable != nil {
					if full, err := p.appCtx.Client.UsersGetFullUser(sendable); err == nil && full != nil && len(full.Users) > 0 {
						if u, ok := full.Users[0].(*telegram.UserObj); ok {
							return u, nil, nil, 0, nil
						}
					}
				}

				// Check MongoDB registered users for saved name fallback
				if p.appCtx.DB != nil && p.appCtx.DB.UserRepo() != nil {
					userRepo := p.appCtx.DB.UserRepo()
					if dbUser, err := userRepo.GetUser(context.Background(), id); err == nil && dbUser != nil {
						name := dbUser.Name
						if name == "" {
							name = "User"
						}
						return &telegram.UserObj{ID: id, FirstName: name}, nil, nil, 0, nil
					}
				}
			}

			return nil, nil, nil, 0, fmt.Errorf("⚠️ <b>User Not Found:</b> User ID <code>%d</code> is not known to the bot or has never interacted with it.", id)
		}

		return nil, nil, nil, 0, fmt.Errorf("⚠️ <b>Invalid Query:</b> Usage: <code>/info [@username | user_id]</code> or reply to a message.")
	}

	// 2. Replied message specified
	if m.IsReply() {
		reply, err := m.GetReplyMessage()
		if err == nil && reply != nil {
			if reply.Sender != nil && reply.Sender.ID != 0 {
				return reply.Sender, nil, nil, reply.ID, nil
			}
			if reply.SenderChat != nil {
				return nil, reply.SenderChat, nil, reply.ID, nil
			}
			if reply.SenderID() != 0 && p.appCtx != nil && p.appCtx.Client != nil {
				if u, err := p.appCtx.Client.GetUser(reply.SenderID()); err == nil && u != nil {
					return u, nil, nil, reply.ID, nil
				}
				// Return minimal user obj for sender ID
				return &telegram.UserObj{ID: reply.SenderID(), FirstName: "User"}, nil, nil, reply.ID, nil
			}
		}
	}

	// 3. Default to command sender
	if m.Sender != nil && m.Sender.ID != 0 {
		return m.Sender, nil, nil, 0, nil
	}

	// Hydrate peer if missing from cache
	plugins.EnsurePeerCached(m, 200*time.Millisecond)
	if m.Sender != nil && m.Sender.ID != 0 {
		return m.Sender, nil, nil, 0, nil
	}
	if m.SenderID() != 0 {
		return &telegram.UserObj{ID: m.SenderID(), FirstName: "User"}, nil, nil, 0, nil
	}

	return nil, nil, nil, 0, fmt.Errorf("⚠️ <i>Unable to determine message sender.</i>")
}

// handleAbout renders the custom /about card with interactive buttons.
func (p *Plugin) handleAbout(m *telegram.NewMessage) error {
	var botUser *telegram.UserObj
	if p.appCtx != nil {
		botUser = p.appCtx.BotUser
	}

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
			"‣ ʟɪʙʀᴀʀʏ : <a href='https://www.gogram.sbs/'>Gogram MTProto</a>\n"+
			"‣ ʟᴀɴɢᴜᴀɢᴇ : <a href='https://go.dev/'>Go</a>\n"+
			"‣ ᴅᴀᴛᴀ ʙᴀsᴇ : <a href='https://www.mongodb.com/'>ᴍᴏɴɢᴏ ᴅʙ & SQLite</a>\n"+
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

// isNumeric checks if a string consists entirely of digits (optionally prefixed with minus for negative chat IDs).
func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	s = strings.TrimPrefix(s, "-")
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
