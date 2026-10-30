package batch

import (
	"sync"
	"time"
)

// Session represents an in-progress interactive batch creation session.
type Session struct {
	BatchID         string
	BatchName       string
	OwnerID         int64
	ChatID          int64
	StatusMessageID int32
	CurrentSequence int
	Items           []*BatchItem
	TempMessageIDs  []int32
	TotalItems      int
	TotalFiles      int
	TotalTexts      int
	TotalDocuments  int
	TotalVideos     int
	TotalAudio      int
	TotalPhotos     int
	CreatedAt       time.Time
	LastActivity    time.Time
	IsFinalizing    bool
	IsDone          bool
	mu              sync.Mutex
}

// AddItem appends an item to the session, assigning the next sequential index.
func (s *Session) AddItem(item *BatchItem, tempMsgID int32) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.CurrentSequence++
	item.Sequence = s.CurrentSequence
	item.BatchID = s.BatchID
	item.CreatedAt = time.Now().UTC()

	s.Items = append(s.Items, item)
	s.TotalItems++

	switch item.ItemType {
	case ItemTypeDocument:
		s.TotalFiles++
		s.TotalDocuments++
	case ItemTypeVideo:
		s.TotalFiles++
		s.TotalVideos++
	case ItemTypeAudio, ItemTypeVoice:
		s.TotalFiles++
		s.TotalAudio++
	case ItemTypePhoto:
		s.TotalFiles++
		s.TotalPhotos++
	case ItemTypeText:
		s.TotalTexts++
	default:
		s.TotalFiles++
	}

	if tempMsgID != 0 {
		s.TempMessageIDs = append(s.TempMessageIDs, tempMsgID)
	}

	s.LastActivity = time.Now()
	return s.CurrentSequence
}

// BeginFinalize attempts to transition the session into the finalizing state.
// Returns false if the session is already finalizing or finalized.
func (s *Session) BeginFinalize() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.IsDone || s.IsFinalizing {
		return false
	}
	s.IsFinalizing = true
	return true
}

// MarkDone marks the session as successfully finalized.
func (s *Session) MarkDone() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.IsDone = true
	s.IsFinalizing = false
}

// AbortFinalize reverts the finalizing state if an error occurred during persistence.
func (s *Session) AbortFinalize() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.IsFinalizing = false
}

// GetSnapshot returns a thread-safe snapshot of session metadata and items.
func (s *Session) GetSnapshot() (*BatchMetadata, []*BatchItem, []int32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta := &BatchMetadata{
		BatchID:        s.BatchID,
		BatchName:      s.BatchName,
		NormalizedName: NormalizeBatchName(s.BatchName),
		OwnerID:        s.OwnerID,
		TotalItems:     s.TotalItems,
		TotalFiles:     s.TotalFiles,
		TotalTexts:     s.TotalTexts,
		TotalDocuments: s.TotalDocuments,
		TotalVideos:    s.TotalVideos,
		TotalAudio:     s.TotalAudio,
		TotalPhotos:    s.TotalPhotos,
		Status:         StatusCollecting,
		CreatedAt:      s.CreatedAt,
		UpdatedAt:      time.Now().UTC(),
	}

	itemsCopy := make([]*BatchItem, len(s.Items))
	copy(itemsCopy, s.Items)

	tempIDsCopy := make([]int32, len(s.TempMessageIDs))
	copy(tempIDsCopy, s.TempMessageIDs)

	return meta, itemsCopy, tempIDsCopy
}

// SessionManager manages active interactive batch sessions across all bot users.
type SessionManager struct {
	sessions    map[int64]*Session // keyed by UserID
	pendingName map[int64]int64    // UserID -> ChatID (waiting for batch name input)
	mu          sync.RWMutex
}

// NewSessionManager creates a new SessionManager.
func NewSessionManager() *SessionManager {
	sm := &SessionManager{
		sessions:    make(map[int64]*Session),
		pendingName: make(map[int64]int64),
	}
	go sm.cleanupRoutine()
	return sm
}

// SetPendingName records that a user was asked for a batch name.
func (sm *SessionManager) SetPendingName(userID, chatID int64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.pendingName[userID] = chatID
}

// ClearPendingName removes the pending name expectation for a user.
func (sm *SessionManager) ClearPendingName(userID int64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.pendingName, userID)
}

// IsPendingName checks if a user is currently expected to provide a batch name.
func (sm *SessionManager) IsPendingName(userID int64) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	_, ok := sm.pendingName[userID]
	return ok
}

// StartSession creates and registers a new active batch session for a user.
func (sm *SessionManager) StartSession(userID, chatID int64, batchName string) *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	delete(sm.pendingName, userID)

	sess := &Session{
		BatchID:      GenerateBatchID(),
		BatchName:    batchName,
		OwnerID:      userID,
		ChatID:       chatID,
		CreatedAt:    time.Now().UTC(),
		LastActivity: time.Now(),
		Items:        make([]*BatchItem, 0),
	}

	sm.sessions[userID] = sess
	return sess
}

// GetSession retrieves the active batch session for a user, or nil if none exists.
func (sm *SessionManager) GetSession(userID int64) *Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.sessions[userID]
}

// RemoveSession drops the session for a user.
func (sm *SessionManager) RemoveSession(userID int64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, userID)
	delete(sm.pendingName, userID)
}

// GetSessionByBatchID retrieves a session matching a specific BatchID.
func (sm *SessionManager) GetSessionByBatchID(batchID string) *Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	for _, sess := range sm.sessions {
		if sess.BatchID == batchID {
			return sess
		}
	}
	return nil
}

// cleanupRoutine periodically purges sessions inactive for more than 30 minutes.
func (sm *SessionManager) cleanupRoutine() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		sm.mu.Lock()
		now := time.Now()
		for uid, sess := range sm.sessions {
			if now.Sub(sess.LastActivity) > 30*time.Minute {
				delete(sm.sessions, uid)
			}
		}
		sm.mu.Unlock()
	}
}
