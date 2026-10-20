package caption

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// DefaultCaptionModel represents the baseline model for media delivery captions.
const DefaultCaptionModel = "{filename}\n\nFile Size : {file size}"

// CaptionService defines the operations for dynamic caption templating and persistence.
type CaptionService interface {
	// GetCaption returns the active caption template (admin custom override if set, otherwise default).
	GetCaption(ctx context.Context) string

	// GetCustomCaption returns the admin-configured custom caption template and whether one is actively set.
	GetCustomCaption(ctx context.Context) (string, bool)

	// SetCaption saves a new admin custom caption template across memory and persistent databases.
	SetCaption(ctx context.Context, template string) error

	// DeleteCaption resets custom caption configuration back to default.
	DeleteCaption(ctx context.Context) error

	// IsCustom reports whether an admin override is actively in use.
	IsCustom() bool

	// FormatMedia formats media attributes using the currently active caption template.
	FormatMedia(ctx context.Context, fileName, sizeStr, originalCaption string) string

	// Format renders any arbitrary template string using the media attributes.
	Format(template, fileName, sizeStr, originalCaption string) string

	// Init loads persisted custom caption settings from the database into memory.
	Init(ctx context.Context) error
}

// Service implements CaptionService with persistent SQLite and MongoDB storage.
type Service struct {
	defaultTemplate string
	mongoProvider   func() *mongo.Database
	sqlDBProvider   func() *sql.DB
	logger          *slog.Logger
	customCaption   string
	hasCustom       bool
	mu              sync.RWMutex
}

// NewService constructs a new CaptionService instance.
func NewService(defaultTemplate string, mongoProv func() *mongo.Database, sqlProv func() *sql.DB, logger *slog.Logger) *Service {
	if strings.TrimSpace(defaultTemplate) == "" {
		defaultTemplate = DefaultCaptionModel
	}
	return &Service{
		defaultTemplate: defaultTemplate,
		mongoProvider:   mongoProv,
		sqlDBProvider:   sqlProv,
		logger:          logger,
	}
}

// Format renders a caption template with given media attributes.
// Supported placeholders:
// - {filename}, {file_name} -> fileName
// - {file sise}, {file size}, {filesize}, {file_size}, {size} -> sizeStr
// - {caption} -> originalCaption
func Format(tmpl, fileName, sizeStr, originalCaption string) string {
	if strings.TrimSpace(tmpl) == "" {
		if strings.TrimSpace(originalCaption) != "" {
			return originalCaption
		}
		return fileName
	}

	result := tmpl

	// 1. Filename substitutions
	result = strings.ReplaceAll(result, "{filename}", fileName)
	result = strings.ReplaceAll(result, "{file_name}", fileName)

	// 2. File size substitutions (supports user prompt '{file sise}' and standard variants)
	result = strings.ReplaceAll(result, "{file sise}", sizeStr)
	result = strings.ReplaceAll(result, "{file size}", sizeStr)
	result = strings.ReplaceAll(result, "{filesize}", sizeStr)
	result = strings.ReplaceAll(result, "{file_size}", sizeStr)
	result = strings.ReplaceAll(result, "{size}", sizeStr)

	// 3. Caption substitution
	effectiveCaption := strings.TrimSpace(originalCaption)
	if effectiveCaption == "" {
		effectiveCaption = fileName
	}
	result = strings.ReplaceAll(result, "{caption}", effectiveCaption)

	return result
}

// Format renders the provided template with media attributes.
func (s *Service) Format(template, fileName, sizeStr, originalCaption string) string {
	return Format(template, fileName, sizeStr, originalCaption)
}

