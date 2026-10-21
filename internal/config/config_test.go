package config

import (
	"os"
	"strings"
	"testing"
)

func TestLoadFromEnv_Success(t *testing.T) {
	os.Setenv("BOT_TOKEN", "123456789:ABCdefGhIJKlmNoPQRsTUVwxyZ")
	os.Setenv("API_ID", "123456")
	os.Setenv("API_HASH", "abcdef0123456789abcdef0123456789")
	os.Setenv("SESSION_PATH", "custom_bot.session")
	os.Setenv("MEMORY_SESSION", "true")
	os.Setenv("OWNER_ID", "99887766")
	os.Setenv("ADMINS", "11223344, 55667788")
	os.Setenv("LOG_LEVEL", "DEBUG")
	os.Setenv("DATABASE_URI", "mongodb+srv://admin:secret123@cluster0.abc.mongodb.net/?retryWrites=true")
	os.Setenv("DATABASE_NAME", "CyberTG")
	os.Setenv("COLLECTION_NAME", "Telegram-Document")
	os.Setenv("SQLITE_DB_PATH", "custom_media.sqlite3")
	os.Setenv("SYNC_BATCH_SIZE", "5000")
	defer func() {
		os.Unsetenv("BOT_TOKEN")
		os.Unsetenv("API_ID")
		os.Unsetenv("API_HASH")
		os.Unsetenv("SESSION_PATH")
		os.Unsetenv("MEMORY_SESSION")
		os.Unsetenv("OWNER_ID")
		os.Unsetenv("ADMINS")
		os.Unsetenv("LOG_LEVEL")
		os.Unsetenv("DATABASE_URI")
		os.Unsetenv("DATABASE_NAME")
		os.Unsetenv("COLLECTION_NAME")
		os.Unsetenv("SQLITE_DB_PATH")
		os.Unsetenv("SYNC_BATCH_SIZE")
	}()

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if cfg.BotToken != "123456789:ABCdefGhIJKlmNoPQRsTUVwxyZ" {
		t.Errorf("expected BotToken to match, got %s", cfg.BotToken)
	}
	if cfg.AppID != 123456 {
		t.Errorf("expected AppID to be 123456, got %d", cfg.AppID)
	}
	if cfg.AppHash != "abcdef0123456789abcdef0123456789" {
		t.Errorf("expected AppHash to match, got %s", cfg.AppHash)
	}
	if cfg.SessionPath != "custom_bot.session" {
		t.Errorf("expected SessionPath to be custom_bot.session, got %s", cfg.SessionPath)
	}
	if !cfg.MemorySession {
		t.Error("expected MemorySession to be true")
	}
	if cfg.OwnerID != 99887766 {
		t.Errorf("expected OwnerID to be 99887766, got %d", cfg.OwnerID)
	}
	if cfg.LogLevel != LogLevelDebug {
		t.Errorf("expected LogLevel to be DEBUG, got %s", cfg.LogLevel)
	}
	if cfg.DatabaseName != "CyberTG" {
		t.Errorf("expected DatabaseName CyberTG, got %s", cfg.DatabaseName)
	}
	if cfg.CollectionName != "Telegram-Document" {
		t.Errorf("expected CollectionName Telegram-Document, got %s", cfg.CollectionName)
	}
	if cfg.SQLiteDBPath != "custom_media.sqlite3" {
		t.Errorf("expected SQLiteDBPath custom_media.sqlite3, got %s", cfg.SQLiteDBPath)
	}
	if cfg.SyncBatchSize != 5000 {
		t.Errorf("expected SyncBatchSize 5000, got %d", cfg.SyncBatchSize)
	}
	if !cfg.HasDatabase() {
		t.Error("expected HasDatabase to return true")
	}
	if !cfg.IsAdmin(99887766) {
		t.Error("expected owner to be admin")
	}
	if !cfg.IsAdmin(11223344) {
		t.Error("expected 11223344 to be admin")
	}
	if !cfg.IsAdmin(55667788) {
		t.Error("expected 55667788 to be admin")
	}
	if cfg.IsAdmin(99999999) {
		t.Error("expected 99999999 NOT to be admin")
	}
	if !cfg.IsOwner(99887766) {
		t.Error("expected 99887766 to be owner")
	}
	if cfg.IsOwner(11223344) {
		t.Error("expected 11223344 NOT to be owner")
	}
}

