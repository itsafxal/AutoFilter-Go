package autodelete

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/AutoFilter-Go/internal/config"
	"go.mongodb.org/mongo-driver/mongo"
)

// Settings represents the runtime auto-delete and weekly trash configuration.
type Settings struct {
	FilterDeleteDuration       time.Duration
	FileDeleteDuration         time.Duration
	FastDownloadDeleteDuration time.Duration
	WeeklyTrashEnabled         bool
	TrashRetentionDays         int
}

// ScheduledItem represents a message scheduled for deletion.
type ScheduledItem struct {
	ID        int64
	ChatID    int64
	MessageID int32
	MsgType   string
	DeleteAt  time.Time
	CreatedAt time.Time
}

// TrashStats contains statistics on auto-delete and weekly trash operations.
type TrashStats struct {
	PendingDeletionsCount int64
	TrackedMessagesCount  int64
	LastCleanedAt         time.Time
	LastDeletedCount      int64
}

// AutoDeleteService defines the contract for auto-delete and weekly trash management.
type AutoDeleteService interface {
	Init(ctx context.Context, client *telegram.Client) error
	Start(ctx context.Context)
	Stop()

	ScheduleDelete(chatID int64, messageID int32, msgType string, duration time.Duration) error
	ScheduleFilterDelete(chatID int64, messageID int32) error
	ScheduleFileDelete(chatID int64, messageID int32) error
	ScheduleFastDownloadDelete(chatID int64, messageID int32) error
	TrackSentMessage(chatID int64, messageID int32, msgType string) error

	GetSettings() Settings
	SetFilterDuration(ctx context.Context, d time.Duration) error
	SetFileDuration(ctx context.Context, d time.Duration) error
	SetFastDownloadDuration(ctx context.Context, d time.Duration) error
	SetWeeklyTrash(ctx context.Context, enabled bool) error
	SetTrashRetention(ctx context.Context, days int) error

	RunWeeklyTrash(ctx context.Context) (int64, error)
	GetStats(ctx context.Context) (TrashStats, error)
}

// Service implements AutoDeleteService with SQLite persistence across restarts.
type Service struct {
	cfg           *config.Config
	sqlDBProvider func() *sql.DB
	mongoProvider func() *mongo.Database
	logger        *slog.Logger
	client        *telegram.Client

	settings  Settings
	lastTrash time.Time
	lastCount int64
	wakeCh    chan struct{}
	stopCh    chan struct{}
	stopped   bool
	mu        sync.RWMutex
}

// NewService constructs a new AutoDeleteService.
func NewService(
	cfg *config.Config,
	sqlProv func() *sql.DB,
	mongoProv func() *mongo.Database,
	logger *slog.Logger,
) *Service {
	filterDur, _ := ParseDuration(cfg.AutoDeleteFilterTime)
	fileDur, _ := ParseDuration(cfg.AutoDeleteFileTime)
	fastDLDur, _ := ParseDuration(cfg.AutoDeleteFastDownloadTime)
	if filterDur <= 0 && cfg.AutoDeleteFilterTime == "" {
		filterDur = 15 * time.Minute
	}
	if fileDur <= 0 && cfg.AutoDeleteFileTime == "" {
		fileDur = 30 * time.Minute
	}
	if fastDLDur <= 0 {
		fastDLDur = 5 * time.Minute
	}
	retentionDays := cfg.TrashRetentionDays
	if retentionDays <= 0 {
		retentionDays = 7
	}

	return &Service{
		cfg:           cfg,
		sqlDBProvider: sqlProv,
		mongoProvider: mongoProv,
		logger:        logger,
		settings: Settings{
			FilterDeleteDuration:       filterDur,
			FileDeleteDuration:         fileDur,
			FastDownloadDeleteDuration: fastDLDur,
			WeeklyTrashEnabled:         cfg.AutoDeleteWeeklyTrash,
			TrashRetentionDays:         retentionDays,
		},
		wakeCh: make(chan struct{}, 1),
		stopCh: make(chan struct{}),
	}
}

