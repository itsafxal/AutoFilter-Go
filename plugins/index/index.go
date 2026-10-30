package index

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/AutoFilter-Go/internal/config"
	"github.com/itsafxal/AutoFilter-Go/internal/database/mongodb"
	"github.com/itsafxal/AutoFilter-Go/internal/plugins"
	"github.com/itsafxal/AutoFilter-Go/plugins/channel"
)

var linkRegex = regexp.MustCompile(`(?:https?://)?(?:t\.me|telegram\.me|telegram\.dog)/(?:c/)?([a-zA-Z0-9_]+)/(\d+)`)

// Plugin implements channel bulk indexing via /index and /setskip commands.
type Plugin struct {
	appCtx     *plugins.AppContext
	skipMu     sync.RWMutex
	skip       int
	indexMu    sync.Mutex
	isIndexing int32
	cancelFlag int32
}

// New creates a new index plugin instance.
func New(appCtx *plugins.AppContext) *Plugin {
	return &Plugin{
		appCtx: appCtx,
		skip:   0,
	}
}

// Name returns the unique plugin name.
func (p *Plugin) Name() string {
	return "index"
}

// Commands returns the command definitions for bulk indexing.
func (p *Plugin) Commands() []plugins.CommandDefinition {
	return []plugins.CommandDefinition{
		{
			Name:        "index",
			Description: "Index media files from a channel or forwarded message",
			Usage:       "/index [message_link]",
			Category:    "Indexing",
			Handler:     p.HandleIndexCommand,
		},
		{
			Name:        "indexlink",
			Description: "Index a range of media files between two message links",
			Usage:       "/indexlink <start_link> <end_link>",
			Category:    "Indexing",
			AdminOnly:   true,
			Handler:     p.HandleIndexLinkCommand,
		},
		{
			Name:        "setskip",
			Description: "Set the starting message ID for the next indexing run",
			Usage:       "/setskip <message_id>",
			Category:    "Indexing",
			AdminOnly:   true,
			Handler:     p.HandleSetSkipCommand,
		},
	}
}

// MessageHandlers registers listener for channel messages forwarded by Admins/Owners in PM.
func (p *Plugin) MessageHandlers() []plugins.MessageDefinition {
	return []plugins.MessageDefinition{
		{
			Filter: func(m *telegram.NewMessage) bool {
				if m == nil || p.appCtx == nil || p.appCtx.Config == nil {
					return false
				}
				if m.Chat != nil && m.Chat.ID < 0 {
					return false
				}
				if m.Message != nil && m.Message.PeerID != nil {
					switch m.Message.PeerID.(type) {
					case *telegram.PeerChannel, *telegram.PeerChat:
						return false
					}
				}
				var senderID int64
				if m.Sender != nil && m.Sender.ID != 0 {
					senderID = m.Sender.ID
				} else if m.Message != nil {
					senderID = m.SenderID()
				}
				if senderID == 0 || (!p.appCtx.Config.IsAdmin(senderID) && !p.appCtx.Config.IsOwner(senderID)) {
					return false
				}
				if !m.IsForward() && (m.Message == nil || m.Message.FwdFrom == nil) {
					return false
				}
				chatIDStr, lastMsgID := extractChannelInfo(m, m)
				return chatIDStr != "" && lastMsgID > 0
			},
			Handler: p.HandleForwardedChannelMessage,
		},
	}
}

// CallbackHandlers registers interactive callback query handlers for database selection and cancellation.
func (p *Plugin) CallbackHandlers() []plugins.CallbackDefinition {
	return []plugins.CallbackDefinition{
		{
			Prefix:  "index#",
			Handler: p.HandleIndexCallback,
		},
		{
			Prefix:  "index_cancel",
			Handler: p.HandleCancelCallback,
		},
		{
			Prefix:  "close_data",
			Handler: p.HandleCloseCallback,
		},
	}
}

// SetSkip sets the starting message ID for indexing.
func (p *Plugin) SetSkip(skip int) {
	p.skipMu.Lock()
	defer p.skipMu.Unlock()
	p.skip = skip
}

// GetSkip gets the starting message ID for indexing.
func (p *Plugin) GetSkip() int {
	p.skipMu.RLock()
	defer p.skipMu.RUnlock()
	return p.skip
}

// HandleSetSkipCommand configures the starting skip message ID.
func (p *Plugin) HandleSetSkipCommand(m *telegram.NewMessage) error {
	if m == nil {
		return nil
	}
	fields := strings.Fields(m.Text())
	if len(fields) < 2 {
		_, err := m.Reply(fmt.Sprintf(
			"ℹ️ <b>Current Skip Value:</b> <code>%d</code>\n\n"+
				"To update the skip message ID for indexing, specify a number:\n"+
				"<b>Usage:</b> <code>/setskip &lt;message_id&gt;</code>\n"+
				"<b>Example:</b> <code>/setskip 100</code>",
			p.GetSkip(),
		), &telegram.SendOptions{
			ParseMode: "html",
		})
		return err
	}

	skipVal, err := strconv.Atoi(fields[1])
	if err != nil || skipVal < 0 {
		_, replyErr := m.Reply("❌ <b>Invalid input!</b> The skip number must be a <b>positive integer</b> (e.g. <code>/setskip 50</code>).", &telegram.SendOptions{
			ParseMode: "html",
		})
		return replyErr
	}

	p.SetSkip(skipVal)
	_, replyErr := m.Reply(fmt.Sprintf("✅ <b>Success!</b> The <b>SKIP number</b> has been set to <code>%d</code>.\n\nThe next indexing operation will commence from this message ID.", skipVal), &telegram.SendOptions{
		ParseMode: "html",
	})
	return replyErr
}

// HandleForwardedChannelMessage allows Admins/Owners to forward a post from a channel directly to the bot to trigger indexing.
func (p *Plugin) HandleForwardedChannelMessage(m *telegram.NewMessage) error {
	if m == nil {
		return nil
	}
	chatIDStr, lastMsgID := extractChannelInfo(m, m)
	if chatIDStr == "" || lastMsgID <= 0 {
		return nil
	}
	return p.promptIndexConfirmation(m, chatIDStr, lastMsgID)
}

// HandleIndexCommand initiates the channel indexing confirmation flow.
func (p *Plugin) HandleIndexCommand(m *telegram.NewMessage) error {
	if m == nil {
		return nil
	}

	targetMsg := m
	if m.IsReply() {
		if reply, err := m.GetReplyMessage(); err == nil && reply != nil {
			targetMsg = reply
		}
	}

	chatIDStr, lastMsgID := extractChannelInfo(m, targetMsg)
	if chatIDStr == "" || lastMsgID <= 0 {
		_, err := m.Reply("💡 To initiate indexing, please forward the <b>last message</b> from the desired channel/group, reply to a forwarded message with <code>/index</code>, or provide a <b>valid Telegram message link</b> (e.g. <code>https://t.me/channel_username/123</code>).", &telegram.SendOptions{
			ParseMode: "html",
		})
		return err
	}

	return p.promptIndexConfirmation(m, chatIDStr, lastMsgID)
}

