package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/itsafxal/AutoFilter-Go/internal/config"
)

// NewLogger creates a new structured logger writing to the specified writer.
// If w is nil, os.Stdout is used by default.
func NewLogger(level config.LogLevel, w io.Writer) *slog.Logger {
	if w == nil {
		w = os.Stdout
	}

	var slogLevel slog.Level
	switch level {
	case config.LogLevelDebug:
		slogLevel = slog.LevelDebug
	case config.LogLevelInfo:
		slogLevel = slog.LevelInfo
	case config.LogLevelWarn:
		slogLevel = slog.LevelWarn
	case config.LogLevelError:
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: slogLevel,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Redact any attributes that might accidentally contain tokens or credentials
			key := strings.ToLower(a.Key)
			if strings.Contains(key, "token") || strings.Contains(key, "secret") || strings.Contains(key, "password") {
				return slog.String(a.Key, "***REDACTED***")
			}
			return a
		},
	}

	// Use text handler for clean container console logs
	handler := slog.NewTextHandler(w, opts)
	return slog.New(handler)
}