func TestLoadFromEnv_DefaultLogLevelAndDB(t *testing.T) {
	os.Setenv("BOT_TOKEN", "123456789:ABCdefGhIJKlmNoPQRsTUVwxyZ")
	os.Setenv("API_ID", "654321")
	os.Setenv("API_HASH", "fedcba9876543210fedcba9876543210")
	os.Setenv("OWNER_ID", "99887766")
	os.Unsetenv("SESSION_PATH")
	os.Unsetenv("MEMORY_SESSION")
	os.Unsetenv("ADMINS")
	os.Unsetenv("LOG_LEVEL")
	os.Unsetenv("DATABASE_URI")
	os.Unsetenv("DATABASE_NAME")
	os.Unsetenv("COLLECTION_NAME")
	os.Unsetenv("SQLITE_DB_PATH")
	defer func() {
		os.Unsetenv("BOT_TOKEN")
		os.Unsetenv("API_ID")
		os.Unsetenv("API_HASH")
		os.Unsetenv("OWNER_ID")
	}()

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.LogLevel != LogLevelInfo {
		t.Errorf("expected default LogLevel to be INFO, got %s", cfg.LogLevel)
	}
	if cfg.SessionPath != "bot.session" {
		t.Errorf("expected default SessionPath to be bot.session, got %s", cfg.SessionPath)
	}
	if cfg.MemorySession {
		t.Error("expected default MemorySession to be false")
	}
	if len(cfg.Admins) != 1 || cfg.Admins[0] != 99887766 {
		t.Errorf("expected Admins to contain only OwnerID, got %v", cfg.Admins)
	}
	if cfg.DatabaseName != "CyberTG" {
		t.Errorf("expected default DatabaseName CyberTG, got %s", cfg.DatabaseName)
	}
	if cfg.CollectionName != "Telegram-Document" {
		t.Errorf("expected default CollectionName Telegram-Document, got %s", cfg.CollectionName)
	}
	if cfg.SQLiteDBPath != "media.sqlite3" {
		t.Errorf("expected default SQLiteDBPath media.sqlite3, got %s", cfg.SQLiteDBPath)
	}
	if cfg.SyncBatchSize != 10000 {
		t.Errorf("expected default SyncBatchSize 10000, got %d", cfg.SyncBatchSize)
	}
	if !cfg.BackgroundSync {
		t.Error("expected default BackgroundSync to be true")
	}
	if cfg.AutoDeleteFilterTime != "15m" {
		t.Errorf("expected default AutoDeleteFilterTime 15m, got %s", cfg.AutoDeleteFilterTime)
	}
	if cfg.AutoDeleteFileTime != "30m" {
		t.Errorf("expected default AutoDeleteFileTime 30m, got %s", cfg.AutoDeleteFileTime)
	}
	if !cfg.AutoDeleteWeeklyTrash {
		t.Error("expected default AutoDeleteWeeklyTrash to be true")
	}
	if cfg.TrashRetentionDays != 7 {
		t.Errorf("expected default TrashRetentionDays 7, got %d", cfg.TrashRetentionDays)
	}
	if cfg.HasDatabase() {
		t.Error("expected HasDatabase to be false when DATABASE_URI is unset")
	}
}

func TestLoadFromEnv_MissingBotToken(t *testing.T) {
	os.Unsetenv("BOT_TOKEN")
	os.Setenv("API_ID", "123")
	os.Setenv("API_HASH", "hash123")
	os.Setenv("OWNER_ID", "12345")
	defer func() {
		os.Unsetenv("API_ID")
		os.Unsetenv("API_HASH")
		os.Unsetenv("OWNER_ID")
	}()

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error for missing BOT_TOKEN, got nil")
	}
	if !strings.Contains(err.Error(), "BOT_TOKEN") {
		t.Errorf("expected error message to mention BOT_TOKEN, got: %v", err)
	}
}

