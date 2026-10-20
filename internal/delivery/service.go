package delivery

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/MYTGBOTZ/internal/commands"
	"github.com/itsafxal/MYTGBOTZ/internal/database"
	"github.com/itsafxal/MYTGBOTZ/internal/database/sql"
)

var validIdentifierRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

// CaptionFormatter defines dynamic formatting operations for media captions.
type CaptionFormatter interface {
	FormatMedia(ctx context.Context, fileName, sizeStr, originalCaption string) string
	Format(template, fileName, sizeStr, originalCaption string) string
}

// MediaDeliveryService defines operations for validating, resolving, and delivering
// media records stored in MongoDB / SQLite cache via Telegram Bot API.
type MediaDeliveryService interface {
	// GetMedia resolves an authoritative media record by its unique identifier.
	GetMedia(ctx context.Context, identifier string) (*MediaDetails, error)

	// ValidateMediaRecord ensures a resolved media record possesses valid delivery attributes.
	ValidateMediaRecord(record *MediaDetails) error

	// Deliver sends the requested media to the destination chat using the appropriate Telegram send method.
	Deliver(ctx context.Context, chatID int64, identifier string, opts DeliveryOptions) (*DeliveryResult, error)

	// SetSender assigns the Telegram sender client (typically *telegram.Client).
	SetSender(sender TelegramSender)

	// SetCaptionFormatter assigns the dynamic caption formatter.
	SetCaptionFormatter(cf CaptionFormatter)

	// SetAdminConfig configures administrative targets for operations like file reference refreshing.
	SetAdminConfig(ownerID int64, admins []int64)

	// RefreshFileRef re-fetches a fresh Telegram file reference by sending the media silently and immediately deleting it.
	RefreshFileRef(ctx context.Context, fileID string) (any, error)

	// SaveMedia stores or updates a media record in the database for on-demand link generation.
	SaveMedia(ctx context.Context, media *MediaDetails) error
}

// Service implements MediaDeliveryService.
type Service struct {
	db               *database.DatabaseManager
	sender           TelegramSender
	senderMu         sync.RWMutex
	logger             *slog.Logger
	inFlight           *InFlightTracker
	captionFormatter   CaptionFormatter
	streamURL          string
	enableFastDownload bool
	ownerID            int64
	admins             []int64
}

// NewService constructs a new media delivery service.
func NewService(db *database.DatabaseManager, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		db:                 db,
		logger:             logger,
		inFlight:           NewInFlightTracker(30 * time.Second),
		enableFastDownload: true,
	}
}

// SetSender sets the active Telegram sender client.
func (s *Service) SetSender(sender TelegramSender) {
	s.senderMu.Lock()
	defer s.senderMu.Unlock()
	s.sender = sender
}

// SetCaptionFormatter sets the dynamic media caption formatter.
func (s *Service) SetCaptionFormatter(cf CaptionFormatter) {
	s.captionFormatter = cf
}

// SetStreamConfig configures the stream base URL and button enablement.
func (s *Service) SetStreamConfig(streamURL string, enableFastDownload bool) {
	s.streamURL = strings.TrimRight(strings.TrimSpace(streamURL), "/")
	s.enableFastDownload = enableFastDownload
}

// SetAdminConfig configures administrative targets for operations like file reference refreshing.
func (s *Service) SetAdminConfig(ownerID int64, admins []int64) {
	s.ownerID = ownerID
	s.admins = admins
}


// getSender safely retrieves the current Telegram sender client.
func (s *Service) getSender() TelegramSender {
	s.senderMu.RLock()
	defer s.senderMu.RUnlock()
	return s.sender
}

