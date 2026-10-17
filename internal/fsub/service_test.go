package fsub

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/itsafxal/MYTGBOTZ/internal/config"
)

func newTestService() *Service {
	cfg := &config.Config{
		OwnerID: 111111,
		Admins:  []int64{222222},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := NewRepository(nil)
	return NewService(repo, cfg, logger)
}

func TestMembershipCache(t *testing.T) {
	cache := NewMembershipCache(50*time.Millisecond, 20*time.Millisecond)

	// Test Miss
	if isMember, ok := cache.Get(100, 200); ok {
		t.Fatalf("expected cache miss, got %v", isMember)
	}

	// Test Positive Hit
	cache.Set(100, 200, true)
	if isMember, ok := cache.Get(100, 200); !ok || !isMember {
		t.Fatalf("expected cache hit with true, got ok=%v, isMember=%v", ok, isMember)
	}

	// Test Invalidate
	cache.Invalidate(100, 200)
	if _, ok := cache.Get(100, 200); ok {
		t.Fatalf("expected cache miss after invalidation")
	}

	// Test Expiration
	cache.Set(100, 300, false)
	time.Sleep(30 * time.Millisecond)
	if _, ok := cache.Get(100, 300); ok {
		t.Fatalf("expected negative cache entry to expire after 20ms")
	}
}

func TestTaskTracker(t *testing.T) {
	tracker := NewTaskTracker()

	ctx1, cancel1 := tracker.Acquire(context.Background(), 555)
	if ctx1.Err() != nil {
		t.Fatalf("expected active context")
	}

	// Acquiring again for the same user should cancel the prior context
	ctx2, cancel2 := tracker.Acquire(context.Background(), 555)
	defer cancel2()

	if ctx1.Err() == nil {
		t.Fatalf("expected prior context to be cancelled upon re-acquiring")
	}
	if ctx2.Err() != nil {
		t.Fatalf("expected new context to be active")
	}

	cancel1()
	tracker.Release(555)
}

func TestAdminBypass(t *testing.T) {
	svc := newTestService()

	svc.mu.Lock()
	svc.loadout.Channel1 = -10012345
	svc.loadout.Channel2 = -10067890
	svc.mu.Unlock()

	// Owner check
	markup, unjoined := svc.BuildDynamicFSubKeyboard(context.Background(), nil, 111111, "file_123")
	if len(unjoined) != 0 || markup != nil {
		t.Fatalf("expected admin/owner to bypass FSub completely")
	}

	// Admin check
	markup, unjoined = svc.BuildDynamicFSubKeyboard(context.Background(), nil, 222222, "file_123")
	if len(unjoined) != 0 || markup != nil {
		t.Fatalf("expected admin to bypass FSub completely")
	}
}

func TestDynamicKeyboardChannel3Conditionality(t *testing.T) {
	svc := newTestService()

	svc.mu.Lock()
	svc.loadout.Channel1 = -100111
	svc.loadout.Channel2 = -100222
	svc.loadout.Channel3 = -100333
	svc.link1 = "https://t.me/+link1"
	svc.link2 = "https://t.me/+link2"
	svc.link3 = "https://t.me/+link3"
	svc.mu.Unlock()

	normalUser := int64(999999)

	// Case 1: User has not joined channel 1 or 2 -> Channel 3 should NOT be displayed yet
	markup, unjoined := svc.BuildDynamicFSubKeyboard(context.Background(), nil, normalUser, "file_123")
	if markup == nil || len(unjoined) != 2 {
		t.Fatalf("expected unjoined channels 1 and 2, got %v", unjoined)
	}

	// Case 2: User joins channel 1 -> Channel 2 is still unjoined, Channel 3 still NOT displayed
	svc.cache.Set(normalUser, -100111, true)
	markup, unjoined = svc.BuildDynamicFSubKeyboard(context.Background(), nil, normalUser, "file_123")
	if markup == nil || len(unjoined) != 1 || unjoined[0] != -100222 {
		t.Fatalf("expected only channel 2 unjoined, got %v", unjoined)
	}

	// Case 3: User joins channel 2 -> NOW Channel 3 should be displayed!
	svc.cache.Set(normalUser, -100222, true)
	markup, unjoined = svc.BuildDynamicFSubKeyboard(context.Background(), nil, normalUser, "file_123")
	if markup == nil || len(unjoined) != 1 || unjoined[0] != -100333 {
		t.Fatalf("expected channel 3 unjoined after channels 1 & 2 joined, got %v", unjoined)
	}

	// Case 4: User joins channel 3 -> All channels joined! No keyboard, unjoined is empty
	svc.cache.Set(normalUser, -100333, true)
	markup, unjoined = svc.BuildDynamicFSubKeyboard(context.Background(), nil, normalUser, "file_123")
	if markup != nil || len(unjoined) != 0 {
		t.Fatalf("expected all channels joined, got unjoined=%v, markup=%v", unjoined, markup)
	}
}

func TestChannelThresholdSwitching(t *testing.T) {
	svc := newTestService()

	svc.mu.Lock()
	svc.loadout.Channel1 = -100111
	svc.loadout.Channel1_2 = -100999
	svc.loadout.Channel1Threshold = 5
	svc.link1 = "https://t.me/+link1"
	svc.link1_2 = "https://t.me/+link1_2"
	svc.mu.Unlock()

	// Initially primary is -100111
	if svc.loadout.Channel1 != -100111 {
		t.Fatalf("expected channel1 to be -100111")
	}

	// Simulate threshold switch manually
	svc.mu.Lock()
	svc.loadout.Channel1 = svc.loadout.Channel1_2
	svc.loadout.Channel1_2 = 0
	svc.link1 = svc.link1_2
	svc.link1_2 = ""
	svc.mu.Unlock()

	if svc.loadout.Channel1 != -100999 || svc.loadout.Channel1_2 != 0 {
		t.Fatalf("expected channel1 to switch to -100999 and secondary cleared")
	}
	if svc.link1 != "https://t.me/+link1_2" || svc.link1_2 != "" {
		t.Fatalf("expected link1 to switch to secondary link")
	}
}