func TestLoadFromEnv_MissingAPIID(t *testing.T) {
	os.Setenv("BOT_TOKEN", "123:abc")
	os.Unsetenv("API_ID")
	os.Unsetenv("APP_ID")
	os.Setenv("API_HASH", "hash123")
	os.Setenv("OWNER_ID", "12345")
	defer func() {
		os.Unsetenv("BOT_TOKEN")
		os.Unsetenv("API_HASH")
		os.Unsetenv("OWNER_ID")
	}()

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error for missing API_ID, got nil")
	}
	if !strings.Contains(err.Error(), "API_ID") {
		t.Errorf("expected error message to mention API_ID, got: %v", err)
	}
}

func TestLoadFromEnv_InvalidAPIID(t *testing.T) {
	os.Setenv("BOT_TOKEN", "123:abc")
	os.Setenv("API_ID", "-10")
	os.Setenv("API_HASH", "hash123")
	os.Setenv("OWNER_ID", "12345")
	defer func() {
		os.Unsetenv("BOT_TOKEN")
		os.Unsetenv("API_ID")
		os.Unsetenv("API_HASH")
		os.Unsetenv("OWNER_ID")
	}()

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error for negative API_ID, got nil")
	}
	if !strings.Contains(err.Error(), "API_ID") {
		t.Errorf("expected error message to mention API_ID, got: %v", err)
	}
}

func TestLoadFromEnv_MissingAPIHash(t *testing.T) {
	os.Setenv("BOT_TOKEN", "123:abc")
	os.Setenv("API_ID", "12345")
	os.Unsetenv("API_HASH")
	os.Unsetenv("APP_HASH")
	os.Setenv("OWNER_ID", "12345")
	defer func() {
		os.Unsetenv("BOT_TOKEN")
		os.Unsetenv("API_ID")
		os.Unsetenv("OWNER_ID")
	}()

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error for missing API_HASH, got nil")
	}
	if !strings.Contains(err.Error(), "API_HASH") {
		t.Errorf("expected error message to mention API_HASH, got: %v", err)
	}
}

func TestLoadFromEnv_MissingOwnerID(t *testing.T) {
	os.Setenv("BOT_TOKEN", "123:abc")
	os.Setenv("API_ID", "12345")
	os.Setenv("API_HASH", "hash123")
	os.Unsetenv("OWNER_ID")
	defer func() {
		os.Unsetenv("BOT_TOKEN")
		os.Unsetenv("API_ID")
		os.Unsetenv("API_HASH")
	}()

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error for missing OWNER_ID, got nil")
	}
	if !strings.Contains(err.Error(), "OWNER_ID") {
		t.Errorf("expected error message to mention OWNER_ID, got: %v", err)
	}
}

func TestLoadFromEnv_InvalidOwnerID(t *testing.T) {
	os.Setenv("BOT_TOKEN", "123:abc")
	os.Setenv("API_ID", "12345")
	os.Setenv("API_HASH", "hash123")
	os.Setenv("OWNER_ID", "not_a_number")
	defer func() {
		os.Unsetenv("BOT_TOKEN")
		os.Unsetenv("API_ID")
		os.Unsetenv("API_HASH")
		os.Unsetenv("OWNER_ID")
	}()

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error for non-numeric OWNER_ID, got nil")
	}
}

func TestLoadFromEnv_InvalidAdminID(t *testing.T) {
	os.Setenv("BOT_TOKEN", "123:abc")
	os.Setenv("API_ID", "12345")
	os.Setenv("API_HASH", "hash123")
	os.Setenv("OWNER_ID", "12345")
	os.Setenv("ADMINS", "123,invalid_admin,456")
	defer func() {
		os.Unsetenv("BOT_TOKEN")
		os.Unsetenv("API_ID")
		os.Unsetenv("API_HASH")
		os.Unsetenv("OWNER_ID")
		os.Unsetenv("ADMINS")
	}()

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error for non-numeric admin ID, got nil")
	}
}

