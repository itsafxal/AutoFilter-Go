package fsub

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/MYTGBOTZ/internal/config"
)

// Service coordinates Force Subscribe configuration, caching, join request events,
// and dynamic seamless subscription polling matching legacy bot behavior.
type Service struct {
	repo        *Repository
	cache       *MembershipCache
	taskTracker *TaskTracker
	cfg         *config.Config
	logger      *slog.Logger

	botID int64

	mu       sync.RWMutex
	loadout  Loadout
	link1    string
	link2    string
	link3    string
	link1_2  string
	link2_2  string
	link3_2  string
}

// NewService creates a new FSub Service instance.
func NewService(repo *Repository, cfg *config.Config, logger *slog.Logger) *Service {
	return &Service{
		repo:        repo,
		cache:       NewMembershipCache(60*time.Second, 8*time.Second),
		taskTracker: NewTaskTracker(),
		cfg:         cfg,
		logger:      logger,
	}
}

// Init loads the loadout configuration from MongoDB and generates/caches invite links.
func (s *Service) Init(ctx context.Context, client *telegram.Client, botID int64) error {
	s.botID = botID
	if s.repo == nil {
		return nil
	}

	loadout, err := s.repo.GetLoadout(ctx, botID)
	if err != nil {
		s.logger.Error("failed to load FSub loadout from MongoDB", "error", err)
		return err
	}

	s.mu.Lock()
	s.loadout = *loadout
	s.mu.Unlock()

	s.logger.Info("loaded FSub loadout from database",
		"channel1", loadout.Channel1,
		"channel2", loadout.Channel2,
		"channel3", loadout.Channel3,
		"channel1_2", loadout.Channel1_2,
		"channel2_2", loadout.Channel2_2,
		"channel3_2", loadout.Channel3_2,
		"threshold1", loadout.Channel1Threshold,
		"threshold2", loadout.Channel2Threshold,
		"threshold3", loadout.Channel3Threshold,
	)

	// Pre-generate invite links if client is ready
	if client != nil {
		s.RefreshInviteLinks(client)
	}

	return nil
}

// RefreshInviteLinks generates or refreshes invite links for all configured channels.
func (s *Service) RefreshInviteLinks(client *telegram.Client) {
	if client == nil {
		return
	}

	s.mu.RLock()
	ch1 := s.loadout.Channel1
	ch2 := s.loadout.Channel2
	ch3 := s.loadout.Channel3
	ch1_2 := s.loadout.Channel1_2
	ch2_2 := s.loadout.Channel2_2
	ch3_2 := s.loadout.Channel3_2
	s.mu.RUnlock()

	channels := []struct {
		id    int64
		dest  *string
		label string
	}{
		{ch1, &s.link1, "channel1"},
		{ch2, &s.link2, "channel2"},
		{ch3, &s.link3, "channel3"},
		{ch1_2, &s.link1_2, "channel1_2"},
		{ch2_2, &s.link2_2, "channel2_2"},
		{ch3_2, &s.link3_2, "channel3_2"},
	}

	for _, item := range channels {
		if item.id == 0 {
			continue
		}
		link, err := s.generateInviteLink(client, item.id)
		if err != nil {
			s.logger.Warn("failed to create join request invite link", "channel_id", item.id, "label", item.label, "error", err)
			continue
		}
		s.mu.Lock()
		*item.dest = link
		s.mu.Unlock()
		s.logger.Info("generated join request invite link", "channel_id", item.id, "label", item.label, "link", link)
	}
}

func (s *Service) generateInviteLink(client *telegram.Client, chatID int64) (string, error) {
	linkObj, err := client.GetChatInviteLink(chatID, &telegram.InviteLinkOptions{
		RequestNeeded: true,
	})
	if err != nil {
		return "", err
	}
	if exp, ok := linkObj.(*telegram.ChatInviteExported); ok {
		return exp.Link, nil
	}
	return "", fmt.Errorf("unexpected invite link type: %T", linkObj)
}