// Init loads persisted settings, registers the Telegram client, and purges expired messages immediately on restart.
func (s *Service) Init(ctx context.Context, client *telegram.Client) error {
	s.mu.Lock()
	s.client = client
	s.mu.Unlock()

	// Load persisted runtime overrides from SQLite
	s.loadSettings(ctx)

	// CRITICAL REQUIREMENT: "bot restart aayi kazhinjaalum delete aavanm"
	// Immediately purge any scheduled deletions whose delete_at expired while bot was offline
	deletedCount, err := s.processScheduledDeletions(ctx)
	if err != nil {
		s.logger.Warn("error purging expired scheduled deletions on startup", "error", err)
	} else if deletedCount > 0 {
		s.logger.Info("recovered and purged expired messages on restart", "count", deletedCount)
	}

	return nil
}

// Start launches the background auto-delete worker and periodic weekly trash cleanup loops.
func (s *Service) Start(ctx context.Context) {
	go s.runAutoDeleteLoop(ctx)
	go s.runWeeklyTrashLoop(ctx)
}

// Stop terminates background workers cleanly.
func (s *Service) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	close(s.stopCh)
	s.mu.Unlock()
}

// ScheduleDelete registers a message to be automatically deleted after the specified duration.
// Records are persisted immediately in SQLite so they survive bot restarts.
func (s *Service) ScheduleDelete(chatID int64, messageID int32, msgType string, duration time.Duration) error {
	if chatID == 0 || messageID == 0 {
		return nil
	}

	// Always track in sent_messages for weekly trash cleanup
	_ = s.TrackSentMessage(chatID, messageID, msgType)

	if duration <= 0 {
		return nil
	}

	db := s.getDB()
	if db == nil {
		return nil
	}

	now := time.Now()
	deleteAt := now.Add(duration)

	query := `
		INSERT OR REPLACE INTO scheduled_deletions (chat_id, message_id, msg_type, delete_at, created_at)
		VALUES (?, ?, ?, ?, ?);
	`
	_, err := db.Exec(query, chatID, messageID, msgType, deleteAt, now)
	if err != nil {
		s.logger.Error("failed to persist scheduled deletion",
			"chat_id", chatID,
			"message_id", messageID,
			"msg_type", msgType,
			"delete_at", deleteAt,
			"error", err,
		)
		return err
	}

	s.logger.Debug("scheduled message for auto-deletion",
		"chat_id", chatID,
		"message_id", messageID,
		"msg_type", msgType,
		"delete_in", duration,
	)

	// Wake worker loop if needed
	select {
	case s.wakeCh <- struct{}{}:
	default:
	}

	return nil
}

// ScheduleFilterDelete schedules auto-deletion of a search result / filter response message using active filter duration.
func (s *Service) ScheduleFilterDelete(chatID int64, messageID int32) error {
	s.mu.RLock()
	dur := s.settings.FilterDeleteDuration
	s.mu.RUnlock()
	return s.ScheduleDelete(chatID, messageID, "filter", dur)
}

// ScheduleFileDelete schedules auto-deletion of a delivered media file message using active file duration.
func (s *Service) ScheduleFileDelete(chatID int64, messageID int32) error {
	s.mu.RLock()
	dur := s.settings.FileDeleteDuration
	s.mu.RUnlock()
	return s.ScheduleDelete(chatID, messageID, "file", dur)
}

// ScheduleFastDownloadDelete schedules auto-deletion of a fast download link response message using active fast download duration.
func (s *Service) ScheduleFastDownloadDelete(chatID int64, messageID int32) error {
	s.mu.RLock()
	dur := s.settings.FastDownloadDeleteDuration
	s.mu.RUnlock()
	if dur <= 0 {
		dur = 5 * time.Minute
	}
	return s.ScheduleDelete(chatID, messageID, "fast_download", dur)
}