func TestLoadFromEnv_InvalidLogLevel(t *testing.T) {
	os.Setenv("BOT_TOKEN", "123:abc")
	os.Setenv("API_ID", "12345")
	os.Setenv("API_HASH", "hash123")
	os.Setenv("OWNER_ID", "12345")
	os.Setenv("LOG_LEVEL", "TRACE_VERBOSE")
	defer func() {
		os.Unsetenv("BOT_TOKEN")
		os.Unsetenv("API_ID")
		os.Unsetenv("API_HASH")
		os.Unsetenv("OWNER_ID")
		os.Unsetenv("LOG_LEVEL")
	}()

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error for invalid LOG_LEVEL, got nil")
	}
}

func TestSecretRedaction(t *testing.T) {
	secret := "987654321:AAFE_secret_bot_token_value_xyz"
	apiHash := "super_secret_api_hash_1234567890abcdef"
	dbURI := "mongodb+srv://botadmin:SuperSecretPass123!@cluster0.abc.mongodb.net/CyberTG?retryWrites=true"

	cfg := &Config{
		BotToken:       secret,
		AppID:          123456,
		AppHash:        apiHash,
		SessionPath:    "bot.session",
		OwnerID:        12345,
		Admins:         []int64{12345},
		LogLevel:       LogLevelInfo,
		DatabaseURI:    dbURI,
		DatabaseName:   "CyberTG",
		CollectionName: "Telegram-Document",
		SQLiteDBPath:   "media.sqlite3",
	}

	str := cfg.String()
	if strings.Contains(str, "AAFE_secret_bot_token_value_xyz") {
		t.Errorf("CRITICAL SECURITY FLAW: secret token leaked in String(): %s", str)
	}
	if strings.Contains(str, "super_secret_api_hash_1234567890abcdef") {
		t.Errorf("CRITICAL SECURITY FLAW: api hash leaked in String(): %s", str)
	}
	if strings.Contains(str, "SuperSecretPass123!") {
		t.Errorf("CRITICAL SECURITY FLAW: database password leaked in String(): %s", str)
	}
	if !strings.Contains(str, "REDACTED") {
		t.Errorf("expected String() to contain REDACTED, got: %s", str)
	}

	masked := cfg.MaskedToken()
	if strings.Contains(masked, "AAFE_secret_bot_token_value_xyz") {
		t.Errorf("CRITICAL SECURITY FLAW: secret token leaked in MaskedToken(): %s", masked)
	}

	maskedHash := cfg.MaskedAppHash()
	if strings.Contains(maskedHash, "1234567890abcdef") {
		t.Errorf("CRITICAL SECURITY FLAW: api hash leaked in MaskedAppHash(): %s", maskedHash)
	}

	maskedURI := MaskURI(dbURI)
	if strings.Contains(maskedURI, "SuperSecretPass123!") {
		t.Errorf("CRITICAL SECURITY FLAW: database password leaked in MaskURI(): %s", maskedURI)
	}
	if !strings.Contains(maskedURI, "***REDACTED***") {
		t.Errorf("expected MaskURI to contain ***REDACTED***, got: %s", maskedURI)
	}
}

