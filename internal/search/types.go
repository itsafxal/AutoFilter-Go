package search

import "time"

// MediaFile represents an indexed media item for search results.
type MediaFile struct {
	FileID    string    `json:"file_id"`
	FileRef   string    `json:"file_ref,omitempty"`
	FileName  string    `json:"file_name"`
	FileSize  int64     `json:"file_size"`
	Size      string    `json:"size"`
	FileType  string    `json:"file_type"`
	MimeType  string    `json:"mime_type,omitempty"`
	Caption   string    `json:"caption,omitempty"`
	DateSaved time.Time `json:"date_saved"`
}

// SearchResult contains paginated media search results.
type SearchResult struct {
	Query      string      `json:"query"`
	CleanQuery string      `json:"clean_query"`
	Files      []MediaFile `json:"files"`
	Total      int64       `json:"total"`
	Offset     int         `json:"offset"`
	NextOffset int         `json:"next_offset"`
	PrevOffset int         `json:"prev_offset"`
	Page       int         `json:"page"`
	TotalPages int         `json:"total_pages"`
	HasNext    bool        `json:"has_next"`
	HasPrev    bool        `json:"has_prev"`
}

// SearchFilter specifies optional search criteria.
type SearchFilter struct {
	Language string
	Quality  string
}