// HandleIndexLinkCommand parses start and end links and prompts for indexing in that range.
func (p *Plugin) HandleIndexLinkCommand(m *telegram.NewMessage) error {
	if m == nil {
		return nil
	}
	fields := strings.Fields(m.Text())
	if len(fields) < 3 {
		_, err := m.Reply(
			"💡 <b>Usage:</b> <code>/indexlink &lt;first_message_link&gt; &lt;last_message_link&gt;</code>\n\n"+
				"<b>Example:</b>\n"+
				"<code>/indexlink https://t.me/c/1234567890/5 https://t.me/c/1234567890/10</code>\n"+
				"or\n"+
				"<code>/indexlink https://t.me/mychannel/5 https://t.me/mychannel/10</code>",
			&telegram.SendOptions{ParseMode: "html"},
		)
		return err
	}

	startLink := fields[1]
	endLink := fields[2]

	m1 := linkRegex.FindStringSubmatch(startLink)
	m2 := linkRegex.FindStringSubmatch(endLink)

	if len(m1) != 3 || len(m2) != 3 {
		_, err := m.Reply("❌ <b>Invalid message link format!</b> Please provide two valid Telegram message links.", &telegram.SendOptions{
			ParseMode: "html",
		})
		return err
	}

	channel1 := m1[1]
	msgID1, _ := strconv.Atoi(m1[2])
	channel2 := m2[1]
	msgID2, _ := strconv.Atoi(m2[2])

	if channel1 != channel2 {
		_, err := m.Reply("❌ <b>Channel mismatch!</b> Both message links must belong to the same channel.", &telegram.SendOptions{
			ParseMode: "html",
		})
		return err
	}

	if msgID1 <= 0 || msgID2 <= 0 {
		_, err := m.Reply("❌ <b>Invalid message ID(s)!</b> Message IDs in the links must be positive integers.", &telegram.SendOptions{
			ParseMode: "html",
		})
		return err
	}

	startID := msgID1
	endID := msgID2
	if startID > endID {
		startID, endID = endID, startID
	}

	chatIDStr := channel1
	if strings.Contains(startLink, "/c/") || isNumeric(channel1) {
		norm := config.NormalizeChannelID(parseInt64(channel1))
		chatIDStr = fmt.Sprintf("%d", norm)
	}

	var peer any = chatIDStr
	if parsedInt, err := strconv.ParseInt(chatIDStr, 10, 64); err == nil {
		peer = parsedInt
	}

	// Verify channel accessibility with bot client
	if p.appCtx != nil && p.appCtx.Client != nil {
		testMsgs, err := p.appCtx.Client.GetMessages(peer, &telegram.SearchOption{
			IDs: []int32{int32(startID), int32(endID)},
		})
		if err != nil {
			errStr := strings.ToUpper(err.Error())
			if strings.Contains(errStr, "CHANNEL_INVALID") || strings.Contains(errStr, "CHANNEL_PRIVATE") || strings.Contains(errStr, "CHAT_ADMIN_REQUIRED") {
				_, rErr := m.Reply("Access Denied 🔒: This channel is private. Please <b>make me an administrator</b> in that chat to enable indexing.", &telegram.SendOptions{
					ParseMode: "html",
				})
				return rErr
			}
			_, rErr := m.Reply(fmt.Sprintf("⚠️ An error occurred while verifying chat access: <code>%s</code>", err.Error()), &telegram.SendOptions{
				ParseMode: "html",
			})
			return rErr
		}
		if len(testMsgs) == 0 {
			_, rErr := m.Reply("🔍 <b>Messages Not Found:</b> The specified messages in the links could not be accessed.", &telegram.SendOptions{
				ParseMode: "html",
			})
			return rErr
		}
	}

	senderID := m.SenderID()
	totalMsgs := endID - startID + 1

	kb := telegram.NewKeyboard().
		AddRow(
			telegram.Button.Data("💾 Index to DB2", fmt.Sprintf("index#link1#%s#%d#%d#%d", chatIDStr, startID, endID, senderID)),
			telegram.Button.Data("🗃️ Index to DB3", fmt.Sprintf("index#link2#%s#%d#%d#%d", chatIDStr, startID, endID, senderID)),
		).
		AddRow(
			telegram.Button.Data("🔄 Distribute (DB2 ↔ DB3)", fmt.Sprintf("index#link5#%s#%d#%d#%d", chatIDStr, startID, endID, senderID)),
		).
		AddRow(
			telegram.Button.Data("✖️ Close Options", "close_data"),
		).Build()

	text := fmt.Sprintf(
		"✨ <b>Range Indexing Request Confirmation</b> ✨\n\n"+
			"• <b>Channel:</b> <code>%s</code>\n"+
			"• <b>Start Message ID:</b> <code>%d</code>\n"+
			"• <b>End Message ID:</b> <code>%d</code>\n"+
			"• <b>Total Messages to Scan:</b> <code>%d</code>\n\n"+
			"Please select your preferred database:",
		chatIDStr, startID, endID, totalMsgs,
	)

	_, err := m.Reply(text, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: kb,
	})
	return err
}

