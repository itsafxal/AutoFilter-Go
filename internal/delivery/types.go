package delivery

import (
	"time"

	"github.com/amarnathcjd/gogram/telegram"
)

// MediaDetails represents normalized media information retrieved from
// authoritative MongoDB or runtime SQLite cache.
type MediaDetails struct {
	FileID    string    `json:"file_id"`
	FileRef   string    `json:"file_ref,omitempty"`
	FileName  string    `json:"file_name"`
	FileSize  int64     `json:"file_size"`
	Size      string    `json:"size,omitempty"`
	FileType  string    `json:"file_type,omitempty"`
	MimeType  string    `json:"mime_type,omitempty"`
	Caption   string    `json:"caption,omitempty"`
	DateSaved time.Time `json:"date_saved,omitempty"`
}

// DeliveryOptions configures media delivery parameters.
type DeliveryOptions struct {
	// UserID is the Telegram ID of the requesting user (used for ban check and in-flight deduplication).
	UserID int64

	// ProtectContent prevents saving and forwarding of the delivered media.
	ProtectContent bool

	// CustomCaption overrides the default caption stored in the media record.
	CustomCaption string

	// ReplyToMessageID allows replying to a specific message.
	ReplyToMessageID int64

	// StreamURL optionally overrides the stream base URL.
	StreamURL string

	// EnableFastDownload optionally overrides the service-level fast download button configuration.
	EnableFastDownload *bool

	// RecipientPeer optionally specifies the direct MTProto InputPeer for delivery, avoiding peer cache lookups.
	RecipientPeer telegram.InputPeer
}

// DeliveryResult contains structured metadata about a completed media delivery.
type DeliveryResult struct {
	Success          bool          `json:"success"`
	MessageID        int64         `json:"message_id"`
	ChatID           int64         `json:"chat_id"`
	FileID           string        `json:"file_id"`
	FileName         string        `json:"file_name"`
	FileType         string        `json:"file_type"`
	FileSize         int64         `json:"file_size"`
	DeliveryMethod   string        `json:"delivery_method"`
	Duration         time.Duration `json:"duration"`
	ProtectedContent bool          `json:"protected_content"`
}
