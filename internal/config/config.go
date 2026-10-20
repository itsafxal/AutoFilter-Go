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

	// AppID is the Telegram MTProto API ID from my.telegram.org.
	AppID int32

	// AppHash is the Telegram MTProto API Hash from my.telegram.org.
	AppHash string

	// SessionPath is the file path for persistent MTProto session storage.
	SessionPath string

	// MemorySession specifies whether to keep the session in memory only without writing to disk.
	MemorySession bool

	// OwnerID is the Telegram user ID of the bot owner.
	OwnerID int64

	// Admins contains Telegram user IDs permitted to perform administrative operations.
	// Always includes OwnerID.
	Admins []int64

	// Channels contains Telegram channel IDs monitored for live media indexing.
	Channels []int64

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

	// BackgroundSync specifies whether media synchronization runs asynchronously in the background, allowing the bot to boot immediately (default: true).
	BackgroundSync bool

	// EnableSync specifies whether MongoDB media documents are synchronized to local SQLite (default: true).
	// If set to false (via ENABLE_SYNC=false or DISABLE_SYNC=true), startup synchronization is completely bypassed
	// and the bot boots in ~1 second, querying authoritative MongoDB directly.
	EnableSync bool

	// CustomCaption defines the default media caption template when none is overridden by an admin.
	CustomCaption string

	// FSubChannelDescription is the default channel description set automatically when a channel is added to FSub.
	FSubChannelDescription string

	// AutoDeleteFilterTime defines the default duration before search filter messages are deleted (default: "15m").
	AutoDeleteFilterTime string

	// AutoDeleteFileTime defines the default duration before delivered file messages are deleted (default: "30m").
	AutoDeleteFileTime string

	// AutoDeleteFastDownloadTime defines the duration before fast download messages in chat are auto-deleted (default: "5m").
	AutoDeleteFastDownloadTime string

	// FastDownloadLinkExpiry defines the duration for which fast download links remain valid on the server (default: "30m").
	FastDownloadLinkExpiry string

	// FastDownloadButtonStyle defines the color/style of the generated fast download button ("green" or "blue", default: "green").
	FastDownloadButtonStyle string

	// AutoDeleteWeeklyTrash specifies whether weekly trash cleanup is enabled (default: true).
	AutoDeleteWeeklyTrash bool

	// TrashRetentionDays specifies the message retention in days before weekly trash purges them (default: 7).
	TrashRetentionDays int

	// StreamServerEnabled specifies whether the HTTP streaming & download server is enabled (default: true).
	StreamServerEnabled bool

	// StreamURL defines the public base URL for fast download and online streaming links (default: "http://localhost:8080").
	StreamURL string

	// StreamPort defines the TCP port for the stream server (default: 8080 or env PORT).
	StreamPort int

	// StreamBindAddr defines the interface IP to bind the stream server (default: "0.0.0.0").
	StreamBindAddr string

	// EnableFastDownloadButton specifies whether the "⚡ Get Fast Download" button is added to delivered media (default: true).
	EnableFastDownloadButton bool
}


