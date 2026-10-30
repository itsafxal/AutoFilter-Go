package gfilter

import (
	"encoding/json"
	"strings"
	"time"
)

// MediaType represents the kind of Telegram response payload.
type MediaType string

const (
	MediaTypeText      MediaType = "text"
	MediaTypePhoto     MediaType = "photo"
	MediaTypeVideo     MediaType = "video"
	MediaTypeDocument  MediaType = "document"
	MediaTypeAudio     MediaType = "audio"
	MediaTypeVoice     MediaType = "voice"
	MediaTypeAnimation MediaType = "animation"
	MediaTypeSticker   MediaType = "sticker"
)

// InlineButton represents a single inline button definition.
type InlineButton struct {
	Text string `json:"text"`
	URL  string `json:"url,omitempty"`
	Data string `json:"data,omitempty"`
}

// ButtonRow represents a row of inline buttons.
type ButtonRow []InlineButton

// GFilter represents an authoritative Global Filter record.
type GFilter struct {
	ID          string      `bson:"_id,omitempty" json:"id"`
	Keyword     string      `bson:"keyword" json:"keyword"`
	KeywordNorm string      `bson:"keyword_norm" json:"keyword_norm"`
	ReplyText   string      `bson:"reply_text,omitempty" json:"reply_text,omitempty"`
	MediaType   MediaType   `bson:"media_type" json:"media_type"`
	FileID      string      `bson:"file_id,omitempty" json:"file_id,omitempty"`
	FileRef     string      `bson:"file_ref,omitempty" json:"file_ref,omitempty"`
	Buttons     []ButtonRow `bson:"-" json:"buttons,omitempty"`
	ButtonsJSON string      `bson:"buttons_json,omitempty" json:"buttons_json,omitempty"`
	CreatedAt   time.Time   `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time   `bson:"updated_at" json:"updated_at"`
}

// NormalizeKeyword returns the lower-cased keyword for case-insensitive exact index matching.
// Does NOT trim whitespace or collapse spaces, preserving original characters and spacing.
func NormalizeKeyword(keyword string) string {
	return strings.ToLower(keyword)
}

// PackButtons serializes the in-memory Buttons slice into JSON.
func (f *GFilter) PackButtons() {
	if len(f.Buttons) == 0 {
		f.ButtonsJSON = ""
		return
	}
	bytes, err := json.Marshal(f.Buttons)
	if err == nil {
		f.ButtonsJSON = string(bytes)
	}
}

// UnpackButtons deserializes ButtonsJSON into the in-memory Buttons slice.
func (f *GFilter) UnpackButtons() {
	if strings.TrimSpace(f.ButtonsJSON) == "" {
		f.Buttons = nil
		return
	}
	var rows []ButtonRow
	if err := json.Unmarshal([]byte(f.ButtonsJSON), &rows); err == nil {
		f.Buttons = rows
	}
}