// CheckChannelMembership checks whether a user is an active participant or has an approved/pending join request.
func (s *Service) CheckChannelMembership(ctx context.Context, client *telegram.Client, userID int64, channelID int64, slot int) bool {
	if userID == 0 || channelID == 0 {
		return true
	}
	if s.cfg.IsAdmin(userID) || s.cfg.IsOwner(userID) {
		return true
	}

	// 1. Check in-memory short-lived TTL cache
	if isMember, found := s.cache.Get(userID, channelID); found {
		return isMember
	}

	// 2. Check MongoDB join-request tables
	if slot > 0 && s.repo != nil {
		if hasReq, err := s.repo.HasReq(ctx, slot, userID); err == nil && hasReq {
			s.cache.Set(userID, channelID, true)
			return true
		}
	}

	// 3. Fallback to Telegram GetChatMember
	if client != nil {
		participant, err := client.GetChatMember(channelID, userID)
		if err == nil && participant != nil {
			isJoined := participant.Status != telegram.Left && participant.Status != telegram.Restricted
			s.cache.Set(userID, channelID, isJoined)
			return isJoined
		}
		s.cache.Set(userID, channelID, false)
		return false
	}

	return false
}

// BuildDynamicFSubKeyboard constructs the Force Subscribe keyboard dynamically.
// Hides buttons for channels the user has already joined.
// - Concurrently checks Channel 1 and Channel 2.
// - If Channel 1 and Channel 2 are BOTH joined, checks Channel 3.
// Returns (replyMarkup, unjoinedChannels).
func (s *Service) BuildDynamicFSubKeyboard(ctx context.Context, client *telegram.Client, userID int64, fileID string) (*telegram.ReplyInlineMarkup, []int64) {
	if s.cfg.IsAdmin(userID) || s.cfg.IsOwner(userID) {
		return nil, nil
	}

	s.mu.RLock()
	ch1 := s.loadout.Channel1
	ch2 := s.loadout.Channel2
	ch3 := s.loadout.Channel3
	l1 := s.link1
	l2 := s.link2
	l3 := s.link3
	s.mu.RUnlock()

	var unjoined []int64
	var buttons [][]telegram.KeyboardButton

	// 1. Parallelize Channel 1 & Channel 2 checks
	var ch1Joined, ch2Joined bool
	var wg sync.WaitGroup

	if ch1 != 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch1Joined = s.CheckChannelMembership(ctx, client, userID, ch1, 1)
		}()
	} else {
		ch1Joined = true
	}

	if ch2 != 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch2Joined = s.CheckChannelMembership(ctx, client, userID, ch2, 2)
		}()
	} else {
		ch2Joined = true
	}

	wg.Wait()

	if ch1 != 0 && !ch1Joined {
		unjoined = append(unjoined, ch1)
		link := l1
		if link == "" && client != nil {
			link, _ = s.generateInviteLink(client, ch1)
		}
		buttons = append(buttons, []telegram.KeyboardButton{
			telegram.Button.URL("💐 Rᴇǫᴜᴇsᴛ Tᴏ Jᴏɪɴ Cʜᴀɴɴᴇʟ 1 💐", link),
		})
	}

	if ch2 != 0 && !ch2Joined {
		unjoined = append(unjoined, ch2)
		link := l2
		if link == "" && client != nil {
			link, _ = s.generateInviteLink(client, ch2)
		}
		buttons = append(buttons, []telegram.KeyboardButton{
			telegram.Button.URL("⚡ RᴇǫᴜESᴛ Tᴏ Jᴏɪɴ Cʜᴀɴɴᴇʟ 2 ⚡", link),
		})
	}

	// 2. Check Channel 3 only if Channel 1 and Channel 2 are BOTH joined
	if ch1Joined && ch2Joined && ch3 != 0 {
		ch3Joined := s.CheckChannelMembership(ctx, client, userID, ch3, 3)
		if !ch3Joined {
			unjoined = append(unjoined, ch3)
			link := l3
			if link == "" && client != nil {
				link, _ = s.generateInviteLink(client, ch3)
			}
			buttons = append(buttons, []telegram.KeyboardButton{
				telegram.Button.URL("♦️ RᴇǫᴜESᴛ Tᴏ Jᴏɪɴ Cʜᴀɴɴᴇʟ 3 ♦️", link),
			})
		}
	}

	if len(buttons) == 0 {
		return nil, nil
	}

	builder := telegram.NewKeyboard()
	for _, row := range buttons {
		builder.AddRow(row...)
	}
	markup := builder.Build()
	return markup, unjoined
}