// LoadFromEnv loads and validates configuration from the current environment variables.
func LoadFromEnv() (*Config, error) {
	token := strings.TrimSpace(os.Getenv("BOT_TOKEN"))
	if token == "" {
		return nil, errors.New("configuration error: BOT_TOKEN is required and cannot be empty")
	}

	apiIDStr := strings.TrimSpace(os.Getenv("API_ID"))
	if apiIDStr == "" {
		apiIDStr = strings.TrimSpace(os.Getenv("APP_ID"))
	}
	if apiIDStr == "" {
		return nil, errors.New("configuration error: API_ID is required and cannot be empty")
	}
	apiID64, err := strconv.ParseInt(apiIDStr, 10, 32)
	if err != nil || apiID64 <= 0 {
		return nil, fmt.Errorf("configuration error: API_ID must be a positive integer, got %q", apiIDStr)
	}
	apiID := int32(apiID64)

	apiHash := strings.TrimSpace(os.Getenv("API_HASH"))
	if apiHash == "" {
		apiHash = strings.TrimSpace(os.Getenv("APP_HASH"))
	}
	if apiHash == "" {
		return nil, errors.New("configuration error: API_HASH is required and cannot be empty")
	}

	sessionPath := strings.TrimSpace(os.Getenv("SESSION_PATH"))
	if sessionPath == "" {
		sessionPath = "bot.session"
	}

	memorySession := strings.EqualFold(strings.TrimSpace(os.Getenv("MEMORY_SESSION")), "true")

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

	customCaption := strings.TrimSpace(os.Getenv("CUSTOM_CAPTION"))
	if customCaption == "" {
		customCaption = "{filename}\n\nFile Size : {file size}"
	} else {
		customCaption = strings.ReplaceAll(customCaption, "\\n", "\n")
	}

	fsubDesc := strings.TrimSpace(os.Getenv("FSUB_CHANNEL_DESCRIPTION"))
	if fsubDesc == "" {
		fsubDesc = "You need to request to join channel to get file 🚀"
	}

	filterDelTime := strings.TrimSpace(os.Getenv("AUTO_DELETE_FILTER_TIME"))
	if filterDelTime == "" {
		filterDelTime = "15m"
	}

	fileDelTime := strings.TrimSpace(os.Getenv("AUTO_DELETE_FILE_TIME"))
	if fileDelTime == "" {
		fileDelTime = "30m"
	}

	fastDLTime := strings.TrimSpace(os.Getenv("AUTO_DELETE_FAST_DOWNLOAD_TIME"))
	if fastDLTime == "" {
		fastDLTime = "5m"
	}

	fastDLExpiry := strings.TrimSpace(os.Getenv("FAST_DOWNLOAD_LINK_EXPIRY"))
	if fastDLExpiry == "" {
		fastDLExpiry = "30m"
	}

	fastDLBtnStyle := strings.ToLower(strings.TrimSpace(os.Getenv("FAST_DOWNLOAD_BUTTON_STYLE")))
	if fastDLBtnStyle == "" {
		fastDLBtnStyle = "green"
	}

	weeklyTrash := true
	if rawTrash := strings.TrimSpace(os.Getenv("AUTO_DELETE_WEEKLY_TRASH")); rawTrash != "" {
		weeklyTrash = strings.EqualFold(rawTrash, "true") || rawTrash == "1"
	}

	trashRetentionDays := 7
	if rawDays := strings.TrimSpace(os.Getenv("TRASH_RETENTION_DAYS")); rawDays != "" {
		if parsed, err := strconv.Atoi(rawDays); err == nil && parsed > 0 {
			trashRetentionDays = parsed
		}
	}

	streamServerEnabled := true
	if rawStream := strings.TrimSpace(os.Getenv("STREAM_SERVER_ENABLED")); rawStream != "" {
		streamServerEnabled = strings.EqualFold(rawStream, "true") || rawStream == "1"
	}

	streamPort := 8080
	portStr := strings.TrimSpace(os.Getenv("STREAM_PORT"))
	if portStr == "" {
		portStr = strings.TrimSpace(os.Getenv("PORT"))
	}
	if portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil && p > 0 && p <= 65535 {
			streamPort = p
		}
	}

	streamBindAddr := strings.TrimSpace(os.Getenv("STREAM_BIND_ADDR"))
	if streamBindAddr == "" {
		streamBindAddr = "0.0.0.0"
	}

	streamURL := strings.TrimSpace(os.Getenv("STREAM_URL"))
	if streamURL == "" {
		streamURL = strings.TrimSpace(os.Getenv("WEB_URL"))
	}
	streamURL = strings.TrimRight(streamURL, "/")

	enableFastDownloadBtn := true
	if rawBtn := strings.TrimSpace(os.Getenv("ENABLE_FAST_DOWNLOAD_BUTTON")); rawBtn != "" {
		enableFastDownloadBtn = strings.EqualFold(rawBtn, "true") || rawBtn == "1"
	}

	backgroundSync := true
	if bgStr := strings.TrimSpace(os.Getenv("BACKGROUND_SYNC")); bgStr != "" {
		if val, err := strconv.ParseBool(bgStr); err == nil {
			backgroundSync = val
		}
	}

	enableSync := true
	if esStr := strings.TrimSpace(os.Getenv("ENABLE_SYNC")); esStr != "" {
		if val, err := strconv.ParseBool(esStr); err == nil {
			enableSync = val
		}
	} else if dsStr := strings.TrimSpace(os.Getenv("DISABLE_SYNC")); dsStr != "" {
		if val, err := strconv.ParseBool(dsStr); err == nil {
			enableSync = !val
		}
	}

	cfg := &Config{
		BotToken:                 token,
		AppID:                    apiID,
		AppHash:                  apiHash,
		SessionPath:              sessionPath,
		MemorySession:            memorySession,
		OwnerID:                  ownerID,
		Admins:                   admins,
		LogLevel:                 logLevel,
		DatabaseURI:              dbURI,
		DatabaseName:             dbName,
		CollectionName:           colName,
		DatabaseURI2:             dbURI2,
		DatabaseURI3:             dbURI3,
		SQLiteDBPath:             sqlitePath,
		SyncBatchSize:            batchSize,
		BackgroundSync:           backgroundSync,
		EnableSync:               enableSync,
		CustomCaption:            customCaption,
		FSubChannelDescription:   fsubDesc,
		AutoDeleteFilterTime:       filterDelTime,
		AutoDeleteFileTime:         fileDelTime,
		AutoDeleteFastDownloadTime: fastDLTime,
		FastDownloadLinkExpiry:     fastDLExpiry,
		FastDownloadButtonStyle:    fastDLBtnStyle,
		AutoDeleteWeeklyTrash:    weeklyTrash,
		TrashRetentionDays:       trashRetentionDays,
		StreamServerEnabled:      streamServerEnabled,
		StreamURL:                streamURL,
		StreamPort:               streamPort,
		StreamBindAddr:           streamBindAddr,
		EnableFastDownloadButton: enableFastDownloadBtn,
		Channels:                 parseChannelIDs(os.Getenv("CHANNELS")),
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

// NormalizeChannelID converts any channel ID format (e.g. 1234567890, -1001234567890)
// into standard Telegram supergroup/channel ID format (-1001234567890).
func NormalizeChannelID(rawID int64) int64 {
	if rawID == 0 {
		return 0
	}
	s := strconv.FormatInt(rawID, 10)
	if strings.HasPrefix(s, "-100") {
		return rawID
	}
	if strings.HasPrefix(s, "100") {
		if id, err := strconv.ParseInt("-" + s, 10, 64); err == nil {
			return id
		}
	}
	s = strings.TrimPrefix(s, "-")
	if id, err := strconv.ParseInt("-100" + s, 10, 64); err == nil {
		return id
	}
	return rawID
}

// parseChannelIDs parses comma-, space-, or newline-separated channel IDs into normalized -100... IDs.
func parseChannelIDs(raw string) []int64 {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == ';'
	})

	var channels []int64
	seen := make(map[int64]struct{})
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		id, err := strconv.ParseInt(f, 10, 64)
		if err != nil {
			continue
		}
		norm := NormalizeChannelID(id)
		if _, exists := seen[norm]; !exists {
			seen[norm] = struct{}{}
			channels = append(channels, norm)
		}
	}
	return channels
}

