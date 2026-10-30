package gfilter

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/AutoFilter-Go/internal/delivery"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Service provides the complete Global Filter business logic:
// - Startup migration and reconciliation from MongoDB to SQLite.
// - Live synchronization upon add/update/delete.
// - In-memory and SQLite cached fast lookup (<0.1ms).
// - Case-insensitive exact whole-message matching.
// - Response delivery across supported Telegram media types.
type Service struct {
	mongoRepo *MongoRepository
	sqlRepo   *SQLRepository
	logger    *slog.Logger
	cache     map[string]*GFilter // keyed by keyword_norm for O(1) matching
	cacheMu   sync.RWMutex
}

// NewService constructs a new Global Filter Service.
func NewService(mongoProv MongoProvider, sqlProv SQLProvider, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		mongoRepo: NewMongoRepository(mongoProv),
		sqlRepo:   NewSQLRepository(sqlProv),
		logger:    logger,
		cache:     make(map[string]*GFilter),
	}
}

// NewServiceWithRepos creates a Service using explicit repository instances (useful for testing).
func NewServiceWithRepos(mongoRepo *MongoRepository, sqlRepo *SQLRepository, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		mongoRepo: mongoRepo,
		sqlRepo:   sqlRepo,
		logger:    logger,
		cache:     make(map[string]*GFilter),
	}
}

// Init performs startup initialization:
// 1. Verifies MongoDB indexes.
// 2. Synchronizes all authoritative MongoDB Global Filters into SQLite.
// 3. Reconciles stale SQLite records.
// 4. Preloads the in-memory cache for instant matching.
func (s *Service) Init(ctx context.Context) error {
	s.logger.Info("initializing Global Filter service...")

	// 1. Ensure MongoDB unique index
	if err := s.mongoRepo.Init(ctx); err != nil {
		s.logger.Warn("could not ensure mongodb global_filters index", "error", err)
	}

	// 2. Startup migration: retrieve authoritative filters from MongoDB
	filters, err := s.mongoRepo.GetAll(ctx)
	if err != nil {
		s.logger.Error("failed fetching global filters from mongodb for startup sync", "error", err)
		// If MongoDB is temporarily unreachable, fallback to SQLite cache
		cached, sqlErr := s.sqlRepo.GetAll(ctx)
		if sqlErr == nil && len(cached) > 0 {
			s.logger.Warn("loaded global filters from local SQLite fallback cache", "count", len(cached))
			s.repopulateCache(cached)
			return nil
		}
		return fmt.Errorf("global filters initialization failed: %w", err)
	}

	// 3. Synchronize authoritative filters into SQLite with reconciliation
	if err := s.sqlRepo.SyncAll(ctx, filters); err != nil {
		s.logger.Error("failed synchronizing global filters into SQLite runtime database", "error", err)
		// Still populate memory cache so bot functions
		s.repopulateCache(filters)
		return fmt.Errorf("global filters sqlite sync failed: %w", err)
	}

	// 4. Preload in-memory lookup cache
	s.repopulateCache(filters)

	s.logger.Info("global filter service initialized successfully",
		"active_filters", len(filters),
	)
	return nil
}

func (s *Service) repopulateCache(filters []*GFilter) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	s.cache = make(map[string]*GFilter, len(filters))
	for _, f := range filters {
		if f == nil || f.Keyword == "" {
			continue
		}
		norm := NormalizeKeyword(f.Keyword)
		f.KeywordNorm = norm
		s.cache[norm] = f
	}
}

// Match performs Case-Insensitive Exact Whole-Message Matching.
//
// Rules:
// - The ENTIRE incoming message text must equal the configured keyword.
// - Only capitalization differences are ignored (using strings.EqualFold).
// - Spaces, punctuation, symbols, and additional words remain strictly significant.
// - NO strings.Contains(), NO substring matching, NO automatic whitespace trimming, NO space collapsing!
func (s *Service) Match(text string) (*GFilter, bool) {
	if text == "" {
		return nil, false
	}

	norm := NormalizeKeyword(text)

	s.cacheMu.RLock()
	f, ok := s.cache[norm]
	s.cacheMu.RUnlock()

	if ok && f != nil {
		// Strict equality check ignoring capitalization only
		if strings.EqualFold(text, f.Keyword) {
			return f, true
		}
	}

	// Fallback to SQLite if cache was unpopulated or missed
	if s.sqlRepo != nil {
		fSQL, err := s.sqlRepo.GetByKeywordNorm(context.Background(), norm)
		if err == nil && fSQL != nil {
			if strings.EqualFold(text, fSQL.Keyword) {
				s.cacheMu.Lock()
				s.cache[norm] = fSQL
				s.cacheMu.Unlock()
				return fSQL, true
			}
		}
	}

	return nil, false
}