func TestLoadFromEnv_StreamConfig(t *testing.T) {
	os.Setenv("BOT_TOKEN", "123456789:ABCdefGhIJKlmNoPQRsTUVwxyZ")
	os.Setenv("API_ID", "123456")
	os.Setenv("API_HASH", "abcdef0123456789abcdef0123456789")
	os.Setenv("OWNER_ID", "99887766")

	// 1. Defaults
	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.StreamServerEnabled {
		t.Error("expected StreamServerEnabled default true")
	}
	if cfg.StreamPort != 8080 {
		t.Errorf("expected default StreamPort 8080, got %d", cfg.StreamPort)
	}
	if cfg.StreamBindAddr != "0.0.0.0" {
		t.Errorf("expected default StreamBindAddr 0.0.0.0, got %s", cfg.StreamBindAddr)
	}
	if cfg.StreamURL != "" {
		t.Errorf("expected default StreamURL empty, got %s", cfg.StreamURL)
	}
	if !cfg.EnableFastDownloadButton {
		t.Error("expected default EnableFastDownloadButton true")
	}

	// 2. Custom values
	os.Setenv("STREAM_SERVER_ENABLED", "false")
	os.Setenv("STREAM_PORT", "9090")
	os.Setenv("STREAM_BIND_ADDR", "127.0.0.1")
	os.Setenv("STREAM_URL", "https://stream.mybot.com/")
	os.Setenv("ENABLE_FAST_DOWNLOAD_BUTTON", "false")

	defer func() {
		os.Unsetenv("BOT_TOKEN")
		os.Unsetenv("API_ID")
		os.Unsetenv("API_HASH")
		os.Unsetenv("OWNER_ID")
		os.Unsetenv("STREAM_SERVER_ENABLED")
		os.Unsetenv("STREAM_PORT")
		os.Unsetenv("STREAM_BIND_ADDR")
		os.Unsetenv("STREAM_URL")
		os.Unsetenv("ENABLE_FAST_DOWNLOAD_BUTTON")
	}()

	customCfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if customCfg.StreamServerEnabled {
		t.Error("expected StreamServerEnabled false")
	}
	if customCfg.StreamPort != 9090 {
		t.Errorf("expected StreamPort 9090, got %d", customCfg.StreamPort)
	}
	if customCfg.StreamBindAddr != "127.0.0.1" {
		t.Errorf("expected StreamBindAddr 127.0.0.1, got %s", customCfg.StreamBindAddr)
	}
	if customCfg.StreamURL != "https://stream.mybot.com" {
		t.Errorf("expected trimmed StreamURL https://stream.mybot.com, got %s", customCfg.StreamURL)
	}
	if customCfg.EnableFastDownloadButton {
		t.Error("expected EnableFastDownloadButton false")
	}
}

func TestConfig_Channels(t *testing.T) {
	os.Setenv("BOT_TOKEN", "123456789:ABCdefGhIJKlmNoPQRsTUVwxyZ")
	os.Setenv("API_ID", "123456")
	os.Setenv("API_HASH", "abcdef0123456789abcdef0123456789")
	os.Setenv("OWNER_ID", "99887766")
	os.Setenv("CHANNELS", "-1001234567890, 1009876543210, -1005555555555")
	defer func() {
		os.Unsetenv("BOT_TOKEN")
		os.Unsetenv("API_ID")
		os.Unsetenv("API_HASH")
		os.Unsetenv("OWNER_ID")
		os.Unsetenv("CHANNELS")
	}()

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.Channels) != 3 {
		t.Fatalf("expected 3 channels, got %d", len(cfg.Channels))
	}

	// Channel -1001234567890 should match directly
	if !cfg.IsConfiguredChannel(-1001234567890) {
		t.Errorf("expected -1001234567890 to be configured")
	}

	// Positive channel ID 1234567890 should normalize and match -1001234567890
	if !cfg.IsConfiguredChannel(1234567890) {
		t.Errorf("expected positive ID 1234567890 to match configured channel")
	}

	// Negative ID without -100 prefix should also normalize and match
	if !cfg.IsConfiguredChannel(-1234567890) {
		t.Errorf("expected -1234567890 to normalize and match configured channel")
	}

	// Unrelated channel
	if cfg.IsConfiguredChannel(-1009999999999) {
		t.Errorf("expected unrelated channel not to match")
	}
}

