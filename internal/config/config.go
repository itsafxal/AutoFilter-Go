package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// LogLevel represents the supported logging levels.
type LogLevel string

const (
	LogLevelDebug LogLevel = "DEBUG"
	LogLevelInfo  LogLevel = "INFO"
	LogLevelWarn  LogLevel = "WARN"
	LogLevelError LogLevel = "ERROR"
)

// Config holds the validated bot configuration loaded from environment variables.
type Config struct {
	// BotToken is the Telegram Bot API authentication token.
	BotToken string

	// OwnerID is the Telegram user ID of the bot owner.
	OwnerID int64

	// Admins contains Telegram user IDs permitted to perform administrative operations.
	// Always includes OwnerID.
	Admins []int64

	// LogLevel defines the application logging verbosity (DEBUG, INFO, WARN, ERROR).
	LogLevel LogLevel

	// DatabaseURI is the primary MongoDB connection string.
	DatabaseURI string

	// DatabaseName is the MongoDB database name (default: "CyberTG").
	DatabaseName string

	// CollectionName is the authoritative MongoDB media collection name (default: "Telegram-Document").
	CollectionName string

	// DatabaseURI2 is an optional secondary MongoDB cluster URI for sharded media.
	DatabaseURI2 string

	// DatabaseURI3 is an optional tertiary MongoDB cluster URI for sharded media.
	DatabaseURI3 string

	// SQLiteDBPath is the file path for the temporary local SQLite runtime database (default: "media.sqlite3").
	SQLiteDBPath string

	// SyncBatchSize defines how many records are fetched and inserted per batch during SQLite sync (default: 10000).
	SyncBatchSize int
}

// LoadFromEnv loads and validates configuration from the current environment variables.
func LoadFromEnv() (*Config, error) {
	token := strings.TrimSpace(os.Getenv("BOT_TOKEN"))
	if token == "" {
		return nil, errors.New("configuration error: BOT_TOKEN is required and cannot be empty")
	}

	ownerIDStr := strings.TrimSpace(os.Getenv("OWNER_ID"))
	if ownerIDStr == "" {
		return nil, errors.New("configuration error: OWNER_ID is required and cannot be empty")
	}
	ownerID, err := strconv.ParseInt(ownerIDStr, 10, 64)
	if err != nil || ownerID <= 0 {
		return nil, fmt.Errorf("configuration error: OWNER_ID must be a positive integer, got %q", ownerIDStr)
	}

	adminsRaw := strings.TrimSpace(os.Getenv("ADMINS"))
	admins, err := parseAdminIDs(adminsRaw, ownerID)
	if err != nil {
		return nil, fmt.Errorf("configuration error: invalid ADMINS value: %w", err)
	}

	logLevelStr := strings.ToUpper(strings.TrimSpace(os.Getenv("LOG_LEVEL")))
	if logLevelStr == "" {
		logLevelStr = string(LogLevelInfo)
	}
	logLevel := LogLevel(logLevelStr)
	switch logLevel {
	case LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError:
		// Valid
	default:
		return nil, fmt.Errorf("configuration error: invalid LOG_LEVEL %q (must be DEBUG, INFO, WARN, or ERROR)", logLevelStr)
	}

	dbURI := strings.TrimSpace(os.Getenv("DATABASE_URI"))
	dbName := strings.TrimSpace(os.Getenv("DATABASE_NAME"))
	if dbName == "" {
		dbName = "CyberTG"
	}
	colName := strings.TrimSpace(os.Getenv("COLLECTION_NAME"))
	if colName == "" {
		colName = "Telegram-Document"
	}

	dbURI2 := strings.TrimSpace(os.Getenv("DATABASE_URI2"))
	if dbURI2 == "" {
		dbURI2 = dbURI
	}
	dbURI3 := strings.TrimSpace(os.Getenv("DATABASE_URI3"))
	if dbURI3 == "" {
		dbURI3 = dbURI
	}

	sqlitePath := strings.TrimSpace(os.Getenv("SQLITE_DB_PATH"))
	if sqlitePath == "" {
		sqlitePath = "media.sqlite3"
	}

	batchSize := 10000
	if rawBatch := strings.TrimSpace(os.Getenv("SYNC_BATCH_SIZE")); rawBatch != "" {
		if parsed, err := strconv.Atoi(rawBatch); err == nil && parsed > 0 {
			batchSize = parsed
		}
	}

	cfg := &Config{
		BotToken:       token,
		OwnerID:        ownerID,
		Admins:         admins,
		LogLevel:       logLevel,
		DatabaseURI:    dbURI,
		DatabaseName:   dbName,
		CollectionName: colName,
		DatabaseURI2:   dbURI2,
		DatabaseURI3:   dbURI3,
		SQLiteDBPath:   sqlitePath,
		SyncBatchSize:  batchSize,
	}

	return cfg, nil
}