// MatchAndExecute checks if the incoming message matches any configured Global Filter.
// If matched:
// - Sends the configured response (text or media with caption and buttons).
// - Returns (true, nil) to signal that this update was handled and Auto Filter must be stopped.
// If not matched:
// - Returns (false, nil) so execution can continue seamlessly into Auto Filter.
func (s *Service) MatchAndExecute(m *telegram.NewMessage) (bool, error) {
	if m == nil || m.IsChannel() || m.IsChannelPost() {
		return false, nil
	}

	text := m.Text()
	if text == "" {
		return false, nil
	}

	filter, matched := s.Match(text)
	if !matched || filter == nil {
		return false, nil
	}

	s.logger.Info("global filter matched incoming message",
		"keyword", filter.Keyword,
		"media_type", filter.MediaType,
		"chat_id", m.ChatID(),
	)

	// Dispatch response
	if err := s.SendResponse(m, filter); err != nil {
		s.logger.Error("failed delivering global filter response",
			"keyword", filter.Keyword,
			"error", err,
		)
		return true, err
	}

	return true, nil
}

// SendResponse delivers the configured Global Filter response to the incoming message.
func (s *Service) SendResponse(m *telegram.NewMessage, f *GFilter) error {
	if m == nil || f == nil {
		return errors.New("cannot send response for nil message or filter")
	}

	replyMarkup := BuildReplyMarkup(f.Buttons)

	switch f.MediaType {
	case MediaTypeText:
		sendOpts := &telegram.SendOptions{
			ParseMode:   "html",
			ReplyMarkup: replyMarkup,
		}
		_, err := m.Reply(f.ReplyText, sendOpts)
		if err != nil {
			// If Telegram rejected button URL or button data, retry without buttons
			if isButtonError(err) {
				sendOpts.ReplyMarkup = nil
				_, err = m.Reply(f.ReplyText, sendOpts)
			}
			// If reply failed (e.g. message was deleted), fallback to responding directly
			if err != nil {
				_, err = m.Respond(f.ReplyText, sendOpts)
			}
		}
		return err

	case MediaTypePhoto, MediaTypeVideo, MediaTypeDocument, MediaTypeAudio, MediaTypeVoice, MediaTypeAnimation, MediaTypeSticker:
		// Resolve media input reference
		var mediaToSend any = f.FileID
		if resolvedMedia, err := delivery.ResolveInputMedia(f.FileID, f.FileRef); err == nil && resolvedMedia != nil {
			mediaToSend = resolvedMedia
		} else if resolved, err := telegram.ResolveBotFileID(f.FileID); err == nil && resolved != nil {
			mediaToSend = resolved
		}

		mediaOpts := &telegram.MediaOptions{
			Caption:       f.ReplyText,
			ParseMode:     "html",
			ReplyMarkup:   replyMarkup,
			ForceDocument: f.MediaType == MediaTypeDocument,
		}

		_, err := m.ReplyMedia(mediaToSend, mediaOpts)
		if err != nil {
			if isButtonError(err) {
				mediaOpts.ReplyMarkup = nil
				_, err = m.ReplyMedia(mediaToSend, mediaOpts)
			}
			if err != nil {
				_, err = m.RespondMedia(mediaToSend, mediaOpts)
			}
		}
		return err

	default:
		// Fallback for unspecified media types: deliver as text
		_, err := m.Reply(f.ReplyText, &telegram.SendOptions{
			ParseMode:   "html",
			ReplyMarkup: replyMarkup,
		})
		return err
	}
}

func isButtonError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "BUTTON_URL_INVALID") ||
		strings.Contains(msg, "BUTTON_DATA_INVALID") ||
		strings.Contains(msg, "Button URL invalid")
}

// SaveFilter adds or updates a Global Filter with live dual-database synchronization.
// 1. Saves to authoritative MongoDB.
// 2. Immediately synchronizes to SQLite.
// 3. Updates in-memory lookup cache.
func (s *Service) SaveFilter(ctx context.Context, f *GFilter) error {
	if f == nil {
		return errors.New("cannot save nil global filter")
	}

	f.Keyword = strings.TrimSpace(f.Keyword)
	if f.Keyword == "" {
		return errors.New("global filter keyword cannot be empty")
	}
	f.KeywordNorm = NormalizeKeyword(f.Keyword)

	if f.ID == "" {
		f.ID = primitive.NewObjectID().Hex()
	}

	// 1. Save to authoritative MongoDB
	if err := s.mongoRepo.Save(ctx, f); err != nil {
		return fmt.Errorf("failed saving global filter to mongodb: %w", err)
	}

	// 2. Live indexing to SQLite
	sqlErr := s.sqlRepo.Save(ctx, f)
	if sqlErr != nil {
		s.logger.Error("failed live indexing global filter to sqlite",
			"keyword", f.Keyword,
			"error", sqlErr,
		)
		// We still update the memory cache so the bot responds immediately
	}

	// 3. Update in-memory cache
	s.cacheMu.Lock()
	s.cache[f.KeywordNorm] = f
	s.cacheMu.Unlock()

	s.logger.Info("global filter saved and indexed",
		"keyword", f.Keyword,
		"media_type", f.MediaType,
		"has_buttons", len(f.Buttons) > 0,
	)

	if sqlErr != nil {
		return fmt.Errorf("saved to mongodb but sqlite live index failed: %w", sqlErr)
	}
	return nil
}