// GetMedia retrieves media metadata from the runtime SQLite cache, falling back to
// authoritative MongoDB if necessary.
func (s *Service) GetMedia(ctx context.Context, identifier string) (*MediaDetails, error) {
	cleanID := strings.TrimSpace(identifier)
	if !validIdentifierRegex.MatchString(cleanID) {
		return nil, ErrInvalidIdentifier
	}

	if s.db == nil {
		return nil, ErrDatabaseUnavailable
	}

	s.logger.Debug("media lookup started", "identifier_prefix", safeIdentifierPrefix(cleanID))

	// 1. Fast indexed lookup in temporary runtime SQLite cache (<0.5ms)
	if sqlRepo := s.db.SqlMediaRepo(); sqlRepo != nil {
		sqlRec, err := sqlRepo.GetFileDetails(ctx, cleanID)
		if err != nil {
			s.logger.Debug("sqlite media lookup error, attempting mongo fallback", "error", err)
		} else if sqlRec != nil {
			s.logger.Debug("media resolved from sqlite cache", "identifier_prefix", safeIdentifierPrefix(cleanID))
			return &MediaDetails{
				FileID:    sqlRec.FileID,
				FileRef:   sqlRec.FileRef,
				FileName:  sqlRec.FileName,
				FileSize:  sqlRec.FileSize,
				Size:      sqlRec.Size,
				FileType:  sqlRec.FileType,
				MimeType:  sqlRec.MimeType,
				Caption:   sqlRec.Caption,
				DateSaved: sqlRec.DateSaved,
			}, nil
		}
	}

	// 2. Authoritative lookup in primary/replica MongoDB collections
	if mongoRepo := s.db.MongoMediaRepo(); mongoRepo != nil {
		mongoDoc, err := mongoRepo.GetFileDetails(ctx, cleanID)
		if err != nil {
			s.logger.Error("authoritative mongodb media lookup failure",
				"identifier_prefix", safeIdentifierPrefix(cleanID),
				"error", err,
			)
			return nil, fmt.Errorf("%w: %v", ErrDatabaseUnavailable, err)
		} else if mongoDoc != nil {
			s.logger.Debug("media resolved from authoritative mongodb", "identifier_prefix", safeIdentifierPrefix(cleanID))
			return &MediaDetails{
				FileID:    mongoDoc.FileID,
				FileRef:   mongoDoc.FileRef,
				FileName:  mongoDoc.FileName,
				FileSize:  mongoDoc.FileSize,
				Size:      mongoDoc.Size,
				FileType:  mongoDoc.FileType,
				MimeType:  mongoDoc.MimeType,
				Caption:   mongoDoc.Caption,
				DateSaved: mongoDoc.Date,
			}, nil
		}
	}

	s.logger.Info("media record not found", "identifier_prefix", safeIdentifierPrefix(cleanID))
	return nil, ErrMediaNotFound
}

// ValidateMediaRecord verifies that the media record contains essential delivery attributes.
func (s *Service) ValidateMediaRecord(record *MediaDetails) error {
	if record == nil {
		return ErrMediaNotFound
	}
	if strings.TrimSpace(record.FileID) == "" {
		return ErrInvalidFileReference
	}
	if record.FileSize < 0 {
		return ErrMalformedMetadata
	}
	return nil
}

// SaveMedia stores or updates a media record in the runtime SQLite database.
func (s *Service) SaveMedia(ctx context.Context, media *MediaDetails) error {
	if s.db == nil || media == nil || media.FileID == "" {
		return nil
	}
	dateSaved := media.DateSaved
	if dateSaved.IsZero() {
		dateSaved = time.Now()
	}
	sqlRec := sql.MediaRecord{
		FileID:    media.FileID,
		FileRef:   media.FileRef,
		FileName:  media.FileName,
		FileSize:  media.FileSize,
		Size:      media.Size,
		FileType:  media.FileType,
		MimeType:  media.MimeType,
		Caption:   media.Caption,
		DateSaved: dateSaved,
	}
	return s.db.SaveMedia(ctx, sqlRec)
}

