package plugins

import (
	"strconv"
	"strings"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
)

// EnsurePeerCached ensures that the message's sender and chat peers are fully resolved and present in Gogram's cache.
// This eliminates the race condition where Gogram processes new incoming messages in parallel with its
// asynchronous peer-fetching (fetchPeersBeforeUpdate), which otherwise causes the first message or command
// from a user to be silently dropped or rejected with "missing from cache".
func EnsurePeerCached(m *telegram.NewMessage, timeout time.Duration) bool {
	if m == nil || m.Client == nil {
		return false
	}

	targetID := m.SenderID()
	if targetID == 0 && m.IsPrivate() {
		targetID = m.ChatID()
	}
	if targetID == 0 && m.Sender != nil && m.Sender.ID != 0 {
		targetID = m.Sender.ID
	}
	if targetID == 0 {
		return false
	}

	client := m.Client
	normID := NormalizePeerID(targetID)

	// If already present in cache, ensure message fields (Sender, Peer, Chat) are properly hydrated
	if client.IdInCache(normID) {
		HydrateMessage(m, targetID)
		return true
	}

	// Gogram automatically invokes fetchPeersBeforeUpdate concurrently on UpdateNewMessage.
	// Poll briefly (every 25ms, up to timeout) to allow that fetch to finish and populate the cache.
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		time.Sleep(25 * time.Millisecond)
		if client.IdInCache(normID) {
			HydrateMessage(m, targetID)
			return true
		}
	}

	// If still not in cache after waiting, actively fetch difference to synchronize peers
	pts := client.ExportPts()
	if pts > 0 {
		_, _ = client.GetDifference(pts, 100)
	} else if state, err := client.UpdatesGetState(); err == nil && state != nil {
		_, _ = client.GetDifference(state.Pts, 100)
	}

	if client.IdInCache(normID) {
		HydrateMessage(m, targetID)
		return true
	}

	return false
}

// NormalizePeerID converts a negative channel ID (-100...) into a positive raw channel ID
// matching how Gogram's InputChannels cache map indexes channel peers.
func NormalizePeerID(id int64) int64 {
	if id < 0 {
		s := strconv.FormatInt(id, 10)
		if strings.HasPrefix(s, "-100") {
			if parsed, err := strconv.ParseInt(strings.TrimPrefix(s, "-100"), 10, 64); err == nil {
				return parsed
			}
		}
		return -id
	}
	return id
}

// HydrateMessage ensures that m.Sender, m.Peer, and m.Chat are populated with full user details
// once the user has been placed in Gogram's cache.
func HydrateMessage(m *telegram.NewMessage, targetID int64) {
	if m == nil || m.Client == nil || targetID == 0 {
		return
	}

	client := m.Client

	// Hydrate Sender if missing or empty (Gogram creates empty &UserObj{} when user was not yet in cache)
	if m.Sender == nil || m.Sender.ID == 0 {
		if user, err := client.GetUser(targetID); err == nil && user != nil && user.ID != 0 {
			m.Sender = user
		}
	}

	// Hydrate Peer if missing
	if m.Peer == nil && m.IsPrivate() {
		if peerUser, err := client.GetPeerUser(targetID); err == nil && peerUser != nil {
			m.Peer = peerUser
		}
	}

	// Hydrate Chat if missing or ID == 0
	if m.IsPrivate() {
		if (m.Chat == nil || m.Chat.ID == 0) && m.Sender != nil && m.Sender.ID != 0 {
			m.Chat = &telegram.ChatObj{
				ID:    m.Sender.ID,
				Title: m.Sender.FirstName,
			}
		}
	}
}