// CheckLoopSubDynamic runs the background verification loop.
// Updates promptMsg reply markup dynamically only when channel status changes.
// Returns true when all channels are joined (or join requests sent), eliminating the need for a manual "Try Again" button.
func (s *Service) CheckLoopSubDynamic(
	parentCtx context.Context,
	client *telegram.Client,
	userID int64,
	chatID int64,
	promptMsgID int32,
	fileID string,
) bool {
	loopCtx, cancel := s.taskTracker.Acquire(parentCtx, userID)
	defer s.taskTracker.Release(userID)
	defer cancel()

	var lastSig string
	maxCount := 10 // 10 iterations * 2s = 20s total

	for count := 0; count < maxCount; count++ {
		select {
		case <-parentCtx.Done():
			return false
		case <-loopCtx.Done():
			return false
		default:
		}

		markup, unjoined := s.BuildDynamicFSubKeyboard(loopCtx, client, userID, fileID)
		if len(unjoined) == 0 {
			return true
		}

		sig := fmt.Sprint(unjoined)
		if sig != lastSig {
			lastSig = sig
			if promptMsgID != 0 && client != nil {
				_, _ = client.EditMessage(chatID, promptMsgID, "<b>Pʟᴇᴀꜱᴇ Jᴏɪɴ Oᴜʀ Cʜᴀɴɴᴇʟs Tᴏ Gᴇᴛ Yᴏᴜʀ Rᴇǫᴜᴇꜱᴛᴇᴅ Mᴏᴠɪᴇ Fɪʟᴇ 🚀</b>", &telegram.SendOptions{
					ParseMode:   "html",
					ReplyMarkup: markup,
				})
			}
		}

		select {
		case <-parentCtx.Done():
			return false
		case <-loopCtx.Done():
			return false
		case <-time.After(2 * time.Second):
		}
	}

	return false
}

// HandleJoinRequest processes an incoming join request update from Gogram.
func (s *Service) HandleJoinRequest(client *telegram.Client, req *telegram.JoinRequestUpdate) error {
	if req == nil {
		return nil
	}

	chatID := getJoinRequestChatID(req)
	userIDs := getJoinRequestUserIDs(req)

	if chatID == 0 || len(userIDs) == 0 {
		return nil
	}

	s.mu.RLock()
	ch1 := s.loadout.Channel1
	ch2 := s.loadout.Channel2
	ch3 := s.loadout.Channel3
	s.mu.RUnlock()

	for _, uid := range userIDs {
		// Update short-lived cache immediately
		s.cache.Set(uid, chatID, true)

		if chatID == ch1 {
			if s.repo != nil {
				_ = s.repo.AddReq(context.Background(), 1, uid)
			}
			go s.CheckAndSwitchChannel(client, 1)
		} else if chatID == ch2 {
			if s.repo != nil {
				_ = s.repo.AddReq(context.Background(), 2, uid)
			}
			go s.CheckAndSwitchChannel(client, 2)
		} else if chatID == ch3 {
			if s.repo != nil {
				_ = s.repo.AddReq(context.Background(), 3, uid)
			}
			go s.CheckAndSwitchChannel(client, 3)
		}
	}

	return nil
}

func getJoinRequestChatID(req *telegram.JoinRequestUpdate) int64 {
	cid := req.ChannelID()
	if cid != 0 {
		return cid
	}
	if req.BotOriginalUpdate != nil && req.BotOriginalUpdate.Peer != nil {
		switch p := req.BotOriginalUpdate.Peer.(type) {
		case *telegram.PeerChannel:
			return -100_000_000_0000 - p.ChannelID
		case *telegram.PeerChat:
			return -p.ChatID
		}
	}
	return 0
}

