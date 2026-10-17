package commands

import (
	"strings"
	"testing"
	"time"
)

func TestValidateStartPayload(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantHas     bool
		wantValid   bool
		wantPayload string
	}{
		{
			name:        "no arguments",
			args:        []string{"/start"},
			wantHas:     false,
			wantValid:   true,
			wantPayload: "",
		},
		{
			name:        "empty payload argument",
			args:        []string{"/start", ""},
			wantHas:     false,
			wantValid:   true,
			wantPayload: "",
		},
		{
			name:        "valid alphanumeric payload",
			args:        []string{"/start", "getfile-12345_abc"},
			wantHas:     true,
			wantValid:   true,
			wantPayload: "getfile-12345_abc",
		},
		{
			name:        "valid single character",
			args:        []string{"/start", "a"},
			wantHas:     true,
			wantValid:   true,
			wantPayload: "a",
		},
		{
			name:        "invalid character space",
			args:        []string{"/start", "hello world"},
			wantHas:     true,
			wantValid:   false,
			wantPayload: "hello world",
		},
		{
			name:        "invalid character punctuation",
			args:        []string{"/start", "id=10&token=xyz"},
			wantHas:     true,
			wantValid:   false,
			wantPayload: "id=10&token=xyz",
		},
		{
			name:        "invalid characters injection attempt",
			args:        []string{"/start", "'; DROP TABLE users; --"},
			wantHas:     true,
			wantValid:   false,
			wantPayload: "'; DROP TABLE users; --",
		},
		{
			name:        "payload exceeds 64 characters",
			args:        []string{"/start", strings.Repeat("a", 65)},
			wantHas:     true,
			wantValid:   false,
			wantPayload: strings.Repeat("a", 65),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := ValidateStartPayload(tt.args)
			if res.HasPayload != tt.wantHas {
				t.Errorf("HasPayload = %v, want %v", res.HasPayload, tt.wantHas)
			}
			if res.IsValid != tt.wantValid {
				t.Errorf("IsValid = %v, want %v (error: %s)", res.IsValid, tt.wantValid, res.ErrorReason)
			}
			if res.Payload != tt.wantPayload {
				t.Errorf("Payload = %q, want %q", res.Payload, tt.wantPayload)
			}
		})
	}
}

func TestFormatUptime(t *testing.T) {
	now := time.Now()
	// Test zero
	if u := FormatUptime(time.Time{}); u != "0S" {
		t.Errorf("expected 0S for zero time, got %s", u)
	}

	// Test 30 seconds ago
	t30s := now.Add(-30 * time.Second)
	u30 := FormatUptime(t30s)
	if !strings.HasSuffix(u30, "S") {
		t.Errorf("expected seconds format, got %s", u30)
	}

	// Test 2 hours and 15 mins ago
	t2h := now.Add(-2 * time.Hour).Add(-15 * time.Minute)
	u2h := FormatUptime(t2h)
	if !strings.Contains(u2h, "H") || !strings.Contains(u2h, "M") {
		t.Errorf("expected hours and minutes in uptime, got %s", u2h)
	}
}

func TestFormatDurationMs(t *testing.T) {
	d := 15420 * time.Microsecond // 15.420 ms
	str := FormatDurationMs(d)
	if str != "15.420 ms" {
		t.Errorf("expected 15.420 ms, got %s", str)
	}
}

func TestEscapeHTML(t *testing.T) {
	input := `<script>alert("xss & fun")</script>`
	escaped := EscapeHTML(input)
	if strings.Contains(escaped, "<script>") {
		t.Errorf("expected HTML to be escaped, got %s", escaped)
	}
	if !strings.Contains(escaped, "&lt;script&gt;") {
		t.Errorf("expected &lt;script&gt;, got %s", escaped)
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{500, "500 B"},
		{1024, "1.00 KB"},
		{1572864, "1.50 MB"},
		{1073741824, "1.00 GB"},
		{2684354560, "2.50 GB"},
	}

	for _, tt := range tests {
		got := FormatBytes(tt.bytes)
		if got != tt.want {
			t.Errorf("FormatBytes(%d) = %s, want %s", tt.bytes, got, tt.want)
		}
	}
}

