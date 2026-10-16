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

// Service implements the SearchService against the high-speed SQLite runtime cache.
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
		resultCache: newLRUCache(5000, 10*time.Minute),
		sugCache:    newLRUCache(2000, 15*time.Minute),
	}
}

// Search executes a sub-millisecond query against the SQLite FTS5 trigram virtual table with LIKE fallback.
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

	cacheKey := fmt.Sprintf("%s:%d:%d:%s:%s", norm, limit, offset, strings.ToLower(filter.Language), strings.ToLower(filter.Quality))
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

	// 1. Primary High-Speed Search: FTS5 Trigram MATCH (Boolean AND)
	var records []MediaFile
	var hasNext bool
	var ftsErr error

	ftsExpr := s.buildFTS5Expression(tokens, filter)
	if ftsExpr != "" {
		ftsSQL := `
			SELECT m.file_id, m.file_ref, m.file_name, m.file_size, m.size, m.file_type, m.mime_type, m.caption, m.date_saved
			FROM media_fts f
			JOIN media m ON f.rowid = m.rowid
			WHERE media_fts MATCH ?
			ORDER BY m.date_saved DESC
			LIMIT ? OFFSET ?;
		`
		records, hasNext, ftsErr = s.queryRows(ctx, db, ftsSQL, []any{ftsExpr, limit + 1, offset}, limit)
	}

	// 2. Secondary Search Fallback: LIKE Query (only if FTS produced 0 records or had error)
	if len(records) == 0 && (ftsExpr == "" || ftsErr != nil || len(tokens) > 0) {
		likeSQL, likeArgs := s.buildLikeQuery(tokens, filter, limit+1, offset)
		likeRecords, likeHasNext, likeErr := s.queryRows(ctx, db, likeSQL, likeArgs, limit)
		if likeErr == nil && len(likeRecords) > 0 {
			records = likeRecords
			hasNext = likeHasNext
		}
	}

	// If no records found
	if len(records) == 0 {
		emptyRes := &SearchResult{
			Query:      query,
			CleanQuery: norm,
			Files:      []MediaFile{},
			Total:      0,
			Offset:     offset,
			Page:       1,
			TotalPages: 0,
			HasNext:    false,
			HasPrev:    offset > 0,
		}
		s.resultCache.set(cacheKey, emptyRes)
		return emptyRes, nil
	}

	// Rank files by title similarity
	type scoredFile struct {
		file  MediaFile
		score float64
	}
	scored := make([]scoredFile, len(records))
	for i, f := range records {
		score := ComputeSimilarity(norm, f.FileName)
		scored[i] = scoredFile{file: f, score: score}
	}
	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	files := make([]MediaFile, len(scored))
	for i, sf := range scored {
		files[i] = sf.file
	}

	page := (offset / limit) + 1
	var total int64
	var totalPages int
	if hasNext {
		total = int64(offset + limit + 1)
		totalPages = page + 1
	} else {
		total = int64(offset + len(files))
		totalPages = page
	}

	nextOffset := 0
	if hasNext {
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
		HasNext:    hasNext,
		HasPrev:    offset > 0,
	}

	s.resultCache.set(cacheKey, result)
	return result, nil
}

func (s *Service) queryRows(ctx context.Context, db *sql.DB, query string, args []any, limit int) ([]MediaFile, bool, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	var records []MediaFile
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
			continue
		}
		if dateSaved.Valid {
			f.DateSaved = dateSaved.Time
		}
		records = append(records, f)
	}

	if err := rows.Err(); err != nil {
		return nil, false, err
	}

	hasNext := false
	if len(records) > limit {
		hasNext = true
		records = records[:limit]
	}

	return records, hasNext, nil
}

func (s *Service) buildFTS5Expression(tokens []string, filter SearchFilter) string {
	var ftsTerms []string
	for _, tok := range tokens {
		clean := cleanFTS5Token(tok)
		if len(clean) >= 2 || (len(clean) == 1 && unicode.IsDigit(rune(clean[0]))) {
			ftsTerms = append(ftsTerms, fmt.Sprintf(`"%s"`, clean))
		}
	}

	if filter.Language != "" {
		cleanLang := cleanFTS5Token(filter.Language)
		if cleanLang != "" {
			ftsTerms = append(ftsTerms, fmt.Sprintf(`"%s"`, cleanLang))
		}
	}

	if filter.Quality != "" {
		cleanQual := cleanFTS5Token(filter.Quality)
		if cleanQual != "" {
			ftsTerms = append(ftsTerms, fmt.Sprintf(`"%s"`, cleanQual))
		}
	}

	if len(ftsTerms) == 0 {
		return ""
	}

	return strings.Join(ftsTerms, " AND ")
}

func cleanFTS5Token(token string) string {
	var sb strings.Builder
	for _, r := range token {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			sb.WriteRune(unicode.ToLower(r))
		}
	}
	return sb.String()
}

func (s *Service) buildLikeQuery(tokens []string, filter SearchFilter, limitPlusOne, offset int) (string, []any) {
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

	whereClause := "1=1"
	if len(whereConditions) > 0 {
		whereClause = strings.Join(whereConditions, " AND ")
	}

	query := fmt.Sprintf(`
		SELECT file_id, file_ref, file_name, file_size, size, file_type, mime_type, caption, date_saved
		FROM media
		WHERE %s
		ORDER BY date_saved DESC
		LIMIT ? OFFSET ?;
	`, whereClause)

	args = append(args, limitPlusOne, offset)
	return query, args
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

	var rawFileNames []string

	// 1. Try FTS5 OR search for fast candidate discovery
	var ftsOrTerms []string
	for _, tok := range tokens {
		clean := cleanFTS5Token(tok)
		if len(clean) >= 2 {
			ftsOrTerms = append(ftsOrTerms, fmt.Sprintf(`"%s"`, clean))
		}
	}

	if len(ftsOrTerms) > 0 {
		ftsOrExpr := strings.Join(ftsOrTerms, " OR ")
		ftsQuery := `
			SELECT m.file_name
			FROM media_fts f
			JOIN media m ON f.rowid = m.rowid
			WHERE media_fts MATCH ?
			LIMIT 60;
		`
		rows, err := db.QueryContext(ctx, ftsQuery, ftsOrExpr)
		if err == nil {
			for rows.Next() {
				var name string
				if err := rows.Scan(&name); err == nil {
					rawFileNames = append(rawFileNames, name)
				}
			}
			_ = rows.Close()
		}
	}

	// 2. Fallback to LIKE if FTS produced 0 candidates
	if len(rawFileNames) == 0 {
		var orClauses []string
		var candidateArgs []any
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

		if len(orClauses) == 0 && len(tokens) > 0 {
			tok := tokens[0]
			orClauses = append(orClauses, "file_name LIKE ?")
			candidateArgs = append(candidateArgs, tok+"%")
		}

		if len(orClauses) > 0 {
			likeQuery := fmt.Sprintf(`
				SELECT DISTINCT file_name
				FROM media
				WHERE %s
				ORDER BY date_saved DESC
				LIMIT 60;
			`, strings.Join(orClauses, " OR "))
			rows, err := db.QueryContext(ctx, likeQuery, candidateArgs...)
			if err == nil {
				for rows.Next() {
					var name string
					if err := rows.Scan(&name); err == nil {
						rawFileNames = append(rawFileNames, name)
					}
				}
				_ = rows.Close()
			}
		}
	}

	type scoredSuggestion struct {
		title string
		score float64
	}

	seen := make(map[string]struct{})
	var candidates []scoredSuggestion

	for _, rawFileName := range rawFileNames {
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
