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

// SearchResult contains paginated media search results and grouped canonical movies.
type SearchResult struct {
	Query              string           `json:"query"`
	CleanQuery         string           `json:"clean_query"`
	Files              []MediaFile      `json:"files"`
	Movies             []CanonicalMovie `json:"movies,omitempty"`
	Total              int64            `json:"total"`
	Offset             int              `json:"offset"`
	NextOffset         int              `json:"next_offset"`
	PrevOffset         int              `json:"prev_offset"`
	Page               int              `json:"page"`
	TotalPages         int              `json:"total_pages"`
	HasNext            bool             `json:"has_next"`
	HasPrev            bool             `json:"has_prev"`
	RequestedLanguage  string           `json:"requested_language,omitempty"`
	LanguageMatched    bool             `json:"language_matched,omitempty"`
	AvailableLanguages []string         `json:"available_languages,omitempty"`
}

// CanonicalMovie represents a unique movie or series entity grouping one or more media files.
type CanonicalMovie struct {
	CanonicalID      string            `json:"canonical_id"`
	CanonicalTitle   string            `json:"canonical_title"`
	NormalizedTitle  string            `json:"normalized_title"`
	CompactTitle     string            `json:"compact_title"`
	Year             int               `json:"year,omitempty"`
	Sequel           string            `json:"sequel,omitempty"`
	MediaType        string            `json:"media_type"` // "movie" or "series"
	Season           int               `json:"season,omitempty"`
	Episode          int               `json:"episode,omitempty"`
	LanguageVariants []LanguageVariant `json:"language_variants"`
	TotalFiles       int               `json:"total_files"`
	PrimaryLanguage  string            `json:"primary_language,omitempty"`
	Score            float64           `json:"score,omitempty"`
}

// LanguageVariant represents a specific language release of a canonical movie.
type LanguageVariant struct {
	Language           string      `json:"language"`
	NormalizedLanguage string      `json:"normalized_language"`
	Files              []MediaFile `json:"files"`
}

// SearchFilter specifies optional search criteria.
type SearchFilter struct {
	Language string
	Quality  string
}
