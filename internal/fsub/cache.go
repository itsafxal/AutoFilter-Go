package fsub

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type cacheEntry struct {
	isMember bool
	expireAt time.Time
}

// MembershipCache provides a short-lived TTL cache for channel membership status.
// Prevents Telegram GetParticipant flood limits while reacting quickly to status changes.
type MembershipCache struct {
	mu          sync.RWMutex
	cache       map[string]cacheEntry
	positiveTTL time.Duration
	negativeTTL time.Duration
}

// NewMembershipCache creates a new MembershipCache with defaults matching legacy bot (60s positive, 8s negative).
func NewMembershipCache(positiveTTL, negativeTTL time.Duration) *MembershipCache {
	if positiveTTL <= 0 {
		positiveTTL = 60 * time.Second
	}
	if negativeTTL <= 0 {
		negativeTTL = 8 * time.Second
	}
	return &MembershipCache{
		cache:       make(map[string]cacheEntry),
		positiveTTL: positiveTTL,
		negativeTTL: negativeTTL,
	}
}

func cacheKey(userID, channelID int64) string {
	return fmt.Sprintf("%d:%d", userID, channelID)
}

// Get checks if membership status is cached and not expired.
func (c *MembershipCache) Get(userID, channelID int64) (bool, bool) {
	if userID == 0 || channelID == 0 {
		return false, false
	}
	c.mu.RLock()
	entry, exists := c.cache[cacheKey(userID, channelID)]
	c.mu.RUnlock()

	if !exists {
		return false, false
	}

	if time.Now().After(entry.expireAt) {
		c.mu.Lock()
		delete(c.cache, cacheKey(userID, channelID))
		c.mu.Unlock()
		return false, false
	}

	return entry.isMember, true
}

// Set stores membership status with appropriate TTL.
func (c *MembershipCache) Set(userID, channelID int64, isMember bool) {
	if userID == 0 || channelID == 0 {
		return
	}
	ttl := c.negativeTTL
	if isMember {
		ttl = c.positiveTTL
	}

	c.mu.Lock()
	c.cache[cacheKey(userID, channelID)] = cacheEntry{
		isMember: isMember,
		expireAt: time.Now().Add(ttl),
	}
	c.mu.Unlock()
}

// Invalidate clears cache entries for a user.
func (c *MembershipCache) Invalidate(userID int64, channelIDs ...int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(channelIDs) > 0 {
		for _, cid := range channelIDs {
			delete(c.cache, cacheKey(userID, cid))
		}
	} else {
		prefix := fmt.Sprintf("%d:", userID)
		for k := range c.cache {
			if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
				delete(c.cache, k)
			}
		}
	}
}

// TaskTracker coordinates active background FSub check loops per user.
// Cancels existing tasks if a user re-triggers /start.
type TaskTracker struct {
	mu    sync.Mutex
	tasks map[int64]context.CancelFunc
}

// NewTaskTracker creates a new TaskTracker instance.
func NewTaskTracker() *TaskTracker {
	return &TaskTracker{
		tasks: make(map[int64]context.CancelFunc),
	}
}

// Acquire cancels any existing loop for this user and returns a new context and cancel func.
func (t *TaskTracker) Acquire(parent context.Context, userID int64) (context.Context, context.CancelFunc) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if cancel, exists := t.tasks[userID]; exists {
		cancel()
	}

	ctx, cancel := context.WithCancel(parent)
	t.tasks[userID] = cancel
	return ctx, cancel
}

// Release removes the user's task entry if it matches.
func (t *TaskTracker) Release(userID int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.tasks, userID)
}
