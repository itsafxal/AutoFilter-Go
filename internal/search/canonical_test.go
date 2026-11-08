package search

import (
	"strings"
	"testing"
)

func TestGroupFilesIntoCanonicalMovies_ImGameGrouping(t *testing.T) {
	files := []MediaFile{
		{
			FileID:   "file_mal_1080p",
			FileName: "I'm.Game.2026.Malayalam.1080p.WEBRip.x264.DD+5.1.mkv",
			FileSize: 2500000000,
			Size:     "2.5 GB",
		},
		{
			FileID:   "file_mal_720p",
			FileName: "I'm.Game.2026.Malayalam.720p.HEVC.x265.mkv",
			FileSize: 1200000000,
			Size:     "1.2 GB",
		},
		{
			FileID:   "file_mal_480p",
			FileName: "I'm.Game.2026.Malayalam.480p.x264.mkv",
			FileSize: 600000000,
			Size:     "600 MB",
		},
		{
			FileID:   "file_mal_remux",
			FileName: "I'm.Game.2026.Malayalam.2160p.4K.HDR.mkv",
			FileSize: 6500000000,
			Size:     "6.5 GB",
		},
		{
			FileID:   "file_tel_1080p",
			FileName: "I'm.Game.2026.Telugu.1080p.WEBRip.mkv",
			FileSize: 2300000000,
			Size:     "2.3 GB",
		},
		{
			FileID:   "file_tel_720p",
			FileName: "I'm.Game.2026.Telugu.720p.mkv",
			FileSize: 1100000000,
			Size:     "1.1 GB",
		},
	}

	intent := ParseQueryIntent("I'm Game Malayalam")
	movies := GroupFilesIntoCanonicalMovies(files, "I'm Game Malayalam", intent)

	if len(movies) != 1 {
		t.Fatalf("expected 1 canonical movie, got %d", len(movies))
	}

	movie := movies[0]
	if movie.Year != 2026 {
		t.Errorf("movie.Year = %d, want 2026", movie.Year)
	}
	if movie.TotalFiles != 6 {
		t.Errorf("movie.TotalFiles = %d, want 6", movie.TotalFiles)
	}

	if len(movie.LanguageVariants) != 2 {
		t.Fatalf("expected 2 language variants (Malayalam & Telugu), got %d", len(movie.LanguageVariants))
	}

	// First variant must be Malayalam because intent was Malayalam
	malVariant := movie.LanguageVariants[0]
	if malVariant.NormalizedLanguage != "malayalam" {
		t.Errorf("expected first variant to be malayalam, got %q", malVariant.NormalizedLanguage)
	}
	if len(malVariant.Files) != 4 {
		t.Errorf("expected 4 files in malayalam variant, got %d", len(malVariant.Files))
	}

	// 4K should be first in Malayalam files, followed by 1080p, 720p, 480p
	if malVariant.Files[0].FileID != "file_mal_remux" {
		t.Errorf("expected 4K file first in Malayalam, got %s", malVariant.Files[0].FileID)
	}
	if malVariant.Files[1].FileID != "file_mal_1080p" {
		t.Errorf("expected 1080p file second in Malayalam, got %s", malVariant.Files[1].FileID)
	}

	// Second variant must be Telugu
	telVariant := movie.LanguageVariants[1]
	if telVariant.NormalizedLanguage != "telugu" {
		t.Errorf("expected second variant to be telugu, got %q", telVariant.NormalizedLanguage)
	}
	if len(telVariant.Files) != 2 {
		t.Errorf("expected 2 files in telugu variant, got %d", len(telVariant.Files))
	}

	// Verify all file IDs are preserved intact
	seenIDs := make(map[string]bool)
	for _, v := range movie.LanguageVariants {
		for _, f := range v.Files {
			seenIDs[f.FileID] = true
		}
	}
	for _, expectedID := range []string{"file_mal_1080p", "file_mal_720p", "file_mal_480p", "file_mal_remux", "file_tel_1080p", "file_tel_720p"} {
		if !seenIDs[expectedID] {
			t.Errorf("missing preserved file ID: %s", expectedID)
		}
	}
}

func TestFormatFileOptionLabel(t *testing.T) {
	file := MediaFile{
		FileName: "I'm.Game.2026.Malayalam.1080p.WEBRip.x264.DD+5.1.mkv",
		Size:     "2.5 GB",
	}

	label := FormatFileOptionLabel(file)
	expectedSubstrings := []string{"1080P", "X264", "DD+5.1", "2.5 GB"}
	for _, sub := range expectedSubstrings {
		if !strings.Contains(strings.ToLower(label), strings.ToLower(sub)) {
			t.Errorf("FormatFileOptionLabel(%q) = %q, expected substring %q", file.FileName, label, sub)
		}
	}
}
