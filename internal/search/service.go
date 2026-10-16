package search

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/itsafxal/MYTGBOTZ/internal/database"
)

// SearchService defines the contract for movie search and suggestion queries.
type SearchService interface {
	Search(ctx context.Context, query string, limit, offset int, filter SearchFilter) (*SearchResult, error)
	GetSuggestions(ctx context.Context, query string, limit int) ([]string, error)
}

// Service implements the SearchService against the SQLite runtime cache.
type Service struct {
	dbManager   *database.DatabaseManager
	logger      *slog.Logger
	resultCache *lruCache
	sugCache    *lruCache
}

// NewService creates a new SearchService instance.
func NewService(dbManager *database.DatabaseManager, logger *slog.Logger) *Service {
	return &Service{
		dbManager:   dbManager,
		logger:      logger,
		resultCache: newLRUCache(2000, 5*time.Minute),
		sugCache:    newLRUCache(1000, 10*time.Minute),
	}
}

// Search executes a multi-token query against the SQLite media database.
func (s *Service) Search(ctx context.Context, query string, limit, offset int, filter SearchFilter) (*SearchResult, error) {
	norm := NormalizeText(query)
	if norm == "" {
		return &SearchResult{Query: query}, nil
	}

	if limit <= 0 {
		limit = 8
	}
	if offset < 0 {
		offset = 0
	}

	cacheKey := fmt.Sprintf("%s:%d:%d:%s:%s", norm, limit, offset, filter.Language, filter.Quality)
	if val, ok := s.resultCache.get(cacheKey); ok {
		if res, ok := val.(*SearchResult); ok {
			return res, nil
		}
	}

	tokens := ExtractMeaningfulTokens(norm)
	if len(tokens) == 0 {
		tokens = strings.Fields(norm)
	}

	db := s.dbManager.SQLManager().DB()
	if db == nil {
		return nil, fmt.Errorf("sqlite database is not initialized")
	}

	// 1. Build dynamic WHERE clause
	var whereConditions []string
	var args []any

	for _, token := range tokens {
		if len(token) == 1 && unicode.IsDigit(rune(token[0])) {
			whereConditions = append(whereConditions, "(file_name LIKE ? OR file_name LIKE ? OR file_name LIKE ? OR caption LIKE ?)")
			args = append(args,
				"%."+token+".%",
				"% "+token+" %",
				"%-"+token+"-%",
				"% "+token+" %",
			)
		} else {
			whereConditions = append(whereConditions, "(file_name LIKE ? OR caption LIKE ?)")
			pattern := "%" + token + "%"
			args = append(args, pattern, pattern)
		}
	}

	if filter.Language != "" {
		whereConditions = append(whereConditions, "(file_name LIKE ? OR caption LIKE ?)")
		langPattern := "%" + strings.ToLower(filter.Language) + "%"
		args = append(args, langPattern, langPattern)
	}

	if filter.Quality != "" {
		whereConditions = append(whereConditions, "(file_name LIKE ? OR caption LIKE ?)")
		qualPattern := "%" + strings.ToLower(filter.Quality) + "%"
		args = append(args, qualPattern, qualPattern)
	}

	whereClause := strings.Join(whereConditions, " AND ")

	// 2. Count total matches
	countSQL := "SELECT COUNT(*) FROM media WHERE " + whereClause + ";"
	var total int64
	if err := db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		s.logger.Error("failed to count search results", "query", norm, "error", err)
		return nil, fmt.Errorf("search count error: %w", err)
	}

	if total == 0 {
		res := &SearchResult{
			Query:      query,
			CleanQuery: norm,
			Files:      []MediaFile{},
			Total:      0,
			Offset:     offset,
			Page:       1,
			TotalPages: 0,
		}
		s.resultCache.set(cacheKey, res)
		return res, nil
	}

	// 3. Fetch paginated records
	selectSQL := fmt.Sprintf(`
		SELECT file_id, file_ref, file_name, file_size, size, file_type, mime_type, caption, date_saved
		FROM media
		WHERE %s
		ORDER BY date_saved DESC
		LIMIT ? OFFSET ?;
	`, whereClause)

	queryArgs := append(args, limit, offset)
	rows, err := db.QueryContext(ctx, selectSQL, queryArgs...)
	if err != nil {
		s.logger.Error("failed to execute search query", "query", norm, "error", err)
		return nil, fmt.Errorf("search select error: %w", err)
	}
	defer rows.Close()

	type scoredFile struct {
		file  MediaFile
		score float64
	}

	var scoredFiles []scoredFile

	for rows.Next() {
		var f MediaFile
		var dateSaved sql.NullTime

		if err := rows.Scan(
			&f.FileID,
			&f.FileRef,
			&f.FileName,
			&f.FileSize,
			&f.Size,
			&f.FileType,
			&f.MimeType,
			&f.Caption,
			&dateSaved,
		); err != nil {
			s.logger.Error("failed to scan media row", "error", err)
			continue
		}

		if dateSaved.Valid {
			f.DateSaved = dateSaved.Time
		}

		score := ComputeSimilarity(norm, f.FileName)
		scoredFiles = append(scoredFiles, scoredFile{file: f, score: score})
	}

	// In-memory ranking: Sort closer title matches to the top
	sort.SliceStable(scoredFiles, func(i, j int) bool {
		return scoredFiles[i].score > scoredFiles[j].score
	})

	files := make([]MediaFile, len(scoredFiles))
	for i, sf := range scoredFiles {
		files[i] = sf.file
	}

	// Calculate pagination state
	page := (offset / limit) + 1
	totalPages := int((total + int64(limit) - 1) / int64(limit))
	nextOffset := 0
	if int64(offset+limit) < total {
		nextOffset = offset + limit
	}
	prevOffset := 0
	if offset > 0 {
		prevOffset = offset - limit
		if prevOffset < 0 {
			prevOffset = 0
		}
	}

	result := &SearchResult{
		Query:      query,
		CleanQuery: norm,
		Files:      files,
		Total:      total,
		Offset:     offset,
		NextOffset: nextOffset,
		PrevOffset: prevOffset,
		Page:       page,
		TotalPages: totalPages,
		HasNext:    nextOffset > 0,
		HasPrev:    offset > 0,
	}

	s.resultCache.set(cacheKey, result)
	return result, nil
}

