package delivery

import (
	"errors"
)

var (
	// ErrMediaNotFound is returned when the requested media cannot be found in MongoDB or SQLite.
	ErrMediaNotFound = errors.New("media record not found")

	// ErrInvalidFileReference is returned when the media record is missing a usable Telegram file ID.
	ErrInvalidFileReference = errors.New("media record does not contain a usable Telegram file ID")

	// ErrInvalidIdentifier is returned when the identifier format is invalid or exceeds safe limits.
	ErrInvalidIdentifier = errors.New("invalid media identifier format")

	// ErrMalformedMetadata is returned when essential metadata fields in the record are corrupted.
	ErrMalformedMetadata = errors.New("media record metadata is malformed or invalid")

	// ErrUserBanned is returned when the requesting user is marked as banned in the authoritative users collection.
	ErrUserBanned = errors.New("user is banned from receiving media")

	// ErrChatDisabled is returned when the target chat is disabled for bot operations.
	ErrChatDisabled = errors.New("chat is disabled for media delivery")

	// ErrDuplicateRequest is returned when an identical delivery request is already in progress.
	ErrDuplicateRequest = errors.New("a delivery request for this file is already in progress")

	// ErrRateLimited is returned when Telegram API returns flood wait / 429.
	ErrRateLimited = errors.New("telegram flood control rate limit")

	// ErrBlockedByUser is returned when the user has blocked the bot or account was deactivated.
	ErrBlockedByUser = errors.New("bot was blocked by user or account deactivated")

	// ErrTelegramAPI is returned when Telegram Bot API rejects the delivery request.
	ErrTelegramAPI = errors.New("telegram API delivery failure")

	// ErrDatabaseUnavailable is returned when the database layer is not initialized or times out.
	ErrDatabaseUnavailable = errors.New("database layer is unavailable")

	// ErrTelegramUnavailable is returned when the Telegram sender client has not been configured.
	ErrTelegramUnavailable = errors.New("telegram sender client is unavailable")
)
