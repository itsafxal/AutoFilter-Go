package config

import (
	"os"
	"strings"
	"testing"
)

func TestLoadFromEnv_Success(t *testing.T) {
	os.Setenv("BOT_TOKEN", "123456789:ABCdefGhIJKlmNoPQRsTUVwxyZ")
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
	os.Setenv("OWNER_ID", "99887766")
	os.Unsetenv("ADMINS")
	os.Unsetenv("LOG_LEVEL")
	os.Unsetenv("DATABASE_URI")
	os.Unsetenv("DATABASE_NAME")
	os.Unsetenv("COLLECTION_NAME")
	os.Unsetenv("SQLITE_DB_PATH")
	defer func() {
		os.Unsetenv("BOT_TOKEN")
		os.Unsetenv("OWNER_ID")
	}()

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.LogLevel != LogLevelInfo {
		t.Errorf("expected default LogLevel to be INFO, got %s", cfg.LogLevel)
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
	if cfg.HasDatabase() {
		t.Error("expected HasDatabase to be false when DATABASE_URI is unset")
	}
}

func TestLoadFromEnv_MissingBotToken(t *testing.T) {
	os.Unsetenv("BOT_TOKEN")
	os.Setenv("OWNER_ID", "12345")
	defer os.Unsetenv("OWNER_ID")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error for missing BOT_TOKEN, got nil")
	}
	if !strings.Contains(err.Error(), "BOT_TOKEN") {
		t.Errorf("expected error message to mention BOT_TOKEN, got: %v", err)
	}
}

func TestLoadFromEnv_MissingOwnerID(t *testing.T) {
	os.Setenv("BOT_TOKEN", "123:abc")
	os.Unsetenv("OWNER_ID")
	defer os.Unsetenv("BOT_TOKEN")

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
	os.Setenv("OWNER_ID", "not_a_number")
	defer func() {
		os.Unsetenv("BOT_TOKEN")
		os.Unsetenv("OWNER_ID")
	}()

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error for non-numeric OWNER_ID, got nil")
	}
}

func TestLoadFromEnv_InvalidAdminID(t *testing.T) {
	os.Setenv("BOT_TOKEN", "123:abc")
	os.Setenv("OWNER_ID", "12345")
	os.Setenv("ADMINS", "123,invalid_admin,456")
	defer func() {
		os.Unsetenv("BOT_TOKEN")
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
	os.Setenv("OWNER_ID", "12345")
	os.Setenv("LOG_LEVEL", "TRACE_VERBOSE")
	defer func() {
		os.Unsetenv("BOT_TOKEN")
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
	dbURI := "mongodb+srv://botadmin:SuperSecretPass123!@cluster0.abc.mongodb.net/CyberTG?retryWrites=true"

	cfg := &Config{
		BotToken:       secret,
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

	maskedURI := MaskURI(dbURI)
	if strings.Contains(maskedURI, "SuperSecretPass123!") {
		t.Errorf("CRITICAL SECURITY FLAW: database password leaked in MaskURI(): %s", maskedURI)
	}
	if !strings.Contains(maskedURI, "***REDACTED***") {
		t.Errorf("expected MaskURI to contain ***REDACTED***, got: %s", maskedURI)
	}
}