// TrackSentMessage logs every message sent by the bot across chats for Weekly Trash tracking.
func (s *Service) TrackSentMessage(chatID int64, messageID int32, msgType string) error {
	if chatID == 0 || messageID == 0 {
		return nil
	}
	db := s.getDB()
	if db == nil {
		return nil
	}

	query := `
		INSERT OR IGNORE INTO sent_messages (chat_id, message_id, msg_type, sent_at)
		VALUES (?, ?, ?, ?);
	`
	_, err := db.Exec(query, chatID, messageID, msgType, time.Now())
	return err
}

// GetSettings returns a snapshot of active auto-delete and trash configuration.
func (s *Service) GetSettings() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings
}

// SetFilterDuration updates and persists the filter auto-delete duration.
func (s *Service) SetFilterDuration(ctx context.Context, d time.Duration) error {
	s.mu.Lock()
	s.settings.FilterDeleteDuration = d
	s.mu.Unlock()
	return s.saveSetting(ctx, "filter_delete_duration", d.String())
}

// SetFileDuration updates and persists the file auto-delete duration.
func (s *Service) SetFileDuration(ctx context.Context, d time.Duration) error {
	s.mu.Lock()
	s.settings.FileDeleteDuration = d
	s.mu.Unlock()
	return s.saveSetting(ctx, "file_delete_duration", d.String())
}

// SetFastDownloadDuration updates and persists the fast download message auto-delete duration.
func (s *Service) SetFastDownloadDuration(ctx context.Context, d time.Duration) error {
	s.mu.Lock()
	s.settings.FastDownloadDeleteDuration = d
	s.mu.Unlock()
	return s.saveSetting(ctx, "fast_download_delete_duration", d.String())
}

// SetWeeklyTrash enables or disables automated weekly trash cleanup.
func (s *Service) SetWeeklyTrash(ctx context.Context, enabled bool) error {
	s.mu.Lock()
	s.settings.WeeklyTrashEnabled = enabled
	s.mu.Unlock()
	return s.saveSetting(ctx, "weekly_trash_enabled", strconv.FormatBool(enabled))
}

// SetTrashRetention updates the message retention period in days for weekly trash.
func (s *Service) SetTrashRetention(ctx context.Context, days int) error {
	if days <= 0 {
		return fmt.Errorf("retention days must be positive")
	}
	s.mu.Lock()
	s.settings.TrashRetentionDays = days
	s.mu.Unlock()
	return s.saveSetting(ctx, "trash_retention_days", strconv.Itoa(days))
}

// RunWeeklyTrash purges all tracked bot messages older than the retention threshold across all chats.
func (s *Service) RunWeeklyTrash(ctx context.Context) (int64, error) {
	db := s.getDB()
	if db == nil {
		return 0, nil
	}

	s.mu.RLock()
	retentionDays := s.settings.TrashRetentionDays
	client := s.client
	s.mu.RUnlock()

	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	rows, err := db.QueryContext(ctx, `
		SELECT id, chat_id, message_id
		FROM sent_messages
		WHERE sent_at <= ?
		ORDER BY sent_at ASC
		LIMIT 1000;
	`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed querying trash messages: %w", err)
	}
	defer rows.Close()

	type msgRecord struct {
		id        int64
		chatID    int64
		messageID int32
	}

	var records []msgRecord
	for rows.Next() {
		var r msgRecord
		if err := rows.Scan(&r.id, &r.chatID, &r.messageID); err == nil {
			records = append(records, r)
		}
	}

	if len(records) == 0 {
		return 0, nil
	}

	// Group by chat ID for batched Telegram deletion (up to 100 per call)
	byChat := make(map[int64][]int32)
	idMap := make(map[int64][]int64) // chatID -> list of DB record IDs
	for _, rec := range records {
		byChat[rec.chatID] = append(byChat[rec.chatID], rec.messageID)
		idMap[rec.chatID] = append(idMap[rec.chatID], rec.id)
	}

	var totalDeleted int64
	for chatID, msgIDs := range byChat {
		recordIDs := idMap[chatID]

		// Chunk by 100 (Telegram limit per DeleteMessages call)
		for i := 0; i < len(msgIDs); i += 100 {
			end := i + 100
			if end > len(msgIDs) {
				end = len(msgIDs)
			}
			chunk := msgIDs[i:end]
			dbChunk := recordIDs[i:end]

			if client != nil {
				_, _ = client.DeleteMessages(chatID, chunk)
			}

			// Prune from SQLite sent_messages
			s.deleteSentMessageRecords(dbChunk)
			totalDeleted += int64(len(chunk))
		}
	}

	// Purge any remaining expired records past the cutoff
	_, _ = db.ExecContext(ctx, "DELETE FROM sent_messages WHERE sent_at <= ?;", cutoff)

	// Safety cap: keep at most 20,000 newest records to strictly prevent multi-gigabyte disk bloat over days
	_, _ = db.ExecContext(ctx, "DELETE FROM sent_messages WHERE id NOT IN (SELECT id FROM sent_messages ORDER BY id DESC LIMIT 20000);")

	s.mu.Lock()
	s.lastTrash = time.Now()
	s.lastCount = totalDeleted
	s.mu.Unlock()

	s.logger.Info("weekly trash cleanup cycle executed",
		"deleted_count", totalDeleted,
		"cutoff", cutoff,
	)

	return totalDeleted, nil
}

