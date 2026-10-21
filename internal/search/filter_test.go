package search

import (
	"testing"
)

func TestMatchesQuality(t *testing.T) {
	tests := []struct {
		name      string
		fileName  string
		caption   string
		target    string
		wantMatch bool
	}{
		{
			name:      "Exact 720p in filename",
			fileName:  "Vikram.2022.720p.WEB-DL.mkv",
			caption:   "Vikram 2022",
			target:    "720p",
			wantMatch: true,
		},
		{
			name:      "1080p in filename does not match 720p",
			fileName:  "Vikram.2022.1080p.WEB-DL.mkv",
			caption:   "Vikram 2022",
			target:    "720p",
			wantMatch: false,
		},
		{
			name:      "1080p in filename ignores 720p in promotional caption",
			fileName:  "Vikram.2022.1080p.WEB-DL.mkv",
			caption:   "Available in: 480p, 720p, 1080p",
			target:    "720p",
			wantMatch: false,
		},
		{
			name:      "4K matches 2160p",
			fileName:  "Oppenheimer.2023.2160p.mkv",
			caption:   "Oppenheimer 2023",
			target:    "4k",
			wantMatch: true,
		},
		{
			name:      "Fallback to caption single resolution",
			fileName:  "Leo.2023.HDRip.mkv",
			caption:   "Leo 2023 720p HDRip",
			target:    "720p",
			wantMatch: true,
		},
		{
			name:      "Fallback caption with multiple resolutions ignored as spam",
			fileName:  "Leo.2023.HDRip.mkv",
			caption:   "Join channel for 480p, 720p, 1080p",
			target:    "720p",
			wantMatch: false,
		},
		{
			name:      "Empty target always matches",
			fileName:  "Vikram.2022.1080p.mkv",
			caption:   "",
			target:    "",
			wantMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchesQuality(tt.fileName, tt.caption, tt.target)
			if got != tt.wantMatch {
				t.Errorf("MatchesQuality(%q, %q, %q) = %v, want %v",
					tt.fileName, tt.caption, tt.target, got, tt.wantMatch)
			}
		})
	}
}

func TestMatchesLanguage(t *testing.T) {
	tests := []struct {
		name      string
		fileName  string
		caption   string
		target    string
		wantMatch bool
	}{
		{
			name:      "Exact Hindi in filename",
			fileName:  "KGF.Chapter.2.2022.720p.Hindi.mkv",
			caption:   "KGF 2 Hindi",
			target:    "hindi",
			wantMatch: true,
		},
		{
			name:      "Tamil in filename does not match Hindi",
			fileName:  "Vikram.2022.Tamil.1080p.mkv",
			caption:   "Vikram Movie",
			target:    "hindi",
			wantMatch: false,
		},
		{
			name:      "Tamil in filename ignores Hindi in spam caption",
			fileName:  "Vikram.2022.Tamil.1080p.mkv",
			caption:   "Audio available: Malayalam, Tamil, Hindi, Telugu",
			target:    "hindi",
			wantMatch: false,
		},
		{
			name:      "Multi-audio file matches Hindi and Tamil",
			fileName:  "Vikram.2022.Hindi.Tamil.Telugu.1080p.mkv",
			caption:   "",
			target:    "hindi",
			wantMatch: true,
		},
		{
			name:      "Multi-audio file does not match Malayalam when not present",
			fileName:  "Vikram.2022.Hindi.Tamil.Telugu.1080p.mkv",
			caption:   "",
			target:    "malayalam",
			wantMatch: false,
		},
		{
			name:      "Dual audio matches Multi",
			fileName:  "Vikram.2022.Dual.Audio.1080p.mkv",
			caption:   "",
			target:    "multi",
			wantMatch: true,
		},
		{
			name:      "Single Tamil does not match Multi",
			fileName:  "Vikram.2022.Tamil.1080p.mkv",
			caption:   "",
			target:    "multi",
			wantMatch: false,
		},
		{
			name:      "Abbreviation Mal matches Malayalam",
			fileName:  "Drishyam.2013.Mal.720p.mkv",
			caption:   "",
			target:    "malayalam",
			wantMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchesLanguage(tt.fileName, tt.caption, tt.target)
			if got != tt.wantMatch {
				t.Errorf("MatchesLanguage(%q, %q, %q) = %v, want %v",
					tt.fileName, tt.caption, tt.target, got, tt.wantMatch)
			}
		})
	}
}
