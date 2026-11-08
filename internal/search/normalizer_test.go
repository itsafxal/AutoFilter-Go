package search

import (
	"strings"
	"testing"
)

func TestNormalizeText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain text",
			input:    "KGF Chapter 2",
			expected: "kgf chapter 2",
		},
		{
			name:     "dots and hyphens",
			input:    "K.G.F-Chapter.2",
			expected: "k g f chapter 2",
		},
		{
			name:     "brackets and symbols",
			input:    "[1080p] Avatar: The Way of Water (2022) @MyChannel",
			expected: "1080p avatar the way of water 2022",
		},
		{
			name:     "transliteration malayalam",
			input:    "കെജിഎഫ്",
			expected: "kgf",
		},
		{
			name:     "transliteration hindi",
			input:    "जवान",
			expected: "jawan",
		},
		{
			name:     "deterministic overlapping compound transliteration malayalam",
			input:    "വിക്രം വേധ",
			expected: "vikram vedha",
		},
		{
			name:     "deterministic longer transliteration malayalam",
			input:    "വിക്രമാദിത്യൻ",
			expected: "vikramadithyan",
		},
		{
			name:     "deterministic compound transliteration tamil",
			input:    "விக்ரம் வேதா",
			expected: "vikram vedha",
		},
		{
			name:     "standalone shorter transliteration malayalam",
			input:    "വിക്രം",
			expected: "vikram",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeText(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizeText(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestCleanReleaseName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "movie file with tags and extension",
			input:    "KGF.Chapter.2.2022.1080p.WEBRip.x264.AAC5.1-[TG].mkv",
			expected: "KGF Chapter 2 2022 TG",
		},
		{
			name:     "simple file",
			input:    "Inception.mp4",
			expected: "Inception",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CleanReleaseName(tt.input)
			if got != tt.expected {
				t.Errorf("CleanReleaseName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestIsValidSearchText(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"kgf", true},
		{"a", false},                  // too short
		{"https://google.com", false}, // url
		{"@channel", false},           // only username
		{"...", false},                // punctuation
		{"Leo 2023", true},
	}

	for _, tt := range tests {
		got := IsValidSearchText(tt.input)
		if got != tt.expected {
			t.Errorf("IsValidSearchText(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestExtractQueryTitle(t *testing.T) {
	input := "📕 Name ➜ : Monster - The Lizzie Borden Story S01E08 Carnival -@MNTGX.-.mkv\n\n📗 Size ➜ : 247.81 MB\n\npowered by #mntgx"
	expected := "Monster - The Lizzie Borden Story S01E08 Carnival -@MNTGX.-.mkv"
	got := ExtractQueryTitle(input)
	if got != expected {
		t.Errorf("ExtractQueryTitle() = %q, want %q", got, expected)
	}

	norm := NormalizeText(got)
	if !strings.Contains(norm, "monster") || !strings.Contains(norm, "lizzie") {
		t.Errorf("expected normalized text to contain 'monster' and 'lizzie', got %q", norm)
	}
}

func TestSplitCamelCaseAndNumbers(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Jailer2", "Jailer 2"},
		{"IAmGame", "I Am Game"},
		{"KGFChapter2", "KGF Chapter 2"},
		{"SpiderMan3", "Spider Man 3"},
		{"2Fast2Furious", "2 Fast 2 Furious"},
		{"Avatar2022", "Avatar 2022"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := SplitCamelCaseAndNumbers(tt.input)
			if got != tt.expected {
				t.Errorf("SplitCamelCaseAndNumbers(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNormalizeCompact(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Jailer2", "jailer2"},
		{"Jailer 2", "jailer2"},
		{"Jailer-2", "jailer2"},
		{"Jailer_2", "jailer2"},
		{"JAILER2", "jailer2"},
		{"jailer2", "jailer2"},
		{"I Am Game", "iamgame"},
		{"I.Am.Game", "iamgame"},
		{"I-Am-Game", "iamgame"},
		{"I_Am_Game", "iamgame"},
		{"IAmGame", "iamgame"},
		{"I  Am  Game", "iamgame"},
		{"I•Am•Game", "iamgame"},
		{"I@Am@Game", "iamgame"},
		{"KGF2", "kgf2"},
		{"KGF 2", "kgf2"},
		{"K.G.F", "kgf"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeCompact(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizeCompact(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestCollapseAcronyms(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"k g f", "kgf"},
		{"d d l j", "ddlj"},
		{"i am game", "i am game"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := CollapseAcronyms(tt.input)
			if got != tt.expected {
				t.Errorf("CollapseAcronyms(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
