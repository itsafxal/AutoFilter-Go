package commands

import (
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// payloadRegex validates Telegram deep-link parameter:
// Must be between 1 and 64 characters consisting only of alphanumeric, hyphen, or underscore.
var payloadRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// PayloadValidationResult holds the result of validating a deep-link payload.
type PayloadValidationResult struct {
	HasPayload  bool
	IsValid     bool
	Payload     string
	ErrorReason string
}

// ValidateStartPayload safely validates a Telegram /start parameter payload.
// Returns an error reason if the payload is invalid or exceeds safe limits.
func ValidateStartPayload(args []string) PayloadValidationResult {
	if len(args) <= 1 {
		return PayloadValidationResult{HasPayload: false, IsValid: true}
	}

	raw := strings.TrimSpace(args[1])
	if raw == "" {
		return PayloadValidationResult{HasPayload: false, IsValid: true}
	}

	if len(raw) > 64 {
		return PayloadValidationResult{
			HasPayload:  true,
			IsValid:     false,
			Payload:     raw,
			ErrorReason: "payload exceeds maximum permitted length of 64 characters",
		}
	}

	if !payloadRegex.MatchString(raw) {
		return PayloadValidationResult{
			HasPayload:  true,
			IsValid:     false,
			Payload:     raw,
			ErrorReason: "payload contains illegal characters (only alphanumeric, hyphens, and underscores are allowed)",
		}
	}

	return PayloadValidationResult{
		HasPayload: true,
		IsValid:    true,
		Payload:    raw,
	}
}

// FormatUptime returns a human-readable uptime string matching the format from TGxCyberBot:
// e.g. "1W : 2D : 3H : 4M : 5S"
func FormatUptime(startTime time.Time) string {
	if startTime.IsZero() {
		return "0S"
	}
	uptime := time.Since(startTime)
	if uptime < 0 {
		uptime = 0
	}

	totalSeconds := int64(uptime.Seconds())
	weeks := totalSeconds / (7 * 24 * 3600)
	remDays := (totalSeconds % (7 * 24 * 3600)) / (24 * 3600)
	remHours := (totalSeconds % (24 * 3600)) / 3600
	remMinutes := (totalSeconds % 3600) / 60
	remSeconds := totalSeconds % 60

	parts := make([]string, 0, 5)
	if weeks > 0 {
		parts = append(parts, fmt.Sprintf("%dW", weeks))
	}
	if remDays > 0 || len(parts) > 0 {
		parts = append(parts, fmt.Sprintf("%dD", remDays))
	}
	if remHours > 0 || len(parts) > 0 {
		parts = append(parts, fmt.Sprintf("%dH", remHours))
	}
	if remMinutes > 0 || len(parts) > 0 {
		parts = append(parts, fmt.Sprintf("%dM", remMinutes))
	}
	parts = append(parts, fmt.Sprintf("%dS", remSeconds))

	return strings.Join(parts, " : ")
}

// FormatDurationMs formats a duration cleanly in milliseconds with 3 decimal precision.
func FormatDurationMs(d time.Duration) string {
	ms := float64(d.Nanoseconds()) / 1e6
	return fmt.Sprintf("%.3f ms", ms)
}

// EscapeHTML escapes untrusted text for safe HTML formatting in Telegram messages.
func EscapeHTML(s string) string {
	return html.EscapeString(s)
}

// FormatBytes formats a byte count into a human-readable size string (e.g., "1.45 GB", "500.00 MB").
func FormatBytes(b int64) string {
	const unit = 1024
	if b < 0 {
		b = 0
	}
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// IsValidTelegramButtonURL verifies if a URL can safely be used in a Telegram InlineKeyboardButton URL.
// Telegram requires public HTTP/HTTPS URLs and strictly rejects localhost, loopback IPs, or invalid formats with BUTTON_URL_INVALID.
func IsValidTelegramButtonURL(rawURL string) bool {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return false
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || host == "127.0.0.1" || host == "0.0.0.0" || host == "::1" || strings.HasSuffix(host, ".local") {
		return false
	}
	return true
}