// DeleteFilter removes a Global Filter across MongoDB, SQLite, and the in-memory cache.
func (s *Service) DeleteFilter(ctx context.Context, keyword string) (bool, error) {
	norm := NormalizeKeyword(strings.TrimSpace(keyword))
	if norm == "" {
		return false, errors.New("keyword cannot be empty")
	}

	// 1. Delete from MongoDB
	deletedMongo, err := s.mongoRepo.DeleteByKeywordNorm(ctx, norm)
	if err != nil {
		return false, fmt.Errorf("failed deleting global filter from mongodb: %w", err)
	}

	// 2. Delete from SQLite
	_, sqlErr := s.sqlRepo.DeleteByKeywordNorm(ctx, norm)
	if sqlErr != nil {
		s.logger.Warn("failed deleting global filter from sqlite", "keyword_norm", norm, "error", sqlErr)
	}

	// 3. Invalidate in-memory cache
	s.cacheMu.Lock()
	delete(s.cache, norm)
	s.cacheMu.Unlock()

	return deletedMongo, nil
}

// DeleteAllFilters deletes all Global Filters from MongoDB, SQLite, and the in-memory cache.
func (s *Service) DeleteAllFilters(ctx context.Context) (int64, error) {
	// 1. Delete all from MongoDB
	deletedCount, err := s.mongoRepo.DeleteAll(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed deleting all global filters from mongodb: %w", err)
	}

	// 2. Truncate SQLite table
	if _, sqlErr := s.sqlRepo.DeleteAll(ctx); sqlErr != nil {
		s.logger.Warn("failed truncating global_filters table in sqlite", "error", sqlErr)
	}

	// 3. Clear in-memory cache
	s.cacheMu.Lock()
	s.cache = make(map[string]*GFilter)
	s.cacheMu.Unlock()

	s.logger.Info("all global filters purged", "deleted_count", deletedCount)
	return deletedCount, nil
}

// GetFilter retrieves details of a specific Global Filter by its keyword.
func (s *Service) GetFilter(ctx context.Context, keyword string) (*GFilter, error) {
	norm := NormalizeKeyword(strings.TrimSpace(keyword))
	if norm == "" {
		return nil, errors.New("keyword cannot be empty")
	}

	s.cacheMu.RLock()
	f, ok := s.cache[norm]
	s.cacheMu.RUnlock()

	if ok && f != nil {
		return f, nil
	}

	// Check SQLite
	if s.sqlRepo != nil {
		fSQL, err := s.sqlRepo.GetByKeywordNorm(ctx, norm)
		if err == nil && fSQL != nil {
			return fSQL, nil
		}
	}

	// Check MongoDB
	return s.mongoRepo.GetByKeywordNorm(ctx, norm)
}

// ListFilters returns a paginated list of all active Global Filters.
func (s *Service) ListFilters(ctx context.Context, limit, offset int) ([]*GFilter, int64, error) {
	s.cacheMu.RLock()
	var all []*GFilter
	for _, f := range s.cache {
		all = append(all, f)
	}
	s.cacheMu.RUnlock()

	// If cache was empty, load from SQLite or MongoDB
	if len(all) == 0 {
		var err error
		if s.sqlRepo != nil {
			all, err = s.sqlRepo.GetAll(ctx)
		}
		if (err != nil || len(all) == 0) && s.mongoRepo != nil {
			all, err = s.mongoRepo.GetAll(ctx)
		}
		if err != nil {
			return nil, 0, err
		}
	}

	total := int64(len(all))
	if offset < 0 {
		offset = 0
	}
	if offset >= len(all) {
		return []*GFilter{}, total, nil
	}

	end := offset + limit
	if end > len(all) {
		end = len(all)
	}

	return all[offset:end], total, nil
}

// Count returns the total number of registered Global Filters.
func (s *Service) Count(ctx context.Context) (int64, error) {
	s.cacheMu.RLock()
	cachedCount := int64(len(s.cache))
	s.cacheMu.RUnlock()

	if cachedCount > 0 {
		return cachedCount, nil
	}

	if s.sqlRepo != nil {
		if cnt, err := s.sqlRepo.Count(ctx); err == nil && cnt > 0 {
			return cnt, nil
		}
	}

	return s.mongoRepo.Count(ctx)
}