func TestLoadFromEnv_PremiumEmoji(t *testing.T) {
	os.Setenv("BOT_TOKEN", "123456789:ABCdefGhIJKlmNoPQRsTUVwxyZ")
	os.Setenv("API_ID", "123456")
	os.Setenv("API_HASH", "abcdef0123456789abcdef0123456789")
	os.Setenv("OWNER_ID", "99887766")
	defer func() {
		os.Unsetenv("BOT_TOKEN")
		os.Unsetenv("API_ID")
		os.Unsetenv("API_HASH")
		os.Unsetenv("OWNER_ID")
		os.Unsetenv("ENABLE_PREMIUM_EMOJI")
		os.Unsetenv("PREV_BUTTON_EMOJI_ID")
		os.Unsetenv("NEXT_BUTTON_EMOJI_ID")
		os.Unsetenv("PAGE_BUTTON_EMOJI_ID")
		os.Unsetenv("QUALITY_BUTTON_EMOJI_ID")
		os.Unsetenv("LANGUAGE_BUTTON_EMOJI_ID")
		os.Unsetenv("FAST_DOWNLOAD_BUTTON_EMOJI_ID")
		os.Unsetenv("GREETING_EMOJI_ID")
		os.Unsetenv("HEY_EMOJI_ID")
		os.Unsetenv("QUERY_EMOJI_ID")
		os.Unsetenv("PAGE_TEXT_EMOJI_ID")
		os.Unsetenv("NOTE_PREFIX_EMOJI_ID")
		os.Unsetenv("NOTE_SUFFIX_EMOJI_ID")
		os.Unsetenv("FILE_SIZE_PREFIX_EMOJI_ID")
		os.Unsetenv("FILE_SIZE_SUFFIX_EMOJI_ID")
	}()

	// Defaults
	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.EnablePremiumEmoji {
		t.Errorf("expected EnablePremiumEmoji to default to true")
	}
	if cfg.PrevButtonEmojiID != 5447183459602669338 {
		t.Errorf("expected default PrevButtonEmojiID 5447183459602669338, got %d", cfg.PrevButtonEmojiID)
	}
	if cfg.NextButtonEmojiID != 5449683594425410231 {
		t.Errorf("expected default NextButtonEmojiID 5449683594425410231, got %d", cfg.NextButtonEmojiID)
	}
	if cfg.PageButtonEmojiID != 5837048052061311256 {
		t.Errorf("expected default PageButtonEmojiID 5837048052061311256, got %d", cfg.PageButtonEmojiID)
	}
	if cfg.QualityButtonEmojiID != 5276251908774858442 {
		t.Errorf("expected default QualityButtonEmojiID 5276251908774858442, got %d", cfg.QualityButtonEmojiID)
	}
	if cfg.LanguageButtonEmojiID != 5276011261757265100 {
		t.Errorf("expected default LanguageButtonEmojiID 5276011261757265100, got %d", cfg.LanguageButtonEmojiID)
	}
	if cfg.FastDownloadButtonEmojiID != 5195033767969839232 {
		t.Errorf("expected default FastDownloadButtonEmojiID 5195033767969839232, got %d", cfg.FastDownloadButtonEmojiID)
	}
	if cfg.GreetingEmojiID != 5427232022252764138 {
		t.Errorf("expected default GreetingEmojiID 5427232022252764138, got %d", cfg.GreetingEmojiID)
	}
	if cfg.QueryEmojiID != 5190859184312167965 {
		t.Errorf("expected default QueryEmojiID 5190859184312167965, got %d", cfg.QueryEmojiID)
	}
	if cfg.PageTextEmojiID != 5836954069586946866 {
		t.Errorf("expected default PageTextEmojiID 5836954069586946866, got %d", cfg.PageTextEmojiID)
	}
	if cfg.NotePrefixEmojiID != 4956461073550017373 {
		t.Errorf("expected default NotePrefixEmojiID 4956461073550017373, got %d", cfg.NotePrefixEmojiID)
	}
	if cfg.NoteSuffixEmojiID != 4958724224962265918 {
		t.Errorf("expected default NoteSuffixEmojiID 4958724224962265918, got %d", cfg.NoteSuffixEmojiID)
	}
	if cfg.FileSizePrefixEmojiID != 5992513524440502236 {
		t.Errorf("expected default FileSizePrefixEmojiID 5992513524440502236, got %d", cfg.FileSizePrefixEmojiID)
	}
	if cfg.FileSizeSuffixEmojiID != 5992519984071315160 {
		t.Errorf("expected default FileSizeSuffixEmojiID 5992519984071315160, got %d", cfg.FileSizeSuffixEmojiID)
	}

	// Custom values
	os.Setenv("ENABLE_PREMIUM_EMOJI", "false")
	os.Setenv("PREV_BUTTON_EMOJI_ID", "12345")
	os.Setenv("NEXT_BUTTON_EMOJI_ID", "67890")
	os.Setenv("PAGE_BUTTON_EMOJI_ID", "111")
	os.Setenv("QUALITY_BUTTON_EMOJI_ID", "222")
	os.Setenv("LANGUAGE_BUTTON_EMOJI_ID", "333")
	os.Setenv("FAST_DOWNLOAD_BUTTON_EMOJI_ID", "444")
	os.Setenv("GREETING_EMOJI_ID", "555")
	os.Setenv("QUERY_EMOJI_ID", "666")
	os.Setenv("PAGE_TEXT_EMOJI_ID", "777")
	os.Setenv("NOTE_PREFIX_EMOJI_ID", "888")
	os.Setenv("NOTE_SUFFIX_EMOJI_ID", "999")
	os.Setenv("FILE_SIZE_PREFIX_EMOJI_ID", "1010")
	os.Setenv("FILE_SIZE_SUFFIX_EMOJI_ID", "2020")

	cfg2, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg2.EnablePremiumEmoji {
		t.Errorf("expected EnablePremiumEmoji to be false")
	}
	if cfg2.PrevButtonEmojiID != 12345 {
		t.Errorf("expected PrevButtonEmojiID 12345, got %d", cfg2.PrevButtonEmojiID)
	}
	if cfg2.NextButtonEmojiID != 67890 {
		t.Errorf("expected NextButtonEmojiID 67890, got %d", cfg2.NextButtonEmojiID)
	}
	if cfg2.PageButtonEmojiID != 111 {
		t.Errorf("expected PageButtonEmojiID 111, got %d", cfg2.PageButtonEmojiID)
	}
	if cfg2.QualityButtonEmojiID != 222 {
		t.Errorf("expected QualityButtonEmojiID 222, got %d", cfg2.QualityButtonEmojiID)
	}
	if cfg2.LanguageButtonEmojiID != 333 {
		t.Errorf("expected LanguageButtonEmojiID 333, got %d", cfg2.LanguageButtonEmojiID)
	}
	if cfg2.FastDownloadButtonEmojiID != 444 {
		t.Errorf("expected FastDownloadButtonEmojiID 444, got %d", cfg2.FastDownloadButtonEmojiID)
	}
	if cfg2.GreetingEmojiID != 555 {
		t.Errorf("expected GreetingEmojiID 555, got %d", cfg2.GreetingEmojiID)
	}
	if cfg2.QueryEmojiID != 666 {
		t.Errorf("expected QueryEmojiID 666, got %d", cfg2.QueryEmojiID)
	}
	if cfg2.PageTextEmojiID != 777 {
		t.Errorf("expected PageTextEmojiID 777, got %d", cfg2.PageTextEmojiID)
	}
	if cfg2.NotePrefixEmojiID != 888 {
		t.Errorf("expected NotePrefixEmojiID 888, got %d", cfg2.NotePrefixEmojiID)
	}
	if cfg2.NoteSuffixEmojiID != 999 {
		t.Errorf("expected NoteSuffixEmojiID 999, got %d", cfg2.NoteSuffixEmojiID)
	}
	if cfg2.FileSizePrefixEmojiID != 1010 {
		t.Errorf("expected FileSizePrefixEmojiID 1010, got %d", cfg2.FileSizePrefixEmojiID)
	}
	if cfg2.FileSizeSuffixEmojiID != 2020 {
		t.Errorf("expected FileSizeSuffixEmojiID 2020, got %d", cfg2.FileSizeSuffixEmojiID)
	}
}