func (p *Plugin) promptIndexConfirmation(m *telegram.NewMessage, chatIDStr string, lastMsgID int) error {
	var peer any = chatIDStr
	if parsedInt, err := strconv.ParseInt(chatIDStr, 10, 64); err == nil {
		peer = parsedInt
	}

	// Verify chat and message accessibility
	if p.appCtx != nil && p.appCtx.Client != nil {
		testMsgs, err := p.appCtx.Client.GetMessages(peer, &telegram.SearchOption{
			IDs: []int32{int32(lastMsgID)},
		})
		if err != nil {
			errStr := strings.ToUpper(err.Error())
			if strings.Contains(errStr, "CHANNEL_INVALID") || strings.Contains(errStr, "CHANNEL_PRIVATE") || strings.Contains(errStr, "CHAT_ADMIN_REQUIRED") {
				_, rErr := m.Reply("Access Denied 🔒: This appears to be a private channel/group. Please <b>make me an administrator</b> in that chat to enable indexing.", &telegram.SendOptions{
					ParseMode: "html",
				})
				return rErr
			}
			if strings.Contains(errStr, "USERNAME_INVALID") || strings.Contains(errStr, "USERNAME_NOT_MODIFIED") {
				_, rErr := m.Reply("❌ <b>Invalid Link/Username:</b> The specified channel username or link is incorrect or not found. Double-check and try again.", &telegram.SendOptions{
					ParseMode: "html",
				})
				return rErr
			}
			_, rErr := m.Reply(fmt.Sprintf("⚠️ An error occurred while verifying chat access or message availability: <code>%s</code>", err.Error()), &telegram.SendOptions{
				ParseMode: "html",
			})
			return rErr
		}
		if len(testMsgs) == 0 {
			_, rErr := m.Reply("🔍 <b>Message Not Found:</b> The specified message ID is invalid or inaccessible.", &telegram.SendOptions{
				ParseMode: "html",
			})
			return rErr
		}
	}

	senderID := m.SenderID()
	isAdmin := p.appCtx != nil && p.appCtx.Config != nil && (p.appCtx.Config.IsAdmin(senderID) || p.appCtx.Config.IsOwner(senderID))

	if isAdmin {
		kb := telegram.NewKeyboard().
			AddRow(
				telegram.Button.Data("💾 Index to DB2", fmt.Sprintf("index#accept1#%s#%d#%d", chatIDStr, lastMsgID, senderID)),
				telegram.Button.Data("🗃️ Index to DB3", fmt.Sprintf("index#accept2#%s#%d#%d", chatIDStr, lastMsgID, senderID)),
			).
			AddRow(
				telegram.Button.Data("🔄 Distribute (DB2 ↔ DB3)", fmt.Sprintf("index#accept5#%s#%d#%d", chatIDStr, lastMsgID, senderID)),
			).
			AddRow(
				telegram.Button.Data("✖️ Close Options", "close_data"),
			).Build()

		text := fmt.Sprintf(
			"✨ <b>Indexing Request Confirmation</b> ✨\n\n"+
				"You are about to initiate indexing for:\n"+
				"• <b>Chat ID/Username:</b> <code>%s</code>\n"+
				"• <b>Last Message ID:</b> <code>%d</code>\n\n"+
				"Please select your preferred database for indexing:",
			chatIDStr, lastMsgID,
		)
		_, err := m.Reply(text, &telegram.SendOptions{
			ParseMode:   "html",
			ReplyMarkup: kb,
		})
		return err
	}

	// Non-admin submission: send to bot admins
	userMention := fmt.Sprintf("<a href=\"tg://user?id=%d\">User</a>", senderID)
	if m.Sender != nil && m.Sender.FirstName != "" {
		userMention = fmt.Sprintf("<a href=\"tg://user?id=%d\">%s</a>", senderID, m.Sender.FirstName)
	}

	adminNotice := fmt.Sprintf(
		"🚨 <b>New Indexing Request</b> 🚨\n\n"+
			"<b>Initiated By:</b> %s (ID: <code>%d</code>)\n"+
			"<b>Target Chat:</b> <code>%s</code>\n"+
			"<b>Last Message ID:</b> <code>%d</code>\n\n"+
			"Please review and decide:",
		userMention, senderID, chatIDStr, lastMsgID,
	)

	reqKb := telegram.NewKeyboard().
		AddRow(
			telegram.Button.Data("✅ Accept (DB2)", fmt.Sprintf("index#accept1#%s#%d#%d", chatIDStr, lastMsgID, senderID)),
			telegram.Button.Data("✅ Accept (DB3)", fmt.Sprintf("index#accept2#%s#%d#%d", chatIDStr, lastMsgID, senderID)),
		).
		AddRow(
			telegram.Button.Data("🔄 Distribute (DB2 ↔ DB3)", fmt.Sprintf("index#accept5#%s#%d#%d", chatIDStr, lastMsgID, senderID)),
		).
		AddRow(
			telegram.Button.Data("❌ Decline Request", fmt.Sprintf("index#reject#%s#%d#%d", chatIDStr, lastMsgID, senderID)),
		).Build()

	if p.appCtx != nil && p.appCtx.Config != nil && p.appCtx.Client != nil {
		for _, admin := range p.appCtx.Config.Admins {
			_, _ = p.appCtx.Client.SendMessage(admin, adminNotice, &telegram.SendOptions{
				ParseMode:   "html",
				ReplyMarkup: reqKb,
			})
		}
	}

	_, err := m.Reply("🌟 <b>Request Submitted!</b> 🌟\n\n"+
		"Thank you for contributing! Your indexing request has been sent to our moderators for review.\n"+
		"You will receive a notification once your request is accepted or declined.\n\n"+
		"Appreciate your patience! 🙏", &telegram.SendOptions{
		ParseMode: "html",
	})
	return err
}

// HandleIndexCallback processes index acceptance and rejection button clicks.
func (p *Plugin) HandleIndexCallback(cb *telegram.CallbackQuery) error {
	if cb == nil {
		return nil
	}

	data := cb.DataString()
	parts := strings.Split(data, "#")
	if len(parts) < 5 {
		_, err := cb.Answer("🚫 Invalid callback payload.", &telegram.CallbackOptions{
			Alert: true,
		})
		return err
	}

	action := parts[1]
	chatIDStr := parts[2]
	var startMsgID, endMsgID int
	var fromUserID int64

	if strings.HasPrefix(action, "link") {
		if len(parts) < 6 {
			_, err := cb.Answer("🚫 Invalid callback payload.", &telegram.CallbackOptions{
				Alert: true,
			})
			return err
		}
		startMsgID, _ = strconv.Atoi(parts[3])
		endMsgID, _ = strconv.Atoi(parts[4])
		fromUserID, _ = strconv.ParseInt(parts[5], 10, 64)
		switch action {
		case "link1":
			action = "accept1"
		case "link2":
			action = "accept2"
		case "link5":
			action = "accept5"
		}
	} else if action == "reject" {
		fromUserID, _ = strconv.ParseInt(parts[4], 10, 64)
		_, _ = cb.Delete()
		if p.appCtx != nil && p.appCtx.Client != nil && fromUserID > 0 {
			_, _ = p.appCtx.Client.SendMessage(fromUserID, fmt.Sprintf("❌ Your submission for indexing <b>%s</b> has been <b>declined</b> by our moderators.", chatIDStr), &telegram.SendOptions{
				ParseMode: "html",
			})
		}
		return nil
	} else {
		lastMsgID, _ := strconv.Atoi(parts[3])
		fromUserID, _ = strconv.ParseInt(parts[4], 10, 64)
		startMsgID = p.GetSkip()
		if startMsgID <= 0 || startMsgID > lastMsgID {
			startMsgID = 1
		}
		endMsgID = lastMsgID
	}

	if action != "accept1" && action != "accept2" && action != "accept5" {
		_, err := cb.Answer("🚫 Unknown action.", &telegram.CallbackOptions{
			Alert: true,
		})
		return err
	}

	// Verify indexing lock
	if !atomic.CompareAndSwapInt32(&p.isIndexing, 0, 1) {
		_, err := cb.Answer("⚠️ An indexing process is already active. Please wait for it to complete.", &telegram.CallbackOptions{
			Alert: true,
		})
		return err
	}

	_, _ = cb.Answer("🚀 Initiating indexing process...", &telegram.CallbackOptions{
		Alert: false,
	})

	if p.appCtx != nil && p.appCtx.Config != nil && !p.appCtx.Config.IsAdmin(fromUserID) && p.appCtx.Client != nil {
		_, _ = p.appCtx.Client.SendMessage(fromUserID, fmt.Sprintf("✅ Your request to index <b>%s</b> has been <b>accepted</b> by our moderators and will commence shortly!", chatIDStr), &telegram.SendOptions{
			ParseMode: "html",
		})
	}

	cancelKb := telegram.NewKeyboard().
		AddRow(
			telegram.Button.Data("🛑 Cancel Indexing", "index_cancel"),
		).Build()

	progressMsg, editErr := cb.Edit(
		"✨ <b>Indexing Operation Commencing!</b> ✨\n\n"+
			"Please monitor this message for real-time progress updates.\n\n"+
			"To halt the process at any time, tap the 'Cancel Indexing' button.",
		&telegram.SendOptions{
			ParseMode:   "html",
			ReplyMarkup: cancelKb,
		},
	)
	if editErr != nil {
		progressMsg, _ = cb.GetMessage()
	}

	atomic.StoreInt32(&p.cancelFlag, 0)

	// Launch indexing in a background goroutine
	go func() {
		defer atomic.StoreInt32(&p.isIndexing, 0)
		p.runIndexRangeOperation(progressMsg, chatIDStr, startMsgID, endMsgID, action, fromUserID)
		if !strings.HasPrefix(parts[1], "link") {
			p.SetSkip(0)
		}
	}()

	return nil
}