// Init loads any previously saved custom caption template from the database.
func (s *Service) Init(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Try reading from SQLite sync_metadata table first
	if s.sqlDBProvider != nil {
		if db := s.sqlDBProvider(); db != nil {
			var val string
			err := db.QueryRowContext(ctx, "SELECT value FROM sync_metadata WHERE key = 'custom_caption' LIMIT 1;").Scan(&val)
			if err == nil && strings.TrimSpace(val) != "" {
				s.customCaption = val
				s.hasCustom = true
				if s.logger != nil {
					s.logger.Info("loaded custom caption from SQLite runtime cache", "template", val)
				}
				return nil
			}
		}
	}

	// 2. If not found in SQLite, try reading from primary MongoDB
	if s.mongoProvider != nil {
		if db := s.mongoProvider(); db != nil {
			col := db.Collection("admins")
			var doc struct {
				CustomCaption string `bson:"custom_caption"`
			}
			err := col.FindOne(ctx, bson.M{"custom_caption": bson.M{"$exists": true, "$ne": ""}}).Decode(&doc)
			if err == nil && strings.TrimSpace(doc.CustomCaption) != "" {
				s.customCaption = doc.CustomCaption
				s.hasCustom = true

				// Synchronize into SQLite for instant local queries
				if s.sqlDBProvider != nil {
					if sqldb := s.sqlDBProvider(); sqldb != nil {
						_, _ = sqldb.ExecContext(ctx, "INSERT OR REPLACE INTO sync_metadata (key, value, updated_at) VALUES ('custom_caption', ?, CURRENT_TIMESTAMP);", doc.CustomCaption)
					}
				}

				if s.logger != nil {
					s.logger.Info("loaded custom caption from authoritative MongoDB", "template", doc.CustomCaption)
				}
				return nil
			}
		}
	}

	return nil
}

// GetCaption returns the active caption template.
func (s *Service) GetCaption(ctx context.Context) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.hasCustom && strings.TrimSpace(s.customCaption) != "" {
		return s.customCaption
	}
	return s.defaultTemplate
}

// GetCustomCaption returns the custom caption and whether it is configured.
func (s *Service) GetCustomCaption(ctx context.Context) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.hasCustom && strings.TrimSpace(s.customCaption) != "" {
		return s.customCaption, true
	}
	return "", false
}

// IsCustom returns true if a custom caption override is set.
func (s *Service) IsCustom() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hasCustom && strings.TrimSpace(s.customCaption) != ""
}

// SetCaption updates the custom caption across memory, SQLite, and MongoDB.
func (s *Service) SetCaption(ctx context.Context, template string) error {
	trimmed := strings.TrimSpace(template)
	if trimmed == "" {
		return errors.New("caption template cannot be empty")
	}

	s.mu.Lock()
	s.customCaption = trimmed
	s.hasCustom = true
	s.mu.Unlock()

	var errs []error

	// 1. Persist to SQLite sync_metadata
	if s.sqlDBProvider != nil {
		if db := s.sqlDBProvider(); db != nil {
			_, err := db.ExecContext(ctx, "INSERT OR REPLACE INTO sync_metadata (key, value, updated_at) VALUES ('custom_caption', ?, CURRENT_TIMESTAMP);", trimmed)
			if err != nil {
				errs = append(errs, fmt.Errorf("failed saving caption to SQLite: %w", err))
			}
		}
	}

	// 2. Persist to MongoDB admins collection
	if s.mongoProvider != nil {
		if db := s.mongoProvider(); db != nil {
			col := db.Collection("admins")
			opts := options.Update().SetUpsert(true)
			_, err := col.UpdateMany(ctx, bson.M{}, bson.M{"$set": bson.M{"custom_caption": trimmed}}, opts)
			if err != nil {
				errs = append(errs, fmt.Errorf("failed saving caption to MongoDB: %w", err))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("caption persistence error: %v", errs)
	}
	return nil
}

// DeleteCaption clears the custom caption across memory, SQLite, and MongoDB.
func (s *Service) DeleteCaption(ctx context.Context) error {
	s.mu.Lock()
	s.customCaption = ""
	s.hasCustom = false
	s.mu.Unlock()

	var errs []error

	// 1. Remove from SQLite
	if s.sqlDBProvider != nil {
		if db := s.sqlDBProvider(); db != nil {
			_, err := db.ExecContext(ctx, "DELETE FROM sync_metadata WHERE key = 'custom_caption';")
			if err != nil {
				errs = append(errs, fmt.Errorf("failed deleting caption from SQLite: %w", err))
			}
		}
	}

	// 2. Remove from MongoDB
	if s.mongoProvider != nil {
		if db := s.mongoProvider(); db != nil {
			col := db.Collection("admins")
			_, err := col.UpdateMany(ctx, bson.M{}, bson.M{"$unset": bson.M{"custom_caption": ""}})
			if err != nil {
				errs = append(errs, fmt.Errorf("failed unsetting caption in MongoDB: %w", err))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("caption deletion error: %v", errs)
	}
	return nil
}

// FormatMedia formats media attributes using the currently active caption template.
func (s *Service) FormatMedia(ctx context.Context, fileName, sizeStr, originalCaption string) string {
	tmpl := s.GetCaption(ctx)
	return Format(tmpl, fileName, sizeStr, originalCaption)
}