// Deliver executes the media delivery workflow with authorization checks, deduplication,
// method selection, and error handling.
func (s *Service) Deliver(ctx context.Context, chatID int64, identifier string, opts DeliveryOptions) (*DeliveryResult, error) {
	startTime := time.Now()
	cleanID := strings.TrimSpace(identifier)

	if !validIdentifierRegex.MatchString(cleanID) {
		s.logger.Warn("delivery rejected: invalid identifier format",
			"chat_id", chatID,
		)
		return nil, ErrInvalidIdentifier
	}

	// 1. In-flight request deduplication (prevents double sends from rapid user clicks)
	if opts.UserID > 0 {
		if !s.inFlight.Acquire(opts.UserID, cleanID) {
			s.logger.Warn("delivery rejected: duplicate request in flight",
				"user_id", opts.UserID,
				"identifier_prefix", safeIdentifierPrefix(cleanID),
			)
			return nil, ErrDuplicateRequest
		}
		defer s.inFlight.Release(opts.UserID, cleanID)
	}

	// 2. Authorization: Check user ban status in MongoDB
	if opts.UserID > 0 && s.db != nil && s.db.UserRepo() != nil {
		user, err := s.db.UserRepo().GetUser(ctx, opts.UserID)
		if err != nil {
			s.logger.Warn("failed to verify user ban status, proceeding cautiously", "user_id", opts.UserID, "error", err)
		} else if user != nil && user.BanStatus.IsBanned {
			s.logger.Warn("delivery rejected: user is banned",
				"user_id", opts.UserID,
				"ban_reason", user.BanStatus.BanReason,
			)
			return nil, ErrUserBanned
		}
	}

	// 3. Authorization: Check group chat status in MongoDB
	if chatID < 0 && s.db != nil && s.db.GroupRepo() != nil {
		group, err := s.db.GroupRepo().GetGroup(ctx, chatID)
		if err != nil {
			s.logger.Warn("failed to verify group chat status, proceeding cautiously", "chat_id", chatID, "error", err)
		} else if group != nil && group.ChatStatus.IsDisabled {
			s.logger.Warn("delivery rejected: chat is disabled",
				"chat_id", chatID,
				"reason", group.ChatStatus.Reason,
			)
			return nil, ErrChatDisabled
		}
	}

	// 4. Resolve authoritative media record
	rec, err := s.GetMedia(ctx, cleanID)
	if err != nil {
		return nil, err
	}

	// 5. Validate media record attributes
	if err := s.ValidateMediaRecord(rec); err != nil {
		s.logger.Warn("delivery rejected: invalid media record",
			"identifier_prefix", safeIdentifierPrefix(cleanID),
			"error", err,
		)
		return nil, err
	}

	// 6. Verify Telegram sender availability
	sender := s.getSender()
	if sender == nil {
		s.logger.Error("delivery failed: telegram sender client not configured")
		return nil, ErrTelegramUnavailable
	}

	// 7. Format caption (prefer custom caption, then dynamic formatter, fallback to record caption or filename)
	sizeStr := rec.Size
	if strings.TrimSpace(sizeStr) == "" && rec.FileSize > 0 {
		sizeStr = commands.FormatBytes(rec.FileSize)
	}

	var caption string
	if strings.TrimSpace(opts.CustomCaption) != "" {
		if s.captionFormatter != nil {
			caption = s.captionFormatter.Format(opts.CustomCaption, rec.FileName, sizeStr, rec.Caption)
		} else {
			caption = opts.CustomCaption
		}
	} else if s.captionFormatter != nil {
		caption = s.captionFormatter.FormatMedia(ctx, rec.FileName, sizeStr, rec.Caption)
	} else {
		caption = rec.Caption
		if strings.TrimSpace(caption) == "" {
			caption = rec.FileName
		}
	}


	var replyID int32
	if opts.ReplyToMessageID > 0 {
		replyID = int32(opts.ReplyToMessageID)
	}

	mediaType := strings.ToLower(strings.TrimSpace(rec.FileType))

	// Resolve the media reference via our comprehensive unpacker or Gogram's ResolveBotFileID
	var mediaToSend any = rec.FileID
	if resolvedMedia, err := ResolveInputMedia(rec.FileID, rec.FileRef); err == nil && resolvedMedia != nil {
		mediaToSend = resolvedMedia
	} else if resolved, err := telegram.ResolveBotFileID(rec.FileID); err == nil && resolved != nil {
		mediaToSend = resolved
	}

	s.logger.Info("attempting telegram media delivery",
		"chat_id", chatID,
		"media_type", mediaType,
		"identifier_prefix", safeIdentifierPrefix(cleanID),
		"protect_content", opts.ProtectContent,
	)

	var sentMsg *telegram.NewMessage
	var methodUsed string

	// Attach Fast Download button if enabled
	targetStreamURL := s.streamURL
	if opts.StreamURL != "" {
		targetStreamURL = strings.TrimRight(strings.TrimSpace(opts.StreamURL), "/")
	}
	enableBtn := s.enableFastDownload && targetStreamURL != ""
	if opts.EnableFastDownload != nil {
		enableBtn = *opts.EnableFastDownload
	}

	var replyMarkup telegram.ReplyMarkup
	if enableBtn {
		replyMarkup = telegram.NewKeyboard().
			AddRow(
				telegram.Button.Data("⚡ Fast Download", fmt.Sprintf("fast_download#%s", cleanID)),
			).
			Build()
	}

	mediaOpts := &telegram.MediaOptions{
		Caption:     caption,
		ParseMode:   "html",
		NoForwards:  opts.ProtectContent,
		ReplyID:     replyID,
		ReplyMarkup: replyMarkup,
	}

	// 8. Dispatch to appropriate Telegram send method with video fallback
	switch mediaType {
	case "video":
		methodUsed = "SendVideo"
		sentMsg, err = sender.SendMedia(chatID, mediaToSend, mediaOpts)
		// If SendVideo failed due to video container format incompatibility, fallback to SendDocument
		if err != nil && isBadMediaRequest(err) {
			s.logger.Warn("sendVideo failed with format constraint, falling back to sendDocument", "error", err)
			methodUsed = "SendDocument"
			fallbackOpts := *mediaOpts
			fallbackOpts.ForceDocument = true
			sentMsg, err = sender.SendMedia(chatID, mediaToSend, &fallbackOpts)
		}

	case "audio":
		methodUsed = "SendAudio"
		sentMsg, err = sender.SendMedia(chatID, mediaToSend, mediaOpts)

	case "photo":
		methodUsed = "SendPhoto"
		sentMsg, err = sender.SendMedia(chatID, mediaToSend, mediaOpts)

	case "animation":
		methodUsed = "SendAnimation"
		sentMsg, err = sender.SendMedia(chatID, mediaToSend, mediaOpts)

	case "voice":
		methodUsed = "SendVoice"
		sentMsg, err = sender.SendMedia(chatID, mediaToSend, mediaOpts)

	default: // "document" or unspecified
		methodUsed = "SendDocument"
		docOpts := *mediaOpts
		docOpts.ForceDocument = true
		sentMsg, err = sender.SendMedia(chatID, mediaToSend, &docOpts)
	}

	// If Telegram rejected with BUTTON_URL_INVALID or BUTTON_DATA_INVALID, retry without markup to guarantee delivery
	if err != nil && (strings.Contains(err.Error(), "BUTTON_URL_INVALID") || strings.Contains(err.Error(), "BUTTON_DATA_INVALID")) {
		s.logger.Warn("sendMedia failed with button error, retrying without replyMarkup", "error", err)
		fallbackOpts := *mediaOpts
		fallbackOpts.ReplyMarkup = nil
		sentMsg, err = sender.SendMedia(chatID, mediaToSend, &fallbackOpts)
	}

	// 9. Handle Telegram errors
	if err != nil {
		return nil, s.handleTelegramError(chatID, cleanID, err)
	}

	duration := time.Since(startTime)
	var sentMsgID int64
	if sentMsg != nil {
		sentMsgID = int64(sentMsg.ID)
		StoreDeliveredMessage(rec.FileID, chatID, sentMsg.ID)
		StoreDeliveredMessage(cleanID, chatID, sentMsg.ID)

		// Try extracting from sentMsg or fetch the full message directly from Telegram
		freshRef := ExtractFileRefFromMessage(sentMsg)
		if len(freshRef) == 0 && sender != nil {
			if msgs, gErr := sender.GetMessages(chatID, &telegram.SearchOption{IDs: []int32{sentMsg.ID}}); gErr == nil && len(msgs) > 0 {
				freshRef = ExtractFileRefFromMessage(&msgs[0])
			}
		}

		if len(freshRef) > 0 {
			StoreFreshFileRef(rec.FileID, freshRef)
			StoreFreshFileRef(cleanID, freshRef)
			if s.db != nil {
				encodedRef := base64.RawURLEncoding.EncodeToString(freshRef)
				_ = s.db.UpdateMediaFileRef(context.Background(), rec.FileID, encodedRef)
			}
		}
	}
	s.logger.Info("telegram media delivery succeeded",
		"chat_id", chatID,
		"message_id", sentMsgID,
		"delivery_method", methodUsed,
		"duration_ms", duration.Milliseconds(),
	)

	return &DeliveryResult{
		Success:          true,
		MessageID:        sentMsgID,
		ChatID:           chatID,
		FileID:           rec.FileID,
		FileName:         rec.FileName,
		FileType:         rec.FileType,
		FileSize:         rec.FileSize,
		DeliveryMethod:   methodUsed,
		Duration:         duration,
		ProtectedContent: opts.ProtectContent,
	}, nil
}

