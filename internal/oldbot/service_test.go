package oldbot

import (
	"testing"
)

func TestBuildAndDecodeSearchDeepLinkPayload(t *testing.T) {
	testCases := []string{
		"KGF",
		"Leo",
		"Manjummel Boys",
		"Vikram 2022 Tamil",
		"Inception 1080p",
		"A very long movie title that goes well beyond the standard forty runes limit to test truncation properly",
	}

	for _, tc := range testCases {
		payload := BuildSearchDeepLinkPayload(tc)
		if len(payload) > 64 {
			t.Errorf("payload length %d exceeds Telegram 64 char limit for query %q", len(payload), tc)
		}
		decoded, ok := DecodeSearchDeepLinkPayload(payload)
		if !ok {
			t.Errorf("failed to decode payload %q for query %q", payload, tc)
		}
		expectedPrefix := tc
		if len([]rune(expectedPrefix)) > 40 {
			expectedPrefix = string([]rune(expectedPrefix)[:40])
		}
		if decoded != expectedPrefix {
			t.Errorf("expected decoded %q, got %q", expectedPrefix, decoded)
		}
	}

	// Test search_<query> fallback
	decoded, ok := DecodeSearchDeepLinkPayload("search_kgf_chapter_2")
	if !ok || decoded != "kgf chapter 2" {
		t.Errorf("expected 'kgf chapter 2', got %q (ok=%v)", decoded, ok)
	}

	// Test invalid payload
	_, ok = DecodeSearchDeepLinkPayload("invalid_payload")
	if ok {
		t.Errorf("expected false for invalid payload")
	}
}

func TestTruncateText(t *testing.T) {
	tests := []struct {
		input    string
		max      int
		expected string
	}{
		{"Short", 10, "Short"},
		{"ExactLength", 11, "ExactLength"},
		{"This is longer than ten", 10, "This is..."},
	}

	for _, tt := range tests {
		got := truncateText(tt.input, tt.max)
		if got != tt.expected {
			t.Errorf("truncateText(%q, %d) = %q, expected %q", tt.input, tt.max, got, tt.expected)
		}
	}
}