func getJoinRequestUserIDs(req *telegram.JoinRequestUpdate) []int64 {
	uids := req.UserIDs()
	if len(uids) > 0 {
		return uids
	}
	if req.BotOriginalUpdate != nil && req.BotOriginalUpdate.UserID != 0 {
		return []int64{req.BotOriginalUpdate.UserID}
	}
	return nil
}

// CheckAndSwitchChannel checks if request count exceeds threshold and performs channel switching.
func (s *Service) CheckAndSwitchChannel(client *telegram.Client, slot int) {
	if s.repo == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := context.Background()

	switch slot {
	case 1:
		count, _ := s.repo.GetReqCount(ctx, 1)
		threshold := s.loadout.Channel1Threshold
		secondary := s.loadout.Channel1_2

		if threshold > 0 && int(count) > threshold && secondary != 0 {
			s.loadout.Channel1 = secondary
			s.loadout.Channel1_2 = 0
			s.link1 = s.link1_2
			s.link1_2 = ""
			_ = s.repo.DeleteAllReq(ctx, 1)
			_ = s.repo.UpdateLoadout(ctx, s.botID, "channel1", secondary)
			_ = s.repo.UpdateLoadout(ctx, s.botID, "channel1_2", nil)
			s.logger.Info("switched channel 1 to secondary channel due to threshold", "count", count, "threshold", threshold)
			return
		}
		if threshold > 0 && int(count) > threshold {
			s.loadout.Channel1 = 0
			s.link1 = ""
			_ = s.repo.DeleteAllReq(ctx, 1)
			_ = s.repo.UpdateLoadout(ctx, s.botID, "channel1", nil)
			s.logger.Info("cleared channel 1 due to threshold exceeded with no secondary", "count", count, "threshold", threshold)
		}

	case 2:
		count, _ := s.repo.GetReqCount(ctx, 2)
		threshold := s.loadout.Channel2Threshold
		secondary := s.loadout.Channel2_2

		if threshold > 0 && int(count) > threshold && secondary != 0 {
			s.loadout.Channel2 = secondary
			s.loadout.Channel2_2 = 0
			s.link2 = s.link2_2
			s.link2_2 = ""
			_ = s.repo.DeleteAllReq(ctx, 2)
			_ = s.repo.UpdateLoadout(ctx, s.botID, "channel2", secondary)
			_ = s.repo.UpdateLoadout(ctx, s.botID, "channel2_2", nil)
			s.logger.Info("switched channel 2 to secondary channel due to threshold", "count", count, "threshold", threshold)
			return
		}
		if threshold > 0 && int(count) > threshold {
			s.loadout.Channel2 = 0
			s.link2 = ""
			_ = s.repo.DeleteAllReq(ctx, 2)
			_ = s.repo.UpdateLoadout(ctx, s.botID, "channel2", nil)
			s.logger.Info("cleared channel 2 due to threshold exceeded with no secondary", "count", count, "threshold", threshold)
		}

	case 3:
		count, _ := s.repo.GetReqCount(ctx, 3)
		threshold := s.loadout.Channel3Threshold
		secondary := s.loadout.Channel3_2

		if threshold > 0 && int(count) > threshold && secondary != 0 {
			s.loadout.Channel3 = secondary
			s.loadout.Channel3_2 = 0
			s.link3 = s.link3_2
			s.link3_2 = ""
			_ = s.repo.DeleteAllReq(ctx, 3)
			_ = s.repo.UpdateLoadout(ctx, s.botID, "channel3", secondary)
			_ = s.repo.UpdateLoadout(ctx, s.botID, "channel3_2", nil)
			s.logger.Info("switched channel 3 to secondary channel due to threshold", "count", count, "threshold", threshold)
			return
		}
		if threshold > 0 && int(count) > threshold {
			s.loadout.Channel3 = 0
			s.link3 = ""
			_ = s.repo.DeleteAllReq(ctx, 3)
			_ = s.repo.UpdateLoadout(ctx, s.botID, "channel3", nil)
			s.logger.Info("cleared channel 3 due to threshold exceeded with no secondary", "count", count, "threshold", threshold)
		}
	}
}