// handleTelegramError parses and sanitizes Telegram API errors.
func (s *Service) handleTelegramError(chatID int64, identifier string, err error) error {
	if err == nil {
		return nil
	}

	errMsg := err.Error()

	// Rate limit / Flood Control: FLOOD_WAIT_X or 429
	if strings.Contains(errMsg, "FLOOD_WAIT_") || strings.Contains(errMsg, "FLOOD_PREMIUM_WAIT_") || strings.Contains(errMsg, "429") {
		var retryAfter int64 = 1
		re := regexp.MustCompile(`FLOOD(?:_PREMIUM)?_WAIT_(\d+)`)
		if matches := re.FindStringSubmatch(errMsg); len(matches) > 1 {
			if parsed, pErr := strconv.ParseInt(matches[1], 10, 64); pErr == nil && parsed > 0 {
				retryAfter = parsed
			}
		} else if strings.Contains(errMsg, "wait ") {
			reSec := regexp.MustCompile(`wait (\d+) seconds`)
			if matchesSec := reSec.FindStringSubmatch(errMsg); len(matchesSec) > 1 {
				if parsed, pErr := strconv.ParseInt(matchesSec[1], 10, 64); pErr == nil && parsed > 0 {
					retryAfter = parsed
				}
			}
		}

		s.logger.Warn("telegram rate limit encountered (flood wait)",
			"chat_id", chatID,
			"retry_after_seconds", retryAfter,
		)
		return fmt.Errorf("%w: wait %d seconds", ErrRateLimited, retryAfter)
	}

	// User blocked bot or account deactivated / inaccessible
	if strings.Contains(errMsg, "USER_IS_BLOCKED") ||
		strings.Contains(errMsg, "PEER_ID_INVALID") ||
		strings.Contains(errMsg, "CHAT_WRITE_FORBIDDEN") ||
		strings.Contains(errMsg, "403") ||
		strings.Contains(strings.ToLower(errMsg), "blocked by the user") {
		s.logger.Warn("delivery failed: user blocked bot or chat inaccessible",
			"chat_id", chatID,
			"description", errMsg,
		)
		return ErrBlockedByUser
	}

	// Bad Request / Invalid reference
	s.logger.Error("telegram api error during media delivery",
		"chat_id", chatID,
		"error", errMsg,
	)
	return fmt.Errorf("%w: %s", ErrTelegramAPI, errMsg)
}

