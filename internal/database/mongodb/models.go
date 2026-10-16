package mongodb

import "time"

// BanStatus tracks user moderation state matching the authoritative MongoDB schema.
type BanStatus struct {
	IsBanned  bool   `bson:"is_banned" json:"is_banned"`
	BanReason string `bson:"ban_reason" json:"ban_reason"`
}

// User represents a Telegram user document in the "users" collection.
type User struct {
	ID        int64     `bson:"id" json:"id"`
	Name      string    `bson:"name" json:"name"`
	BanStatus BanStatus `bson:"ban_status" json:"ban_status"`
}

// ChatStatus tracks group chat moderation state.
type ChatStatus struct {
	IsDisabled bool   `bson:"is_disabled" json:"is_disabled"`
	Reason     string `bson:"reason" json:"reason"`
}

// Group represents a Telegram group document in the "groups" collection.
type Group struct {
	ID         int64      `bson:"id" json:"id"`
	Title      string     `bson:"title" json:"title"`
	ChatStatus ChatStatus `bson:"chat_status" json:"chat_status"`
}

// MediaDoc represents an authoritative movie/file document in the media collections (e.g. "Telegram-Document").
type MediaDoc struct {
	FileID   string    `bson:"_id" json:"file_id"`
	FileRef  string    `bson:"file_ref,omitempty" json:"file_ref,omitempty"`
	FileName string    `bson:"file_name" json:"file_name"`
	FileSize int64     `bson:"file_size" json:"file_size"`
	Size     string    `bson:"size,omitempty" json:"size,omitempty"`
	FileType string    `bson:"file_type,omitempty" json:"file_type,omitempty"`
	MimeType string    `bson:"mime_type,omitempty" json:"mime_type,omitempty"`
	Caption  string    `bson:"caption,omitempty" json:"caption,omitempty"`
	Date     time.Time `bson:"date,omitempty" json:"date,omitempty"`
}