// IsConfiguredChannel checks if the given chat ID matches any monitored channel.
func (c *Config) IsConfiguredChannel(chatID int64) bool {
	if c == nil || len(c.Channels) == 0 || chatID == 0 {
		return false
	}
	normTarget := NormalizeChannelID(chatID)
	rawTargetStr := strings.TrimPrefix(strings.TrimPrefix(strconv.FormatInt(chatID, 10), "-"), "100")

	for _, ch := range c.Channels {
		if ch == chatID || ch == normTarget || NormalizeChannelID(ch) == normTarget {
			return true
		}
		chStr := strings.TrimPrefix(strings.TrimPrefix(strconv.FormatInt(ch, 10), "-"), "100")
		if chStr != "" && chStr == rawTargetStr {
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

// MaskedAppHash returns a safely redacted representation of the Telegram API hash.
func (c *Config) MaskedAppHash() string {
	if len(c.AppHash) > 6 {
		return c.AppHash[:4] + "***REDACTED***"
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
	return fmt.Sprintf("Config{BotToken:%s, AppID:%d, AppHash:%s, SessionPath:%s, MemorySession:%t, OwnerID:%d, Admins:%v, LogLevel:%s, DB:%s, Col:%s, URI:%s, SQLite:%s, BgSync:%t, Caption:%s, FSubDesc:%s, FilterDel:%s, FileDel:%s, WeeklyTrash:%t, TrashRetention:%dd, StreamServer:%t, StreamURL:%s, StreamPort:%d, FastDownloadBtn:%t}",
		c.MaskedToken(), c.AppID, c.MaskedAppHash(), c.SessionPath, c.MemorySession, c.OwnerID, c.Admins, c.LogLevel, c.DatabaseName, c.CollectionName, MaskURI(c.DatabaseURI), c.SQLiteDBPath, c.BackgroundSync, c.CustomCaption, c.FSubChannelDescription, c.AutoDeleteFilterTime, c.AutoDeleteFileTime, c.AutoDeleteWeeklyTrash, c.TrashRetentionDays, c.StreamServerEnabled, c.StreamURL, c.StreamPort, c.EnableFastDownloadButton)
}