// safeIdentifierPrefix masks long identifiers in log statements.
func safeIdentifierPrefix(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8] + "..."
}

// isBadMediaRequest checks if error is related to incompatible media format.
func isBadMediaRequest(err error) bool {
	if err == nil {
		return false
	}
	desc := strings.ToLower(err.Error())
	return strings.Contains(desc, "wrong file identifier") ||
		strings.Contains(desc, "wrong type") ||
		strings.Contains(desc, "bad request") ||
		strings.Contains(desc, "can't use file") ||
		strings.Contains(desc, "media_invalid") ||
		strings.Contains(desc, "file_id_invalid") ||
		strings.Contains(desc, "fileref_")
}

// RefreshFileRef re-fetches a fresh Telegram file reference by querying the delivered message or owner chat.
func (s *Service) RefreshFileRef(ctx context.Context, fileID string) (any, error) {
	rec, err := s.GetMedia(ctx, fileID)
	if err != nil || rec == nil {
		return nil, fmt.Errorf("media not found: %w", err)
	}

	sender := s.getSender()
	if sender == nil {
		return nil, errors.New("sender not available")
	}

	var freshRef []byte

	// 1. If this file was previously delivered, fetch that message directly to get the valid reference
	if dm, ok := GetDeliveredMessage(rec.FileID); ok {
		if msgs, gErr := sender.GetMessages(dm.ChatID, &telegram.SearchOption{IDs: []int32{dm.MsgID}}); gErr == nil && len(msgs) > 0 {
			freshRef = ExtractFileRefFromMessage(&msgs[0])
		}
	}
	if len(freshRef) == 0 {
		if dm, ok := GetDeliveredMessage(fileID); ok {
			if msgs, gErr := sender.GetMessages(dm.ChatID, &telegram.SearchOption{IDs: []int32{dm.MsgID}}); gErr == nil && len(msgs) > 0 {
				freshRef = ExtractFileRefFromMessage(&msgs[0])
			}
		}
	}

	// 2. If not found or reference still empty, send to owner/admin to generate a fresh reference
	if len(freshRef) == 0 {
		var targetChat int64
		if s.ownerID != 0 {
			targetChat = s.ownerID
		} else if len(s.admins) > 0 {
			targetChat = s.admins[0]
		}
		if targetChat != 0 {
			mediaToSend, rErr := ResolveInputMedia(rec.FileID, rec.FileRef)
			if rErr == nil {
				sentMsg, sErr := sender.SendMedia(targetChat, mediaToSend, &telegram.MediaOptions{Silent: true})
				if sErr == nil && sentMsg != nil {
					// Fetch full message to get DocumentObj
					if msgs, gErr := sender.GetMessages(targetChat, &telegram.SearchOption{IDs: []int32{sentMsg.ID}}); gErr == nil && len(msgs) > 0 {
						freshRef = ExtractFileRefFromMessage(&msgs[0])
					}
					if len(freshRef) == 0 {
						freshRef = ExtractFileRefFromMessage(sentMsg)
					}
					// Keep message alive briefly before deleting so Telegram doesn't immediately revoke the reference
					time.AfterFunc(10*time.Minute, func() {
						_, _ = sentMsg.Delete()
					})
				}
			}
		}
	}

	if len(freshRef) > 0 {
		StoreFreshFileRef(rec.FileID, freshRef)
		StoreFreshFileRef(fileID, freshRef)
		if s.db != nil {
			encodedRef := base64.RawURLEncoding.EncodeToString(freshRef)
			_ = s.db.UpdateMediaFileRef(ctx, rec.FileID, encodedRef)
		}
		fileName := rec.FileName
		if fileName == "" {
			fileName = "file_" + fileID
		}
		return ResolveDownloadMedia(rec.FileID, base64.RawURLEncoding.EncodeToString(freshRef), fileName, rec.FileSize, rec.MimeType)
	}

	return nil, errors.New("failed to extract file reference from sent message")
}