// HandleCancelCallback signals cancellation to the active indexing operation.
func (p *Plugin) HandleCancelCallback(cb *telegram.CallbackQuery) error {
	if cb == nil {
		return nil
	}
	atomic.StoreInt32(&p.cancelFlag, 1)
	_, err := cb.Answer("🛑 Indexing cancellation requested. Stopping gracefully...", &telegram.CallbackOptions{
		Alert: true,
	})
	return err
}

// HandleCloseCallback closes an options dialog.
func (p *Plugin) HandleCloseCallback(cb *telegram.CallbackQuery) error {
	if cb == nil {
		return nil
	}
	_, err := cb.Delete()
	return err
}

func (p *Plugin) runIndexOperation(progressMsg *telegram.NewMessage, chatIDStr string, lastMsgID int, dbType string, fromUserID int64) {
	startMsgID := p.GetSkip()
	if startMsgID <= 0 || startMsgID > lastMsgID {
		startMsgID = 1
	}
	p.runIndexRangeOperation(progressMsg, chatIDStr, startMsgID, lastMsgID, dbType, fromUserID)
}

type messageBatch struct {
	chunkStart int
	chunkEnd   int
	messages   []telegram.NewMessage
}

type indexStats struct {
	mu                sync.RWMutex
	startTime         time.Time
	startMsgID        int
	endMsgID          int
	totalMessages     int
	messagesFetched   int
	messagesProcessed int
	maxSavedMsgID     int
	totalSaved        int
	totalDuplicates   int
	totalNoMedia      int
	totalErrors       int
	isFloodWait       bool
	floodWaitSec      int
	lastScannedID     int
}

func newIndexStats(startID, endID int) *indexStats {
	total := endID - startID + 1
	if total < 0 {
		total = 0
	}
	return &indexStats{
		startTime:     time.Now(),
		startMsgID:    startID,
		endMsgID:      endID,
		totalMessages: total,
		lastScannedID: startID,
	}
}

func (s *indexStats) addFetched(n int, lastID int) {
	s.mu.Lock()
	s.messagesFetched += n
	if lastID > s.lastScannedID {
		s.lastScannedID = lastID
	}
	s.mu.Unlock()
}

func (s *indexStats) addProcessed(n int) {
	s.mu.Lock()
	s.messagesProcessed += n
	s.mu.Unlock()
}

func (s *indexStats) addSaved(n int) {
	s.mu.Lock()
	s.totalSaved += n
	s.mu.Unlock()
}

func (s *indexStats) addDuplicates(n int) {
	s.mu.Lock()
	s.totalDuplicates += n
	s.mu.Unlock()
}

func (s *indexStats) addNoMedia(n int) {
	s.mu.Lock()
	s.totalNoMedia += n
	s.mu.Unlock()
}

func (s *indexStats) addErrors(n int) {
	s.mu.Lock()
	s.totalErrors += n
	s.mu.Unlock()
}

func (s *indexStats) updateMaxSavedID(id int) {
	s.mu.Lock()
	if id > s.maxSavedMsgID {
		s.maxSavedMsgID = id
	}
	s.mu.Unlock()
}

func (s *indexStats) setFloodWait(sec int) {
	s.mu.Lock()
	s.isFloodWait = true
	s.floodWaitSec = sec
	s.mu.Unlock()
}

func (s *indexStats) clearFloodWait() {
	s.mu.Lock()
	s.isFloodWait = false
	s.floodWaitSec = 0
	s.mu.Unlock()
}

type indexSnapshot struct {
	fetched       int
	processed     int
	maxSavedID    int
	saved         int
	duplicates    int
	noMedia       int
	errorsCount   int
	isFloodWait   bool
	floodSec      int
	fetchSpeed    float64
	indexSpeed    float64
	remaining     int
	lastScannedID int
	elapsed       time.Duration
	timeLeft      time.Duration
}

func (s *indexStats) getSnapshot() indexSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	elapsed := time.Since(s.startTime)
	sec := elapsed.Seconds()
	if sec < 0.1 {
		sec = 0.1
	}

	fetchSpd := float64(s.messagesFetched) / sec
	idxSpd := float64(s.totalSaved) / sec
	rem := s.totalMessages - s.messagesFetched
	if rem < 0 {
		rem = 0
	}

	var tLeft time.Duration
	if fetchSpd > 0 && rem > 0 {
		tLeft = time.Duration(float64(rem)/fetchSpd) * time.Second
	}

	return indexSnapshot{
		fetched:       s.messagesFetched,
		processed:     s.messagesProcessed,
		maxSavedID:    s.maxSavedMsgID,
		saved:         s.totalSaved,
		duplicates:    s.totalDuplicates,
		noMedia:       s.totalNoMedia,
		errorsCount:   s.totalErrors,
		isFloodWait:   s.isFloodWait,
		floodSec:      s.floodWaitSec,
		fetchSpeed:    fetchSpd,
		indexSpeed:    idxSpd,
		remaining:     rem,
		lastScannedID: s.lastScannedID,
		elapsed:       elapsed,
		timeLeft:      tLeft,
	}
}