// GetSuggestions generates spelling and movie title suggestions when exact search finds 0 results.
func (s *Service) GetSuggestions(ctx context.Context, query string, limit int) ([]string, error) {
	norm := NormalizeText(query)
	if norm == "" || len(norm) < 2 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 5
	}

	if val, ok := s.sugCache.get(norm); ok {
		if sugs, ok := val.([]string); ok {
			return sugs, nil
		}
	}

	db := s.dbManager.SQLManager().DB()
	if db == nil {
		return nil, fmt.Errorf("sqlite database is not initialized")
	}

	tokens := ExtractMeaningfulTokens(norm)
	if len(tokens) == 0 {
		tokens = strings.Fields(norm)
	}
	if len(tokens) == 0 {
		return nil, nil
	}

	// Strategy: Retrieve candidate titles matching at least one token or prefix
	var candidateQuery string
	var candidateArgs []any

	var orClauses []string
	for _, tok := range tokens {
		if len(tok) >= 3 {
			orClauses = append(orClauses, "file_name LIKE ?")
			candidateArgs = append(candidateArgs, "%"+tok+"%")
		}
		if len(tok) >= 4 {
			orClauses = append(orClauses, "file_name LIKE ?")
			candidateArgs = append(candidateArgs, "%"+tok[:3]+"%")
		}
	}

	// If tokens are short (e.g. 2 chars), fallback to prefix
	if len(orClauses) == 0 && len(tokens) > 0 {
		tok := tokens[0]
		orClauses = append(orClauses, "file_name LIKE ?")
		candidateArgs = append(candidateArgs, tok+"%")
	}

	if len(orClauses) == 0 {
		return nil, nil
	}

	candidateQuery = fmt.Sprintf(`
		SELECT DISTINCT file_name
		FROM media
		WHERE %s
		ORDER BY date_saved DESC
		LIMIT 60;
	`, strings.Join(orClauses, " OR "))

	rows, err := db.QueryContext(ctx, candidateQuery, candidateArgs...)
	if err != nil {
		s.logger.Error("failed to query candidate suggestions", "query", norm, "error", err)
		return nil, err
	}
	defer rows.Close()

	type scoredSuggestion struct {
		title string
		score float64
	}

	seen := make(map[string]struct{})
	var candidates []scoredSuggestion

	for rows.Next() {
		var rawFileName string
		if err := rows.Scan(&rawFileName); err != nil {
			continue
		}

		cleanTitle := CleanReleaseName(rawFileName)
		if cleanTitle == "" || len(cleanTitle) < 2 {
			cleanTitle = strings.TrimSpace(rawFileName)
		}

		key := strings.ToLower(cleanTitle)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		score := ComputeSimilarity(norm, cleanTitle)
		if score >= 0.50 {
			candidates = append(candidates, scoredSuggestion{title: cleanTitle, score: score})
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	suggestions := make([]string, 0, limit)
	for _, cand := range candidates {
		suggestions = append(suggestions, cand.title)
		if len(suggestions) >= limit {
			break
		}
	}

	s.sugCache.set(norm, suggestions)
	return suggestions, nil
}

// --- Thread-Safe In-Memory LRU Cache ---

type cacheItem struct {
	value     any
	expiresAt time.Time
}

type lruCache struct {
	mu      sync.RWMutex
	items   map[string]cacheItem
	maxSize int
	ttl     time.Duration
}

func newLRUCache(maxSize int, ttl time.Duration) *lruCache {
	return &lruCache{
		items:   make(map[string]cacheItem),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

func (c *lruCache) get(key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, exists := c.items[key]
	if !exists {
		return nil, false
	}
	if time.Now().After(item.expiresAt) {
		return nil, false
	}
	return item.value, true
}

func (c *lruCache) set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.items) >= c.maxSize {
		// Evict an arbitrary item (simple pruning)
		for k := range c.items {
			delete(c.items, k)
			break
		}
	}

	c.items[key] = cacheItem{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
}
