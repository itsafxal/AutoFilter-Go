package broadcast

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
)

func TestErrorClassification(t *testing.T) {
	// 1. FloodWait Tests
	floodErr1 := errors.New("rpc error: 420 FLOOD_WAIT_35")
	if !isFloodWaitError(floodErr1) {
		t.Errorf("expected isFloodWaitError to be true for %v", floodErr1)
	}
	if sec := extractFloodWaitSeconds(floodErr1); sec != 35 {
		t.Errorf("expected 35 seconds, got %d", sec)
	}

	floodErr2 := &telegram.RpcError{Message: "FLOOD_PREMIUM_WAIT_120"}
	if !isFloodWaitError(floodErr2) {
		t.Errorf("expected isFloodWaitError to be true for %v", floodErr2)
	}
	if sec := extractFloodWaitSeconds(floodErr2); sec != 120 {
		t.Errorf("expected 120 seconds, got %d", sec)
	}

	floodErr3 := errors.New("A wait of 15 seconds is required")
	if !isFloodWaitError(floodErr3) {
		t.Errorf("expected isFloodWaitError to be true for %v", floodErr3)
	}
	if sec := extractFloodWaitSeconds(floodErr3); sec != 15 {
		t.Errorf("expected 15 seconds, got %d", sec)
	}

	// 2. Blocked User Tests
	blockedErr := errors.New("RPCError: 403 USER_IS_BLOCKED: user has blocked the bot")
	if !isUserBlockedError(blockedErr) {
		t.Errorf("expected isUserBlockedError to be true for %v", blockedErr)
	}

	// 3. Deactivated User Tests
	deactErr := errors.New("RPCError: 400 INPUT_USER_DEACTIVATED")
	if !isUserDeactivatedError(deactErr) {
		t.Errorf("expected isUserDeactivatedError to be true for %v", deactErr)
	}

	// 4. Chat Forbidden Tests
	forbidErr := errors.New("RPCError: 403 CHAT_WRITE_FORBIDDEN")
	if !isChatForbiddenError(forbidErr) {
		t.Errorf("expected isChatForbiddenError to be true for %v", forbidErr)
	}

	peerErr := errors.New("RPCError: 400 PEER_ID_INVALID")
	if !isChatForbiddenError(peerErr) {
		t.Errorf("expected isChatForbiddenError to be true for %v", peerErr)
	}

	// 5. Transient Error Tests
	transErr := errors.New("connection reset by peer")
	if !isTransientError(transErr) {
		t.Errorf("expected isTransientError to be true for %v", transErr)
	}
}

func TestFormattingHelpers(t *testing.T) {
	// Test formatNumber
	if got := formatNumber(1234567); got != "1,234,567" {
		t.Errorf("expected 1,234,567, got %s", got)
	}
	if got := formatNumber(500); got != "500" {
		t.Errorf("expected 500, got %s", got)
	}

	// Test renderProgressBar
	bar := renderProgressBar(50, 100, 10)
	if bar != "█████░░░░░" {
		t.Errorf("expected 50%% bar, got %s", bar)
	}

	barFull := renderProgressBar(100, 100, 10)
	if barFull != "██████████" {
		t.Errorf("expected full bar, got %s", barFull)
	}

	// Test formatDuration
	d := 2*time.Hour + 15*time.Minute + 30*time.Second
	if str := formatDuration(d); str != "02h 15m 30s" {
		t.Errorf("expected 02h 15m 30s, got %s", str)
	}
}

func TestFloodCoordinator(t *testing.T) {
	fc := &floodCoordinator{}

	// Initially not paused
	if paused, _ := fc.Status(); paused {
		t.Error("expected floodCoordinator to not be paused initially")
	}

	// Trigger pause
	fc.TriggerPause(2)
	paused, sec := fc.Status()
	if !paused || sec <= 0 {
		t.Errorf("expected paused status with remaining seconds, got paused=%v, sec=%d", paused, sec)
	}

	// Wait with context cancellation should abort
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := fc.WaitIfNeeded(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got %v", err)
	}
}

func TestEngineLocking(t *testing.T) {
	engine := New(nil, nil, nil, 10)
	if engine.IsBroadcasting() {
		t.Error("expected new engine to not be broadcasting")
	}

	// Without valid messages, start should return error
	err := engine.StartBroadcast(context.Background(), nil, nil, Options{})
	if err == nil {
		t.Error("expected error when source message is nil")
	}
	if engine.IsBroadcasting() {
		t.Error("expected engine to remain not broadcasting on validation failure")
	}
}