func (p *Plugin) runIndexRangeOperation(progressMsg *telegram.NewMessage, chatIDStr string, startMsgID, endMsgID int, dbType string, fromUserID int64) {
	startTime := time.Now()
	if startMsgID <= 0 {
		startMsgID = 1
	}
	if endMsgID < startMsgID {
		startMsgID, endMsgID = endMsgID, startMsgID
	}

	totalMessages := endMsgID - startMsgID + 1
	stats := newIndexStats(startMsgID, endMsgID)

	var peer any = chatIDStr
	if parsedInt, err := strconv.ParseInt(chatIDStr, 10, 64); err == nil {
		peer = parsedInt
	}

	cancelKb := telegram.NewKeyboard().
		AddRow(
			telegram.Button.Data("🛑 Cancel Indexing", "index_cancel"),
		).Build()

	channelDisplay := formatChannelDisplay(chatIDStr)
	targetDBDisplay := formatTargetDB(dbType)

	// Initial progress notification
	if progressMsg != nil {
		initSnap := stats.getSnapshot()
		initText := renderProgressUpdate(
			channelDisplay, targetDBDisplay, startMsgID, endMsgID, totalMessages,
			initSnap.fetched, initSnap.processed, initSnap.maxSavedID,
			initSnap.saved, initSnap.duplicates, initSnap.noMedia, initSnap.errorsCount,
			0, 0, totalMessages, "0s", "Calculating...", false, 0,
		)
		_, _ = progressMsg.Edit(initText, &telegram.SendOptions{
			ParseMode:   "html",
			ReplyMarkup: cancelKb,
		})
	}

	queue := make(chan messageBatch, 30)
	stopCh := make(chan struct{})
	var wg sync.WaitGroup

	// Determine base pacing delay (default 850ms, safely beneath Telegram's ~1s rate limit bucket)
	baseDelay := 850 * time.Millisecond
	if envDelay := os.Getenv("INDEX_FETCH_DELAY_MS"); envDelay != "" {
		if parsed, err := strconv.Atoi(envDelay); err == nil && parsed >= 200 {
			baseDelay = time.Duration(parsed) * time.Millisecond
		}
	}
	minDelay := 800 * time.Millisecond
	maxDelay := 3000 * time.Millisecond

	// 1. Fetcher Worker (Dedicated Telegram MTProto Producer)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(queue)

		currentDelay := baseDelay
		consecutiveSuccess := 0
		const chunkSize = 200

		for currentID := startMsgID; currentID <= endMsgID; currentID += chunkSize {
			if atomic.LoadInt32(&p.cancelFlag) == 1 {
				break
			}
			select {
			case <-stopCh:
				return
			default:
			}

			chunkEnd := currentID + chunkSize - 1
			if chunkEnd > endMsgID {
				chunkEnd = endMsgID
			}

			ids := make([]int32, 0, chunkEnd-currentID+1)
			for id := currentID; id <= chunkEnd; id++ {
				ids = append(ids, int32(id))
			}

			var fetched []telegram.NewMessage
			var success bool

			for attempt := 1; attempt <= 6; attempt++ {
				if atomic.LoadInt32(&p.cancelFlag) == 1 {
					break
				}
				select {
				case <-stopCh:
					return
				default:
				}

				msgs, err := p.appCtx.Client.GetMessages(peer, &telegram.SearchOption{
					IDs: ids,
				})
				if err == nil {
					fetched = msgs
					success = true
					break
				}

				// Telegram FloodWait / 420 Rate Limit Interception
				if isFloodWaitError(err) {
					waitSec := extractFloodWaitSeconds(err)
					if waitSec <= 0 {
						waitSec = 30
					}
					if p.appCtx.Logger != nil {
						p.appCtx.Logger.Warn("⚠️ [Index] Telegram FloodWait encountered",
							"wait_seconds", waitSec,
							"chunk_start", currentID,
							"chunk_end", chunkEnd,
							"attempt", attempt,
						)
					}

					stats.setFloodWait(waitSec)

					sleepTimer := time.NewTimer(time.Duration(waitSec+1) * time.Second)
					select {
					case <-sleepTimer.C:
					case <-stopCh:
						sleepTimer.Stop()
						return
					}

					stats.clearFloodWait()
					// Adaptive backoff: increase delay to avoid re-triggering Telegram bucket limit
					if currentDelay < maxDelay {
						currentDelay += 250 * time.Millisecond
					}
					consecutiveSuccess = 0
					// Retry the EXACT same chunk without dropping files
					continue
				}

				errStr := strings.ToUpper(err.Error())
				if strings.Contains(errStr, "CHANNEL_INVALID") || strings.Contains(errStr, "CHANNEL_PRIVATE") || strings.Contains(errStr, "CHAT_ADMIN_REQUIRED") {
					if p.appCtx.Logger != nil {
						p.appCtx.Logger.Error("❌ [Index] Fatal chat permission error during fetch", "error", err)
					}
					stats.addErrors(len(ids))
					return
				}

				// Transient network or connection error
				if p.appCtx.Logger != nil {
					p.appCtx.Logger.Warn("⚠️ [Index] Transient MTProto error, retrying...",
						"attempt", attempt,
						"error", err,
						"chunk_start", currentID,
					)
				}
				backoff := time.Duration(attempt) * time.Second
				if backoff > 5*time.Second {
					backoff = 5 * time.Second
				}
				time.Sleep(backoff)
			}

			if !success {
				if atomic.LoadInt32(&p.cancelFlag) == 1 {
					break
				}
				stats.addErrors(len(ids))
				continue
			}

			// Hand over to processing pipeline
			stats.addFetched(len(ids), chunkEnd)
			select {
			case queue <- messageBatch{chunkStart: currentID, chunkEnd: chunkEnd, messages: fetched}:
			case <-stopCh:
				return
			}

			// Adaptive rate tuning: gradually reduce delay when stable
			consecutiveSuccess++
			if consecutiveSuccess >= 25 && currentDelay > minDelay {
				currentDelay -= 50 * time.Millisecond
				consecutiveSuccess = 0
			}

			// Inter-request pacing sleep
			select {
			case <-time.After(currentDelay):
			case <-stopCh:
				return
			}
		}
	}()

	// 2. Indexer Worker (Dedicated MongoDB & SQLite Consumer)
	wg.Add(1)
	go func() {
		defer wg.Done()

		for batch := range queue {
			if atomic.LoadInt32(&p.cancelFlag) == 1 {
				// Finish current batch quickly then terminate
			}

			var mediaDocs []mongodb.MediaDoc
			var fileIDs []string
			var msgIDs []int

			for _, m := range batch.messages {
				doc := channel.ExtractMediaFromMessage(&m)
				if doc == nil || doc.FileID == "" {
					stats.addNoMedia(1)
					continue
				}
				mediaDocs = append(mediaDocs, *doc)
				fileIDs = append(fileIDs, doc.FileID)
				msgIDs = append(msgIDs, int(m.ID))
			}

			stats.addProcessed(len(batch.messages))

			if len(mediaDocs) > 0 {
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				existMap, err := p.appCtx.DB.BulkCheckMediaExistsByDocs(ctx, mediaDocs)
				cancel()
				if err != nil {
					if p.appCtx.Logger != nil {
						p.appCtx.Logger.Warn("⚠️ [Index] Pre-write deduplication check failed; skipping batch to prevent duplicate storage growth",
							"error", err,
							"batch_size", len(mediaDocs),
						)
					}
					stats.addErrors(len(mediaDocs))
					continue
				}

				var newDocs []mongodb.MediaDoc
				var newMsgIDs []int
				for i, doc := range mediaDocs {
					if existMap[doc.FileID] {
						stats.addDuplicates(1)
					} else {
						newDocs = append(newDocs, doc)
						newMsgIDs = append(newMsgIDs, msgIDs[i])
					}
				}

				if len(newDocs) > 0 {
					ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
					var saved, dups int
					var saveErr error

					switch dbType {
					case "accept1", "link1":
						saved, dups, saveErr = p.appCtx.DB.BulkSaveMedia(ctx, newDocs, 0)
					case "accept2", "link2":
						saved, dups, saveErr = p.appCtx.DB.BulkSaveMedia(ctx, newDocs, 1)
					case "accept5", "link5":
						saved, dups, saveErr = p.appCtx.DB.BulkSaveMediaAlternating(ctx, newDocs, 100)
					default:
						saved, dups, saveErr = p.appCtx.DB.BulkSaveMediaAlternating(ctx, newDocs, 100)
					}
					cancel()

					if saveErr != nil {
						stats.addErrors(len(newDocs))
					} else {
						stats.addSaved(saved)
						stats.addDuplicates(dups)
						for _, id := range newMsgIDs {
							stats.updateMaxSavedID(id)
						}
					}
				}
			}
		}
	}()

	// 3. UI Updater Goroutine (Throttled progress reporting every 3.5 seconds)
	uiDone := make(chan struct{})
	go func() {
		defer close(uiDone)
		ticker := time.NewTicker(3500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				snap := stats.getSnapshot()
				elapsedStr := formatCompactDuration(snap.elapsed)
				timeLeftStr := formatETA(snap.timeLeft)
				if snap.fetchSpeed <= 0 {
					timeLeftStr = "Calculating..."
				}

				text := renderProgressUpdate(
					channelDisplay, targetDBDisplay, startMsgID, endMsgID, totalMessages,
					snap.fetched, snap.processed, snap.maxSavedID,
					snap.saved, snap.duplicates, snap.noMedia, snap.errorsCount,
					snap.fetchSpeed, snap.indexSpeed, snap.remaining,
					elapsedStr, timeLeftStr, snap.isFloodWait, snap.floodSec,
				)

				safeEditProgress(progressMsg, text, cancelKb)
			case <-stopCh:
				return
			}
		}
	}()

	// Wait for fetcher and indexer to conclude
	wg.Wait()
	close(stopCh)
	<-uiDone

	// Final Report Generation
	finalSnap := stats.getSnapshot()
	totalDuration := formatDuration(time.Since(startTime))
	var finalReport string

	if atomic.LoadInt32(&p.cancelFlag) == 1 {
		resumeSkip := finalSnap.maxSavedID
		if resumeSkip <= 0 {
			resumeSkip = finalSnap.lastScannedID
		}
		finalReport = fmt.Sprintf(
			"🛑 <b>ɪɴᴅᴇxɪɴɢ ᴏᴘᴇʀᴀᴛɪᴏɴ ᴄᴀɴᴄᴇʟʟᴇᴅ</b> 🛑\n\n"+
				"★ <b>ᴄʜᴀɴɴᴇʟ:</b> <code>%s</code>\n"+
				"★ <b>ᴛᴀʀɢᴇᴛ ᴅʙ:</b> <code>%s</code>\n"+
				"★ <b>ᴛᴀʀɢᴇᴛ ʀᴀɴɢᴇ:</b> <code>%d</code> ᴛᴏ <code>%d</code>\n\n"+
				"--- <b>ꜱᴛᴀᴛꜱ ʙᴇꜰᴏʀᴇ ᴄᴀɴᴄᴇʟʟᴀᴛɪᴏɴ</b> ---\n"+
				"• <b>ᴛᴏᴛᴀʟ ᴍᴇꜱꜱᴀɢᴇꜱ ꜱᴄᴀɴɴᴇᴅ:</b> <code>%d</code>\n"+
				"• <b>ᴍᴇꜱꜱᴀɢᴇꜱ ᴘʀᴏᴄᴇꜱꜱᴇᴅ:</b> <code>%d</code>\n"+
				"• <b>ꜱᴜᴄᴄᴇꜱꜱꜰᴜʟʟʏ ꜱᴀᴠᴇᴅ ꜰɪʟᴇꜱ:</b> <code>%d</code>\n"+
				"• <b>ꜱᴋɪᴘᴘᴇᴅ (ᴅᴜᴘʟɪᴄᴀᴛᴇꜱ):</b> <code>%d</code>\n"+
				"• <b>ꜱᴋɪᴘᴘᴇᴅ (ɴᴏ ᴍᴇᴅɪᴀ):</b> <code>%d</code>\n"+
				"• <b>ᴇʀʀᴏʀꜱ ᴇɴᴄᴏᴜɴᴛᴇʀᴇᴅ:</b> <code>%d</code>\n"+
				"• <b>ᴍᴀx ꜱᴀᴠᴇᴅ ᴍꜱɢ ɪᴅ:</b> <code>%d</code>\n"+
				"• <b>ᴛɪᴍᴇ ᴇʟᴀᴘꜱᴇᴅ:</b> <code>%s</code>\n\n"+
				"💡 <i>You can resume indexing later from where you left off:</i>\n"+
				"<code>/setskip %d</code>",
			channelDisplay, targetDBDisplay, startMsgID, endMsgID,
			finalSnap.fetched, finalSnap.processed, finalSnap.saved, finalSnap.duplicates,
			finalSnap.noMedia, finalSnap.errorsCount, finalSnap.maxSavedID, totalDuration,
			resumeSkip,
		)
	} else {
		finalReport = fmt.Sprintf(
			"✅ <b>ɪɴᴅᴇxɪɴɢ ᴄᴏᴍᴘʟᴇᴛᴇᴅ ꜱᴜᴄᴄᴇꜱꜱꜰᴜʟʟʏ</b> ✅\n\n"+
				"★ <b>ᴄʜᴀɴɴᴇʟ:</b> <code>%s</code>\n"+
				"★ <b>ᴛᴀʀɢᴇᴛ ᴅʙ:</b> <code>%s</code>\n"+
				"★ <b>ᴛᴀʀɢᴇᴛ ʀᴀɴɢᴇ:</b> <code>%d</code> ᴛᴏ <code>%d</code>\n\n"+
				"--- <b>ꜰɪɴᴀʟ ꜱᴛᴀᴛꜱ</b> ---\n"+
				"• <b>ᴛᴏᴛᴀʟ ᴍᴇꜱꜱᴀɢᴇꜱ ꜱᴄᴀɴɴᴇᴅ:</b> <code>%d</code>\n"+
				"• <b>ᴍᴇꜱꜱᴀɢᴇꜱ ᴘʀᴏᴄᴇꜱꜱᴇᴅ:</b> <code>%d</code>\n"+
				"• <b>ꜱᴜᴄᴄᴇꜱꜱꜰᴜʟʟʏ ꜱᴀᴠᴇᴅ ꜰɪʟᴇꜱ:</b> <code>%d</code>\n"+
				"• <b>ꜱᴋɪᴘᴘᴇᴅ (ᴅᴜᴘʟɪᴄᴀᴛᴇꜱ):</b> <code>%d</code>\n"+
				"• <b>ꜱᴋɪᴘᴘᴇᴅ (ɴᴏ ᴍᴇᴅɪᴀ):</b> <code>%d</code>\n"+
				"• <b>ᴇʀʀᴏʀꜱ ᴇɴᴄᴏᴜɴᴛᴇʀᴇᴅ:</b> <code>%d</code>\n"+
				"• <b>ᴍᴀx ꜱᴀᴠᴇᴅ ᴍꜱɢ ɪᴅ:</b> <code>%d</code>\n"+
				"• <b>ᴛᴏᴛᴀʟ ᴛɪᴍᴇ ᴛᴀᴋᴇɴ:</b> <code>%s</code>\n\n"+
				"🎉 <i>All available content in the selected range has been indexed and synced!</i>",
			channelDisplay, targetDBDisplay, startMsgID, endMsgID,
			finalSnap.fetched, finalSnap.processed, finalSnap.saved, finalSnap.duplicates,
			finalSnap.noMedia, finalSnap.errorsCount, finalSnap.maxSavedID, totalDuration,
		)
	}

	if progressMsg != nil {
		_, _ = progressMsg.Edit(finalReport, &telegram.SendOptions{
			ParseMode: "html",
		})
	}

	if p.appCtx != nil && p.appCtx.Client != nil && fromUserID != 0 && (progressMsg == nil || progressMsg.ChatID() != fromUserID) {
		_, _ = p.appCtx.Client.SendMessage(fromUserID, finalReport, &telegram.SendOptions{
			ParseMode: "html",
		})
	}
}