// GetStats returns current auto-delete and trash metrics.
func (s *Service) GetStats(ctx context.Context) (TrashStats, error) {
	db := s.getDB()
	var stats TrashStats
	s.mu.RLock()
	stats.LastCleanedAt = s.lastTrash
	stats.LastDeletedCount = s.lastCount
	s.mu.RUnlock()

	if db == nil {
		return stats, nil
	}

	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM scheduled_deletions;").Scan(&stats.PendingDeletionsCount)
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sent_messages;").Scan(&stats.TrackedMessagesCount)
	return stats, nil
}

// runAutoDeleteLoop processes scheduled deletions with low CPU idle overhead.
func (s *Service) runAutoDeleteLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			_, _ = s.processScheduledDeletions(ctx)
		case <-s.wakeCh:
			_, _ = s.processScheduledDeletions(ctx)
		}
	}
}

// runWeeklyTrashLoop executes recurring weekly trash cleanup.
func (s *Service) runWeeklyTrashLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.mu.RLock()
			enabled := s.settings.WeeklyTrashEnabled
			s.mu.RUnlock()

			if enabled {
				_, _ = s.RunWeeklyTrash(ctx)
			}
		}
	}
}

// processScheduledDeletions queries and executes all deletions whose delete_at timestamp has arrived.
func (s *Service) processScheduledDeletions(ctx context.Context) (int64, error) {
	db := s.getDB()
	if db == nil {
		return 0, nil
	}

	s.mu.RLock()
	client := s.client
	s.mu.RUnlock()

	rows, err := db.QueryContext(ctx, `
		SELECT id, chat_id, message_id, msg_type, delete_at
		FROM scheduled_deletions
		WHERE delete_at <= ?
		ORDER BY delete_at ASC
		LIMIT 100;
	`, time.Now())
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var items []ScheduledItem
	for rows.Next() {
		var item ScheduledItem
		if err := rows.Scan(&item.ID, &item.ChatID, &item.MessageID, &item.MsgType, &item.DeleteAt); err == nil {
			items = append(items, item)
		}
	}

	if len(items) == 0 {
		return 0, nil
	}

	// Group by chat ID for efficient batch deletion
	byChat := make(map[int64][]int32)
	idList := make([]int64, 0, len(items))

	for _, it := range items {
		byChat[it.ChatID] = append(byChat[it.ChatID], it.MessageID)
		idList = append(idList, it.ID)
	}

	for chatID, msgIDs := range byChat {
		if client != nil {
			_, _ = client.DeleteMessages(chatID, msgIDs)
		}
	}

	// Prune from scheduled_deletions
	s.deleteScheduledRecords(idList)

	// Also prune from sent_messages in batch so we don't attempt to delete them again during weekly trash
	if len(items) > 0 {
		var b strings.Builder
		b.WriteString("DELETE FROM sent_messages WHERE (chat_id, message_id) IN (")
		args := make([]any, 0, len(items)*2)
		for i, it := range items {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString("(?,?)")
			args = append(args, it.ChatID, it.MessageID)
		}
		b.WriteString(");")
		_, _ = db.ExecContext(ctx, b.String(), args...)
	}

	s.logger.Debug("processed expired scheduled deletions", "count", len(items))
	return int64(len(items)), nil
}

