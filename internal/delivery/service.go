package delivery

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/itsafxal/MYTGBOTZ/internal/database"
)

var validIdentifierRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

// MediaDeliveryService defines operations for validating, resolving, and delivering
// media records stored in MongoDB / SQLite cache via Telegram Bot API.
type MediaDeliveryService interface {
	// GetMedia resolves an authoritative media record by its unique identifier.
	GetMedia(ctx context.Context, identifier string) (*MediaDetails, error)

	// ValidateMediaRecord ensures a resolved media record possesses valid delivery attributes.
	ValidateMediaRecord(record *MediaDetails) error

	// Deliver sends the requested media to the destination chat using the appropriate Telegram send method.
	Deliver(ctx context.Context, chatID int64, identifier string, opts DeliveryOptions) (*DeliveryResult, error)

	// SetSender assigns the Telegram sender client (typically *gotgbot.Bot).
	SetSender(sender TelegramSender)
}

// Service implements MediaDeliveryService.
type Service struct {
	db       *database.DatabaseManager
	sender   TelegramSender
	senderMu sync.RWMutex
	logger   *slog.Logger
	inFlight *InFlightTracker
}

// NewService constructs a new media delivery service.
func NewService(db *database.DatabaseManager, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		db:       db,
		logger:   logger,
		inFlight: NewInFlightTracker(30 * time.Second),
	}
}

// SetSender sets the active Telegram sender client.
func (s *Service) SetSender(sender TelegramSender) {
	s.senderMu.Lock()
	defer s.senderMu.Unlock()
	s.sender = sender
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

	// 7. Format caption (prefer custom caption, then record caption, fallback to filename)
	caption := opts.CustomCaption
	if strings.TrimSpace(caption) == "" {
		caption = rec.Caption
	}
	if strings.TrimSpace(caption) == "" {
		caption = rec.FileName
	}

	var replyParams *gotgbot.ReplyParameters
	if opts.ReplyToMessageID > 0 {
		replyParams = &gotgbot.ReplyParameters{
			MessageId: opts.ReplyToMessageID,
		}
	}

	fileInput := gotgbot.InputFileByID(rec.FileID)
	mediaType := strings.ToLower(strings.TrimSpace(rec.FileType))

	s.logger.Info("attempting telegram media delivery",
		"chat_id", chatID,
		"media_type", mediaType,
		"identifier_prefix", safeIdentifierPrefix(cleanID),
		"protect_content", opts.ProtectContent,
	)

	var sentMsg *gotgbot.Message
	var methodUsed string

	// 8. Dispatch to appropriate Telegram send method with video fallback
	switch mediaType {
	case "video":
		methodUsed = "SendVideo"
		sentMsg, err = sender.SendVideo(chatID, fileInput, &gotgbot.SendVideoOpts{
			Caption:         caption,
			ProtectContent:  opts.ProtectContent,
			ReplyParameters: replyParams,
		})
		// If SendVideo failed due to video container format incompatibility (e.g. MKV uploaded as document), fallback to SendDocument
		if err != nil && isBadMediaRequest(err) {
			s.logger.Warn("sendVideo failed with format constraint, falling back to sendDocument", "error", err)
			methodUsed = "SendDocument"
			sentMsg, err = sender.SendDocument(chatID, fileInput, &gotgbot.SendDocumentOpts{
				Caption:         caption,
				ProtectContent:  opts.ProtectContent,
				ReplyParameters: replyParams,
			})
		}

	case "audio":
		methodUsed = "SendAudio"
		sentMsg, err = sender.SendAudio(chatID, fileInput, &gotgbot.SendAudioOpts{
			Caption:         caption,
			ProtectContent:  opts.ProtectContent,
			ReplyParameters: replyParams,
		})

	case "photo":
		methodUsed = "SendPhoto"
		sentMsg, err = sender.SendPhoto(chatID, fileInput, &gotgbot.SendPhotoOpts{
			Caption:         caption,
			ProtectContent:  opts.ProtectContent,
			ReplyParameters: replyParams,
		})

	case "animation":
		methodUsed = "SendAnimation"
		sentMsg, err = sender.SendAnimation(chatID, fileInput, &gotgbot.SendAnimationOpts{
			Caption:         caption,
			ProtectContent:  opts.ProtectContent,
			ReplyParameters: replyParams,
		})

	case "voice":
		methodUsed = "SendVoice"
		sentMsg, err = sender.SendVoice(chatID, fileInput, &gotgbot.SendVoiceOpts{
			Caption:         caption,
			ProtectContent:  opts.ProtectContent,
			ReplyParameters: replyParams,
		})

	default: // "document" or unspecified
		methodUsed = "SendDocument"
		sentMsg, err = sender.SendDocument(chatID, fileInput, &gotgbot.SendDocumentOpts{
			Caption:         caption,
			ProtectContent:  opts.ProtectContent,
			ReplyParameters: replyParams,
		})
	}

	// 9. Handle Telegram errors
	if err != nil {
		return nil, s.handleTelegramError(chatID, cleanID, err)
	}

	duration := time.Since(startTime)
	s.logger.Info("telegram media delivery succeeded",
		"chat_id", chatID,
		"message_id", sentMsg.MessageId,
		"delivery_method", methodUsed,
		"duration_ms", duration.Milliseconds(),
	)

	return &DeliveryResult{
		Success:          true,
		MessageID:        sentMsg.MessageId,
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
	var tgErr *gotgbot.TelegramError
	if errors.As(err, &tgErr) {
		// Rate limit / Flood Control
		if tgErr.Code == 429 || (tgErr.ResponseParams != nil && tgErr.ResponseParams.RetryAfter > 0) {
			var retryAfter int64 = 1
			if tgErr.ResponseParams != nil && tgErr.ResponseParams.RetryAfter > 0 {
				retryAfter = tgErr.ResponseParams.RetryAfter
			}
			s.logger.Warn("telegram rate limit encountered (flood wait)",
				"chat_id", chatID,
				"retry_after_seconds", retryAfter,
			)
			return fmt.Errorf("%w: wait %d seconds", ErrRateLimited, retryAfter)
		}

		// User blocked bot or account deactivated
		if tgErr.Code == 403 {
			s.logger.Warn("delivery failed: user blocked bot or chat inaccessible",
				"chat_id", chatID,
				"description", tgErr.Description,
			)
			return ErrBlockedByUser
		}

		// Bad Request / Invalid reference
		s.logger.Error("telegram api error during media delivery",
			"chat_id", chatID,
			"code", tgErr.Code,
			"description", tgErr.Description,
		)
		return fmt.Errorf("%w: %s", ErrTelegramAPI, tgErr.Description)
	}

	// Generic/network failure
	s.logger.Error("unexpected delivery error",
		"chat_id", chatID,
		"error", err,
	)
	return fmt.Errorf("%w: %v", ErrTelegramAPI, err)
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
	var tgErr *gotgbot.TelegramError
	if errors.As(err, &tgErr) {
		desc := strings.ToLower(tgErr.Description)
		return strings.Contains(desc, "wrong file identifier") ||
			strings.Contains(desc, "wrong type") ||
			strings.Contains(desc, "bad request") ||
			strings.Contains(desc, "can't use file")
	}
	return false
}