func renderProgressUpdate(channelDisplay, targetDB string, startMsgID, endMsgID, totalMessages int,
	fetched, processed, maxSavedID, saved, duplicates, noMedia, errorsCount int,
	fetchSpeed, indexSpeed float64, remaining int, elapsedStr, timeLeftStr string,
	isFloodWait bool, floodSec int) string {

	floodNotice := ""
	if isFloodWait && floodSec > 0 {
		floodNotice = fmt.Sprintf("\n\n⚠️ <b>Telegram Rate Limit:</b> Paused for <code>%d</code>s before auto-retrying...", floodSec)
	}

	return fmt.Sprintf(
		"📊 <b>ɪɴᴅᴇxɪɴɢ ᴘʀᴏɢʀᴇꜱꜱ ᴜᴘᴅᴀᴛᴇ</b> 📊\n\n"+
			"★ <b>ᴄʜᴀɴɴᴇʟ:</b> <code>%s</code>\n"+
			"★ <b>ᴛᴀʀɢᴇᴛ ᴅʙ:</b> <code>%s</code>\n"+
			"★ <b>ᴛᴀʀɢᴇᴛ ʀᴀɴɢᴇ:</b> <code>%d</code> ᴛᴏ <code>%d</code>\n\n"+
			"★ <b>ᴍᴇꜱꜱᴀɢᴇꜱ ꜰᴇᴛᴄʜᴇᴅ:</b> <code>%d</code> / <code>%d</code>\n"+
			"★ <b>ᴍᴇꜱꜱᴀɢᴇꜱ ᴘʀᴏᴄᴇꜱꜱᴇᴅ:</b> <code>%d</code>\n"+
			"★ <b>ᴍᴀx ꜱᴀᴠᴇᴅ ᴍꜱɢ ɪᴅ (ꜰɪʟᴇ ᴍᴇᴛᴀᴅᴀᴛᴀ, ɴᴏᴛ ᴄʜᴇᴄᴋᴘᴏɪɴᴛ):</b> <code>%d</code>\n\n"+
			"<b>ᴄᴜʀʀᴇɴᴛ ꜱᴛᴀᴛꜱ</b>\n"+
			"✅ <b>ꜱᴀᴠᴇᴅ:</b> <code>%d</code> ꜰɪʟᴇꜱ\n"+
			"⚠️ <b>ᴅᴜᴘʟɪᴄᴀᴛᴇꜱ:</b> <code>%d</code> ꜱᴋɪᴘᴘᴇᴅ\n"+
			"🚫 <b>ɴᴏ ᴍᴇᴅɪᴀ:</b> <code>%d</code> ꜱᴋɪᴘᴘᴇᴅ\n"+
			"❌ <b>ᴇʀʀᴏʀꜱ:</b> <code>%d</code> ᴇɴᴄᴏᴜɴᴛᴇʀᴇᴅ\n\n"+
			"<b>ᴘʀᴏᴄᴇꜱꜱɪɴɢ ꜱᴘᴇᴇᴅ:</b>\n"+
			"├─ <b>ꜰᴇᴛᴄʜ:</b> <code>%.1f</code> ᴍꜱɢ/ꜱ\n"+
			"├─ <b>ɪɴᴅᴇxɪɴɢ:</b> <code>%.1f</code> ꜰɪʟᴇꜱ/ꜱ\n"+
			"└─ <b>ʀᴇᴍᴀɪɴɪɴɢ:</b> <code>%d</code> ᴍᴇꜱꜱᴀɢᴇꜱ\n\n"+
			"⏱️ <b>ᴛɪᴍᴇ ᴇʟᴀᴘꜱᴇᴅ:</b> <code>%s</code>  ⏳ <b>ᴛɪᴍᴇ ʟᴇꜰᴛ:</b> ~<code>%s</code>%s",
		channelDisplay, targetDB, startMsgID, endMsgID,
		fetched, totalMessages,
		processed,
		maxSavedID,
		saved, duplicates, noMedia, errorsCount,
		fetchSpeed, indexSpeed,
		remaining,
		elapsedStr, timeLeftStr, floodNotice,
	)
}

