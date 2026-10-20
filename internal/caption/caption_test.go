package caption

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)


func TestFormat_Placeholders(t *testing.T) {
	tmpl := "{filename}\n\nFile Size : {file size}"
	got := Format(tmpl, "Avatar.2009.mkv", "2.10 GB", "Original Caption")
	want := "Avatar.2009.mkv\n\nFile Size : 2.10 GB"
	if got != want {
		t.Errorf("Format() = %q, want %q", got, want)
	}

	// Test alternative placeholders
	tmpl2 := "📁 File: {file_name}\n📦 Size: {filesize}\n📝 Note: {caption}"
	got2 := Format(tmpl2, "Oppenheimer.mkv", "4.50 GB", "Oppenheimer 4K UHD")
	if !strings.Contains(got2, "Oppenheimer.mkv") || !strings.Contains(got2, "4.50 GB") || !strings.Contains(got2, "Oppenheimer 4K UHD") {
		t.Errorf("unexpected Format output: %s", got2)
	}

	// Test empty template fallback
	got3 := Format("", "Movie.mp4", "1 GB", "My Movie")
	if got3 != "My Movie" {
		t.Errorf("expected original caption fallback, got %q", got3)
	}

	got4 := Format("", "Movie.mp4", "1 GB", "")
	if got4 != "Movie.mp4" {
		t.Errorf("expected filename fallback, got %q", got4)
	}
}

func TestService_SQLitePersistenceAndLifecycle(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed opening in-memory SQLite: %v", err)
	}

	defer db.Close()

	// Initialize sync_metadata table
	_, err = db.Exec(`CREATE TABLE sync_metadata (
		key TEXT PRIMARY KEY,
		value TEXT,
		updated_at TIMESTAMP
	);`)
	if err != nil {
		t.Fatalf("failed creating table: %v", err)
	}

	sqlProv := func() *sql.DB { return db }
	svc := NewService("{filename}\n\nFile Size : {file size}", nil, sqlProv, nil)

	ctx := context.Background()

	// 1. Initial state (default template)
	if svc.IsCustom() {
		t.Error("expected IsCustom to be false initially")
	}
	if svc.GetCaption(ctx) != "{filename}\n\nFile Size : {file size}" {
		t.Errorf("expected default template, got %q", svc.GetCaption(ctx))
	}

	// 2. Set new custom caption
	newTmpl := "🎬 {filename} - {file sise}"
	if err := svc.SetCaption(ctx, newTmpl); err != nil {
		t.Fatalf("SetCaption failed: %v", err)
	}

	if !svc.IsCustom() {
		t.Error("expected IsCustom to be true after SetCaption")
	}
	custom, ok := svc.GetCustomCaption(ctx)
	if !ok || custom != newTmpl {
		t.Errorf("expected custom %q, got %q (ok=%v)", newTmpl, custom, ok)
	}

	// Verify formatted output
	out := svc.FormatMedia(ctx, "Test.mkv", "500 MB", "")
	if out != "🎬 Test.mkv - 500 MB" {
		t.Errorf("FormatMedia() = %q, want '🎬 Test.mkv - 500 MB'", out)
	}

	// 3. Test reload via Init()
	svc2 := NewService("{filename}\n\nFile Size : {file size}", nil, sqlProv, nil)
	if err := svc2.Init(ctx); err != nil {
		t.Fatalf("Init() failed: %v", err)
	}
	if !svc2.IsCustom() || svc2.GetCaption(ctx) != newTmpl {
		t.Errorf("expected reloaded custom caption %q, got %q", newTmpl, svc2.GetCaption(ctx))
	}

	// 4. Delete custom caption
	if err := svc2.DeleteCaption(ctx); err != nil {
		t.Fatalf("DeleteCaption failed: %v", err)
	}
	if svc2.IsCustom() {
		t.Error("expected IsCustom false after DeleteCaption")
	}
	if svc2.GetCaption(ctx) != "{filename}\n\nFile Size : {file size}" {
		t.Errorf("expected fallback to default, got %q", svc2.GetCaption(ctx))
	}
}