func (s *Service) deleteScheduledRecords(ids []int64) {
	if len(ids) == 0 {
		return
	}
	db := s.getDB()
	if db == nil {
		return
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf("DELETE FROM scheduled_deletions WHERE id IN (%s);", strings.Join(placeholders, ","))
	_, _ = db.Exec(query, args...)
}

func (s *Service) deleteSentMessageRecords(ids []int64) {
	if len(ids) == 0 {
		return
	}
	db := s.getDB()
	if db == nil {
		return
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf("DELETE FROM sent_messages WHERE id IN (%s);", strings.Join(placeholders, ","))
	_, _ = db.Exec(query, args...)
}

func (s *Service) loadSettings(ctx context.Context) {
	db := s.getDB()
	if db == nil {
		return
	}

	rows, err := db.QueryContext(ctx, "SELECT key, value FROM autodelete_settings;")
	if err != nil {
		return
	}
	defer rows.Close()

	s.mu.Lock()
	defer s.mu.Unlock()

	for rows.Next() {
		var key, val string
		if err := rows.Scan(&key, &val); err != nil {
			continue
		}
		switch key {
		case "filter_delete_duration":
			if d, err := ParseDuration(val); err == nil {
				s.settings.FilterDeleteDuration = d
			}
		case "file_delete_duration":
			if d, err := ParseDuration(val); err == nil {
				s.settings.FileDeleteDuration = d
			}
		case "fast_download_delete_duration":
			if d, err := ParseDuration(val); err == nil {
				s.settings.FastDownloadDeleteDuration = d
			}
		case "weekly_trash_enabled":
			s.settings.WeeklyTrashEnabled = strings.EqualFold(val, "true") || val == "1"
		case "trash_retention_days":
			if days, err := strconv.Atoi(val); err == nil && days > 0 {
				s.settings.TrashRetentionDays = days
			}
		}
	}
}

func (s *Service) saveSetting(ctx context.Context, key, val string) error {
	db := s.getDB()
	if db == nil {
		return nil
	}

	query := `
		INSERT OR REPLACE INTO autodelete_settings (key, value, updated_at)
		VALUES (?, ?, ?);
	`
	_, err := db.ExecContext(ctx, query, key, val, time.Now())
	return err
}

func (s *Service) getDB() *sql.DB {
	if s.sqlDBProvider != nil {
		return s.sqlDBProvider()
	}
	return nil
}

// ParseDuration parses human-friendly duration strings like "10m", "1h", "30", "0", or "off".
func ParseDuration(input string) (time.Duration, error) {
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" || input == "0" || input == "off" || input == "disable" || input == "disabled" {
		return 0, nil
	}
	if d, err := time.ParseDuration(input); err == nil {
		return d, nil
	}
	// Bare numbers parsed as minutes
	if mins, err := strconv.Atoi(input); err == nil && mins >= 0 {
		return time.Duration(mins) * time.Minute, nil
	}
	return 0, fmt.Errorf("invalid duration %q (use e.g. 10m, 30m, 1h, or 0 to disable)", input)
}

// FormatDuration returns a clean human-readable representation of a duration.
func FormatDuration(d time.Duration) string {
	if d <= 0 {
		return "Disabled"
	}
	if d >= time.Hour && d%time.Hour == 0 {
		hours := int(d / time.Hour)
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	}
	if d >= time.Minute && d%time.Minute == 0 {
		mins := int(d / time.Minute)
		if mins == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", mins)
	}
	return d.String()
}