func safeEditProgress(msg *telegram.NewMessage, text string, kb telegram.ReplyMarkup) {
	if msg == nil {
		return
	}
	_, _ = msg.Edit(text, &telegram.SendOptions{
		ParseMode:   "html",
		ReplyMarkup: kb,
	})
}

func isFloodWaitError(err error) bool {
	if err == nil {
		return false
	}
	if telegram.MatchError(err, "FLOOD_WAIT_") || telegram.MatchError(err, "FLOOD_PREMIUM_WAIT_") {
		return true
	}
	errStr := strings.ToUpper(err.Error())
	return strings.Contains(errStr, "FLOOD_WAIT") ||
		strings.Contains(errStr, "FLOOD_PREMIUM_WAIT") ||
		strings.Contains(errStr, "420") ||
		strings.Contains(errStr, "WAIT OF") ||
		strings.Contains(errStr, "WAIT ")
}

func extractFloodWaitSeconds(err error) int {
	if err == nil {
		return 0
	}
	if wait := telegram.GetFloodWait(err); wait > 0 {
		return wait
	}
	re := regexp.MustCompile(`FLOOD(?:_PREMIUM)?_WAIT_(\d+)`)
	if matches := re.FindStringSubmatch(err.Error()); len(matches) > 1 {
		if val, pErr := strconv.Atoi(matches[1]); pErr == nil && val > 0 {
			return val
		}
	}
	reSec := regexp.MustCompile(`(?i)wait (?:of )?(\d+) seconds`)
	if matches := reSec.FindStringSubmatch(err.Error()); len(matches) > 1 {
		if val, pErr := strconv.Atoi(matches[1]); pErr == nil && val > 0 {
			return val
		}
	}
	return 0
}