// parseAdminIDs parses space- or comma-separated user IDs, ensuring ownerID is included and deduplicated.
func parseAdminIDs(raw string, ownerID int64) ([]int64, error) {
	adminSet := make(map[int64]struct{})
	adminSet[ownerID] = struct{}{}

	if raw != "" {
		fields := strings.FieldsFunc(raw, func(r rune) bool {
			return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == ';'
		})

		for _, field := range fields {
			field = strings.TrimSpace(field)
			if field == "" {
				continue
			}
			id, err := strconv.ParseInt(field, 10, 64)
			if err != nil || id <= 0 {
				return nil, fmt.Errorf("invalid Telegram ID %q", field)
			}
			adminSet[id] = struct{}{}
		}
	}

	admins := make([]int64, 0, len(adminSet))
	admins = append(admins, ownerID)
	for id := range adminSet {
		if id != ownerID {
			admins = append(admins, id)
		}
	}

	return admins, nil
}

// IsAdmin checks if the given user ID is in the admin list or is the owner.
func (c *Config) IsAdmin(userID int64) bool {
	if userID <= 0 {
		return false
	}
	if userID == c.OwnerID {
		return true
	}
	for _, adminID := range c.Admins {
		if adminID == userID {
			return true
		}
	}
	return false
}

// IsOwner checks if the given user ID is the owner.
func (c *Config) IsOwner(userID int64) bool {
	return userID > 0 && userID == c.OwnerID
}

// HasDatabase checks if a primary MongoDB database URI is configured.
func (c *Config) HasDatabase() bool {
	return c.DatabaseURI != ""
}

// MaskedToken returns a safely redacted representation of the bot token.
func (c *Config) MaskedToken() string {
	parts := strings.Split(c.BotToken, ":")
	if len(parts) >= 2 {
		return parts[0] + ":***REDACTED***"
	}
	if len(c.BotToken) > 6 {
		return c.BotToken[:4] + "***REDACTED***"
	}
	return "***REDACTED***"
}

var uriUserInfoRegex = regexp.MustCompile(`://([^:]+):([^@]+)@`)

// MaskURI redacts credentials from database URIs to prevent secret leaks in logs.
func MaskURI(rawURI string) string {
	if rawURI == "" {
		return ""
	}
	if uriUserInfoRegex.MatchString(rawURI) {
		return uriUserInfoRegex.ReplaceAllString(rawURI, `://$1:***REDACTED***@`)
	}
	// Also check if URI has no user but has password or token
	u, err := url.Parse(rawURI)
	if err == nil && u.User != nil {
		user := u.User.Username()
		if user != "" {
			return strings.Replace(rawURI, u.User.String(), user+":***REDACTED***", 1)
		}
		return strings.Replace(rawURI, u.User.String(), "***REDACTED***", 1)
	}
	return rawURI
}

// String provides a safe string representation that NEVER exposes secrets.
func (c *Config) String() string {
	return fmt.Sprintf("Config{BotToken:%s, OwnerID:%d, Admins:%v, LogLevel:%s, DB:%s, Col:%s, URI:%s, SQLite:%s}",
		c.MaskedToken(), c.OwnerID, c.Admins, c.LogLevel, c.DatabaseName, c.CollectionName, MaskURI(c.DatabaseURI), c.SQLiteDBPath)
}
