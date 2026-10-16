package search

import (
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