func formatChannelDisplay(chatIDStr string) string {
	cleanID := chatIDStr
	if strings.HasPrefix(cleanID, "-100") {
		cleanID = strings.TrimPrefix(cleanID, "-100")
	}
	if isNumeric(cleanID) {
		return fmt.Sprintf("ᴘʀɪᴠᴀᴛᴇ ᴄʜᴀɴɴᴇʟ %s", cleanID)
	}
	if !strings.HasPrefix(cleanID, "@") {
		return "@" + cleanID
	}
	return cleanID
}

func formatTargetDB(dbType string) string {
	switch dbType {
	case "accept5", "link5":
		return "ᴅɪꜱᴛʀɪʙᴜᴛᴇ (ᴀʟᴛᴇʀɴᴀᴛᴇ →DB2↔DB3)"
	case "accept1", "link1":
		return "DB2 (Media Shard 1)"
	case "accept2", "link2":
		return "DB3 (Media Shard 2)"
	default:
		return "ᴅɪꜱᴛʀɪʙᴜᴛᴇ (ᴀʟᴛᴇʀɴᴀᴛᴇ →DB2↔DB3)"
	}
}

func formatETA(d time.Duration) string {
	if d <= 0 {
		return "0ꜱ"
	}
	totalSec := int64(d.Seconds())
	hours := totalSec / 3600
	minutes := (totalSec % 3600) / 60
	seconds := totalSec % 60

	if hours > 0 {
		return fmt.Sprintf("%dʜ %dᴍ", hours, minutes)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dᴍ %dꜱ", minutes, seconds)
	}
	return fmt.Sprintf("%dꜱ", seconds)
}

func formatCompactDuration(d time.Duration) string {
	totalSec := int64(d.Seconds())
	if totalSec < 0 {
		totalSec = 0
	}
	days := totalSec / 86400
	hours := (totalSec % 86400) / 3600
	minutes := (totalSec % 3600) / 60
	seconds := totalSec % 60

	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if seconds > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%ds", seconds))
	}
	return strings.Join(parts, "")
}

func extractChannelInfo(m *telegram.NewMessage, target *telegram.NewMessage) (string, int) {
	// 1. Check message text and caption for telegram links
	for _, text := range []string{m.Text(), m.MessageText(), target.Text(), target.MessageText()} {
		if text == "" {
			continue
		}
		if match := linkRegex.FindStringSubmatch(text); len(match) == 3 {
			channelPart := match[1]
			msgID, _ := strconv.Atoi(match[2])
			if msgID > 0 {
				if strings.Contains(text, "/c/") || isNumeric(channelPart) {
					norm := config.NormalizeChannelID(parseInt64(channelPart))
					return fmt.Sprintf("%d", norm), msgID
				}
				return channelPart, msgID
			}
		}
	}

	// 2. Check forwarded info
	if target.Message != nil && target.Message.FwdFrom != nil {
		fwd := target.Message.FwdFrom
		var lastID int
		if fwd.ChannelPost != 0 {
			lastID = int(fwd.ChannelPost)
		} else {
			lastID = int(target.ID)
		}

		if peerCh, ok := fwd.FromID.(*telegram.PeerChannel); ok && peerCh.ChannelID != 0 {
			norm := config.NormalizeChannelID(peerCh.ChannelID)
			return fmt.Sprintf("%d", norm), lastID
		}
		if peerCh, ok := fwd.SavedFromPeer.(*telegram.PeerChannel); ok && peerCh.ChannelID != 0 {
			norm := config.NormalizeChannelID(peerCh.ChannelID)
			return fmt.Sprintf("%d", norm), lastID
		}
		if peerCh, ok := fwd.SavedFromID.(*telegram.PeerChannel); ok && peerCh.ChannelID != 0 {
			norm := config.NormalizeChannelID(peerCh.ChannelID)
			return fmt.Sprintf("%d", norm), lastID
		}
	}

	// 3. Fallback: executed inside a channel
	if m.IsChannel() && m.ChatID() != 0 {
		return fmt.Sprintf("%d", m.ChatID()), int(m.ID)
	}

	return "", 0
}

func isNumeric(s string) bool {
	_, err := strconv.ParseInt(s, 10, 64)
	return err == nil
}

func parseInt64(s string) int64 {
	val, _ := strconv.ParseInt(s, 10, 64)
	return val
}

func formatDuration(d time.Duration) string {
	seconds := int64(d.Seconds())
	if seconds < 0 {
		seconds = 0
	}
	days := seconds / 86400
	hours := (seconds % 86400) / 3600
	minutes := (seconds % 3600) / 60
	secs := seconds % 60

	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if secs > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%ds", secs))
	}
	return strings.Join(parts, " ")
}
