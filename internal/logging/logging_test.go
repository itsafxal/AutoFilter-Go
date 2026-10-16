package logging

import (
	"bytes"
	"strings"
	"testing"

	"github.com/itsafxal/MYTGBOTZ/internal/config"
)

func TestNewLogger_Levels(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewLogger(config.LogLevelWarn, buf)

	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message")

	output := buf.String()
	if strings.Contains(output, "debug message") {
		t.Error("debug message should not appear when level is WARN")
	}
	if strings.Contains(output, "info message") {
		t.Error("info message should not appear when level is WARN")
	}
	if !strings.Contains(output, "warn message") {
		t.Error("warn message should appear when level is WARN")
	}
	if !strings.Contains(output, "error message") {
		t.Error("error message should appear when level is WARN")
	}
}

func TestNewLogger_Redaction(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewLogger(config.LogLevelInfo, buf)

	logger.Info("bot initialized", "bot_token", "123456:secret-token-value")
	output := buf.String()

	if strings.Contains(output, "secret-token-value") {
		t.Fatalf("CRITICAL: Token leaked in log output: %s", output)
	}
	if !strings.Contains(output, "***REDACTED***") {
		t.Fatalf("Expected ***REDACTED*** in log output, got: %s", output)
	}
}
