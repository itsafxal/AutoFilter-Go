package delivery

import (
	"fmt"
	"sync"
	"time"
)

// InFlightTracker coordinates concurrent delivery requests to prevent duplicate file deliveries
// caused by repeated clicks, double taps, or duplicate callback submissions.
type InFlightTracker struct {
	mu      sync.Mutex
	active  map[string]time.Time
	timeout time.Duration
}

// NewInFlightTracker creates a new InFlightTracker with the given lease timeout.
func NewInFlightTracker(timeout time.Duration) *InFlightTracker {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &InFlightTracker{
		active:  make(map[string]time.Time),
		timeout: timeout,
	}
}

// Acquire attempts to acquire a lock for the given user and file identifier.
// Returns true if acquired, or false if an identical delivery is currently in-flight.
func (t *InFlightTracker) Acquire(userID int64, fileID string) bool {
	if t == nil || userID <= 0 || fileID == "" {
		return true
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Lazy cleanup of expired locks
	now := time.Now()
	for k, ts := range t.active {
		if now.Sub(ts) > t.timeout {
			delete(t.active, k)
		}
	}

	key := fmt.Sprintf("%d:%s", userID, fileID)
	if _, exists := t.active[key]; exists {
		return false
	}

	t.active[key] = now
	return true
}

// Release frees the lock for the given user and file identifier.
func (t *InFlightTracker) Release(userID int64, fileID string) {
	if t == nil || userID <= 0 || fileID == "" {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	key := fmt.Sprintf("%d:%s", userID, fileID)
	delete(t.active, key)
}

// IsInFlight checks whether an identical delivery request is currently marked active.
func (t *InFlightTracker) IsInFlight(userID int64, fileID string) bool {
	if t == nil || userID <= 0 || fileID == "" {
		return false
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	key := fmt.Sprintf("%d:%s", userID, fileID)
	ts, exists := t.active[key]
	if !exists {
		return false
	}

	if time.Since(ts) > t.timeout {
		delete(t.active, key)
		return false
	}

	return true
}