// SetChannel configures a primary Force Subscribe channel for a slot (1, 2, or 3).
func (s *Service) SetChannel(ctx context.Context, client *telegram.Client, slot int, chatID int64) (string, string, error) {
	title, link, err := s.verifyAndGenerateLink(client, chatID)
	if err != nil {
		return "", "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := fmt.Sprintf("channel%d", slot)
	switch slot {
	case 1:
		s.loadout.Channel1 = chatID
		s.link1 = link
	case 2:
		s.loadout.Channel2 = chatID
		s.link2 = link
	case 3:
		s.loadout.Channel3 = chatID
		s.link3 = link
	default:
		return "", "", fmt.Errorf("invalid slot: %d", slot)
	}

	if s.repo != nil {
		_ = s.repo.UpdateLoadout(ctx, s.botID, key, chatID)
		_ = s.repo.DeleteAllReq(ctx, slot)
	}

	// Automatically update channel description matching bot requirements
	_ = s.UpdateChannelDescription(client, chatID, "")

	return title, link, nil
}

// SetSecondaryChannel configures a secondary channel for threshold switching.
func (s *Service) SetSecondaryChannel(ctx context.Context, client *telegram.Client, slot int, chatID int64) (string, string, error) {
	title, link, err := s.verifyAndGenerateLink(client, chatID)
	if err != nil {
		return "", "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := fmt.Sprintf("channel%d_2", slot)
	switch slot {
	case 1:
		s.loadout.Channel1_2 = chatID
		s.link1_2 = link
	case 2:
		s.loadout.Channel2_2 = chatID
		s.link2_2 = link
	case 3:
		s.loadout.Channel3_2 = chatID
		s.link3_2 = link
	default:
		return "", "", fmt.Errorf("invalid slot: %d", slot)
	}

	if s.repo != nil {
		_ = s.repo.UpdateLoadout(ctx, s.botID, key, chatID)
	}

	// Automatically update channel description matching bot requirements
	_ = s.UpdateChannelDescription(client, chatID, "")

	return title, link, nil
}

// SetCountThreshold configures the join request count threshold for a slot.
func (s *Service) SetCountThreshold(ctx context.Context, slot int, threshold int) error {
	if threshold <= 0 {
		return fmt.Errorf("count must be a positive number")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := fmt.Sprintf("channel%d", slot)
	switch slot {
	case 1:
		s.loadout.Channel1Threshold = threshold
	case 2:
		s.loadout.Channel2Threshold = threshold
	case 3:
		s.loadout.Channel3Threshold = threshold
	default:
		return fmt.Errorf("invalid slot: %d", slot)
	}

	if s.repo != nil {
		_ = s.repo.UpdateThreshold(ctx, key, threshold)
	}

	return nil
}

// DeleteChannel removes a primary Force Subscribe channel from a slot.
func (s *Service) DeleteChannel(ctx context.Context, slot int) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var oldID int64
	key := fmt.Sprintf("channel%d", slot)
	switch slot {
	case 1:
		oldID = s.loadout.Channel1
		s.loadout.Channel1 = 0
		s.link1 = ""
	case 2:
		oldID = s.loadout.Channel2
		s.loadout.Channel2 = 0
		s.link2 = ""
	case 3:
		oldID = s.loadout.Channel3
		s.loadout.Channel3 = 0
		s.link3 = ""
	default:
		return 0, fmt.Errorf("invalid slot: %d", slot)
	}

	if oldID == 0 {
		return 0, fmt.Errorf("no force subscribe channel set for slot %d to delete", slot)
	}

	if s.repo != nil {
		_ = s.repo.UpdateLoadout(ctx, s.botID, key, nil)
		_ = s.repo.DeleteAllReq(ctx, slot)
	}

	return oldID, nil
}

// DeleteSecondaryChannel removes a secondary Force Subscribe channel from a slot.
func (s *Service) DeleteSecondaryChannel(ctx context.Context, slot int) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var oldID int64
	key := fmt.Sprintf("channel%d_2", slot)
	switch slot {
	case 1:
		oldID = s.loadout.Channel1_2
		s.loadout.Channel1_2 = 0
		s.link1_2 = ""
	case 2:
		oldID = s.loadout.Channel2_2
		s.loadout.Channel2_2 = 0
		s.link2_2 = ""
	case 3:
		oldID = s.loadout.Channel3_2
		s.loadout.Channel3_2 = 0
		s.link3_2 = ""
	default:
		return 0, fmt.Errorf("invalid slot: %d", slot)
	}

	if oldID == 0 {
		return 0, fmt.Errorf("no secondary force subscribe channel set for slot %d to delete", slot)
	}

	if s.repo != nil {
		_ = s.repo.UpdateLoadout(ctx, s.botID, key, nil)
	}

	return oldID, nil
}

func (s *Service) verifyAndGenerateLink(client *telegram.Client, chatID int64) (string, string, error) {
	if client == nil {
		return "", "", fmt.Errorf("telegram client unavailable")
	}

	// Resolve peer and fetch title
	peer, err := client.ResolvePeer(chatID)
	if err != nil {
		return "", "", fmt.Errorf("cannot access chat: %w", err)
	}

	title := fmt.Sprintf("Chat %d", chatID)
	if ch, ok := peer.(*telegram.InputPeerChannel); ok {
		if channel, err := client.GetChannel(ch.ChannelID); err == nil && channel != nil {
			title = channel.Title
		}
	}

	// Create invite link with join request requirement enabled
	link, err := s.generateInviteLink(client, chatID)
	if err != nil {
		return title, "", fmt.Errorf("error creating invite link: %w", err)
	}

	return title, link, nil
}

// GetStatus returns the formatted status overview matching /view_sub in TGxCyberBot.
func (s *Service) GetStatus(client *telegram.Client) (string, error) {
	s.mu.RLock()
	ch1 := s.loadout.Channel1
	ch2 := s.loadout.Channel2
	ch3 := s.loadout.Channel3
	ch1_2 := s.loadout.Channel1_2
	ch2_2 := s.loadout.Channel2_2
	ch3_2 := s.loadout.Channel3_2
	t1 := s.loadout.Channel1Threshold
	t2 := s.loadout.Channel2Threshold
	t3 := s.loadout.Channel3Threshold
	l1 := s.link1
	l2 := s.link2
	l3 := s.link3
	s.mu.RUnlock()

	ctx := context.Background()
	text := "📊 <b>Force Subscribe Channels Status</b> 📊\n\n"

	// Slot 1
	if ch1 != 0 {
		title1 := s.getChatTitle(client, ch1)
		link1 := l1
		if link1 == "" {
			link1 = "N/A (No invite link available)"
		}
		var count1 int64
		if s.repo != nil {
			count1, _ = s.repo.GetReqCount(ctx, 1)
		}
		threshStr := "Not Set"
		if t1 > 0 {
			threshStr = strconv.Itoa(t1)
		}
		secStr := "Not Set"
		if ch1_2 != 0 {
			secStr = strconv.FormatInt(ch1_2, 10)
		}

		text += fmt.Sprintf(
			"<b>─── Channel 1 ───</b>\n"+
				"  <b>🆔 Chat ID:</b> <code>%d</code>\n"+
				"  <b>📝 Chat Name:</b> <code>%s</code>\n"+
				"  <b>🔗 Invite Link:</b> <a href=\"%s\">Click Here</a>\n"+
				"  <b>👥 Total Requests:</b> <code>%d</code>\n"+
				"  <b>📊 Count Threshold:</b> <code>%s</code>\n"+
				"  <b>🔄 Secondary Channel:</b> <code>%s</code>\n\n",
			ch1, title1, link1, count1, threshStr, secStr,
		)
	} else {
		text += "<b>─── Channel 1 ───</b>\n  <i>No Force Subscribe channel set for slot 1.</i>\n\n"
	}

	// Slot 2
	if ch2 != 0 {
		title2 := s.getChatTitle(client, ch2)
		link2 := l2
		if link2 == "" {
			link2 = "N/A (No invite link available)"
		}
		var count2 int64
		if s.repo != nil {
			count2, _ = s.repo.GetReqCount(ctx, 2)
		}
		threshStr := "Not Set"
		if t2 > 0 {
			threshStr = strconv.Itoa(t2)
		}
		secStr := "Not Set"
		if ch2_2 != 0 {
			secStr = strconv.FormatInt(ch2_2, 10)
		}

		text += fmt.Sprintf(
			"<b>─── Channel 2 ───</b>\n"+
				"  <b>🆔 Chat ID:</b> <code>%d</code>\n"+
				"  <b>📝 Chat Name:</b> <code>%s</code>\n"+
				"  <b>🔗 Invite Link:</b> <a href=\"%s\">Click Here</a>\n"+
				"  <b>👥 Total Requests:</b> <code>%d</code>\n"+
				"  <b>📊 Count Threshold:</b> <code>%s</code>\n"+
				"  <b>🔄 Secondary Channel:</b> <code>%s</code>\n\n",
			ch2, title2, link2, count2, threshStr, secStr,
		)
	} else {
		text += "<b>─── Channel 2 ───</b>\n  <i>No Force Subscribe channel set for slot 2.</i>\n\n"
	}

	// Slot 3
	if ch3 != 0 {
		title3 := s.getChatTitle(client, ch3)
		link3 := l3
		if link3 == "" {
			link3 = "N/A (No invite link available)"
		}
		var count3 int64
		if s.repo != nil {
			count3, _ = s.repo.GetReqCount(ctx, 3)
		}
		threshStr := "Not Set"
		if t3 > 0 {
			threshStr = strconv.Itoa(t3)
		}
		secStr := "Not Set"
		if ch3_2 != 0 {
			secStr = strconv.FormatInt(ch3_2, 10)
		}

		text += fmt.Sprintf(
			"<b>─── Channel 3 ───</b>\n"+
				"  <b>🆔 Chat ID:</b> <code>%d</code>\n"+
				"  <b>📝 Chat Name:</b> <code>%s</code>\n"+
				"  <b>🔗 Invite Link:</b> <a href=\"%s\">Click Here</a>\n"+
				"  <b>👥 Total Requests:</b> <code>%d</code>\n"+
				"  <b>📊 Count Threshold:</b> <code>%s</code>\n"+
				"  <b>🔄 Secondary Channel:</b> <code>%s</code>",
			ch3, title3, link3, count3, threshStr, secStr,
		)
	} else {
		text += "<b>─── Channel 3 ───</b>\n  <i>No Force Subscribe channel set for slot 3.</i>"
	}

	return text, nil
}

func (s *Service) getChatTitle(client *telegram.Client, chatID int64) string {
	if client == nil {
		return fmt.Sprintf("Chat %d", chatID)
	}
	peer, err := client.ResolvePeer(chatID)
	if err == nil {
		if ch, ok := peer.(*telegram.InputPeerChannel); ok {
			if channel, err := client.GetChannel(ch.ChannelID); err == nil && channel != nil {
				return channel.Title
			}
		}
	}
	return fmt.Sprintf("Chat %d", chatID)
}

// GetDefaultChannelDescription returns the configured or default about/description text for FSub channels.
func (s *Service) GetDefaultChannelDescription() string {
	if s.cfg != nil && strings.TrimSpace(s.cfg.FSubChannelDescription) != "" {
		return s.cfg.FSubChannelDescription
	}
	return "You need to request to join channel to get file 🚀"
}

// UpdateChannelDescription updates the channel's about/description text in Telegram.
func (s *Service) UpdateChannelDescription(client *telegram.Client, chatID int64, description string) error {
	if client == nil {
		return fmt.Errorf("telegram client unavailable")
	}
	if strings.TrimSpace(description) == "" {
		description = s.GetDefaultChannelDescription()
	}

	peer, err := client.ResolvePeer(chatID)
	if err != nil {
		return fmt.Errorf("cannot resolve peer for description update: %w", err)
	}

	_, err = client.MessagesEditChatAbout(peer, description)
	if err != nil {
		if strings.Contains(strings.ToUpper(err.Error()), "CHAT_NOT_MODIFIED") {
			return nil
		}
		s.logger.Warn("failed to update channel description", "chat_id", chatID, "error", err)
		return err
	}

	s.logger.Info("successfully updated channel description", "chat_id", chatID, "description", description)
	return nil
}

