package batch

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Status constants for batch lifecycle.
const (
	StatusCollecting = "collecting"
	StatusFinalized  = "finalized"
	StatusCancelled  = "cancelled"
)

// ItemType constants for media and text content within a batch.
const (
	ItemTypeDocument  = "document"
	ItemTypeVideo     = "video"
	ItemTypeAudio     = "audio"
	ItemTypePhoto     = "photo"
	ItemTypeVoice     = "voice"
	ItemTypeAnimation = "animation"
	ItemTypeVideoNote = "video_note"
	ItemTypeText      = "text"
)

// BatchMetadata represents the authoritative header record of a batch collection.
type BatchMetadata struct {
	BatchID        string     `bson:"batch_id" json:"batch_id"`
	BatchName      string     `bson:"batch_name" json:"batch_name"`
	NormalizedName string     `bson:"normalized_name" json:"normalized_name"`
	OwnerID        int64      `bson:"owner_id" json:"owner_id"`
	TotalItems     int        `bson:"total_items" json:"total_items"`
	TotalFiles     int        `bson:"total_files" json:"total_files"`
	TotalTexts     int        `bson:"total_texts" json:"total_texts"`
	TotalDocuments int        `bson:"total_documents" json:"total_documents"`
	TotalVideos    int        `bson:"total_videos" json:"total_videos"`
	TotalAudio     int        `bson:"total_audio" json:"total_audio"`
	TotalPhotos    int        `bson:"total_photos" json:"total_photos"`
	Status         string     `bson:"status" json:"status"`
	CreatedAt      time.Time  `bson:"created_at" json:"created_at"`
	UpdatedAt      time.Time  `bson:"updated_at" json:"updated_at"`
	FinalizedAt    *time.Time `bson:"finalized_at,omitempty" json:"finalized_at,omitempty"`
}

// BatchItem represents a single sequential element within a batch.
type BatchItem struct {
	ID                   int64     `bson:"-" json:"id,omitempty"`
	BatchID              string    `bson:"batch_id" json:"batch_id"`
	Sequence             int       `bson:"sequence" json:"sequence"`
	ItemType             string    `bson:"item_type" json:"item_type"`
	TelegramFileID       string    `bson:"telegram_file_id,omitempty" json:"telegram_file_id,omitempty"`
	TelegramFileUniqueID string    `bson:"telegram_file_unique_id,omitempty" json:"telegram_file_unique_id,omitempty"`
	StorageReference     string    `bson:"storage_reference,omitempty" json:"storage_reference,omitempty"`
	FileName             string    `bson:"file_name,omitempty" json:"file_name,omitempty"`
	FileSize             int64     `bson:"file_size" json:"file_size"`
	MimeType             string    `bson:"mime_type,omitempty" json:"mime_type,omitempty"`
	Caption              string    `bson:"caption,omitempty" json:"caption,omitempty"`
	TextContent          string    `bson:"text_content,omitempty" json:"text_content,omitempty"`
	MessageEntities      string    `bson:"message_entities,omitempty" json:"message_entities,omitempty"`
	CreatedAt            time.Time `bson:"created_at" json:"created_at"`
}

// NormalizeBatchName normalizes a batch name for case-insensitive exact and prefix matching.
// Converts to lower-case, trims whitespace, and collapses multiple spaces.
func NormalizeBatchName(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	return strings.Join(strings.Fields(lower), " ")
}

// GenerateBatchID creates a URL-safe, compact unique batch identifier (e.g. "b_6a4f89d10e2c").
func GenerateBatchID() string {
	bytes := make([]byte, 6)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp hex if crypto/rand encounters system issues
		return fmt.Sprintf("b_%x", time.Now().UnixNano())
	}
	return "b_" + hex.EncodeToString(bytes)
}
