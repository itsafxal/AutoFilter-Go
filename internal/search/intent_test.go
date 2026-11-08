package search

import (
	"testing"
)

func TestParseQueryIntent(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantTitle  string
		wantLang   string
		wantQual   string
		wantYear   int
		wantSeason int
		wantEp     int
	}{
		{
			name:      "Plain title with contraction",
			query:     "I'm Game",
			wantTitle: "I'm Game",
		},
		{
			name:      "Title with Malayalam language tag",
			query:     "I'm Game Malayalam",
			wantTitle: "I'm Game",
			wantLang:  "malayalam",
		},
		{
			name:      "Title with lowercase contraction and language",
			query:     "i am game malayalam",
			wantTitle: "i am game",
			wantLang:  "malayalam",
		},
		{
			name:      "Title with year, language and quality",
			query:     "I'm Game 2026 Malayalam 1080p",
			wantTitle: "I'm Game",
			wantYear:  2026,
			wantLang:  "malayalam",
			wantQual:  "1080p",
		},
		{
			name:      "Title with dot notation and language",
			query:     "I.Am.Game.Malayalam",
			wantTitle: "I Am Game",
			wantLang:  "malayalam",
		},
		{
			name:      "Telugu language query",
			query:     "Jailer 2 Telugu",
			wantTitle: "Jailer 2",
			wantLang:  "telugu",
		},
		{
			name:       "Series query with Season and Episode",
			query:      "Loki S02E03 720p",
			wantTitle:  "Loki",
			wantQual:   "720p",
			wantSeason: 2,
			wantEp:     3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseQueryIntent(tt.query)
			if got.CleanTitle != tt.wantTitle {
				t.Errorf("CleanTitle = %q, want %q", got.CleanTitle, tt.wantTitle)
			}
			if got.Language != tt.wantLang {
				t.Errorf("Language = %q, want %q", got.Language, tt.wantLang)
			}
			if got.Quality != tt.wantQual {
				t.Errorf("Quality = %q, want %q", got.Quality, tt.wantQual)
			}
			if got.Year != tt.wantYear {
				t.Errorf("Year = %d, want %d", got.Year, tt.wantYear)
			}
			if got.Season != tt.wantSeason {
				t.Errorf("Season = %d, want %d", got.Season, tt.wantSeason)
			}
			if got.Episode != tt.wantEp {
				t.Errorf("Episode = %d, want %d", got.Episode, tt.wantEp)
			}
		})
	}
}

func TestNormalizeLanguageCode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"malayalam", "malayalam"},
		{"Malayalam", "malayalam"},
		{"mal", "malayalam"},
		{"Malay", "malayalam"},
		{"മലയാളം", "malayalam"},
		{"tamil", "tamil"},
		{"tam", "tamil"},
		{"தமிழ்", "tamil"},
		{"telugu", "telugu"},
		{"tel", "telugu"},
		{"తెలుగు", "telugu"},
		{"hindi", "hindi"},
		{"hin", "hindi"},
		{"multi", "multi"},
		{"dual audio", "multi"},
		{"multi audio", "multi"},
		{"unknown_language_xyz", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeLanguageCode(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeLanguageCode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
