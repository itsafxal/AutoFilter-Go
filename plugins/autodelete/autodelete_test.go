package autodelete

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	coreAutoDel "github.com/itsafxal/AutoFilter-Go/internal/autodelete"
	"github.com/itsafxal/AutoFilter-Go/internal/config"
	"github.com/itsafxal/AutoFilter-Go/internal/plugins"
)

type mockAutoDeleteService struct {
	settings coreAutoDel.Settings
}

func (m *mockAutoDeleteService) Init(ctx context.Context, client *telegram.Client) error {
	return nil
}
func (m *mockAutoDeleteService) Start(ctx context.Context) {}
func (m *mockAutoDeleteService) Stop()                     {}
func (m *mockAutoDeleteService) ScheduleDelete(chatID int64, messageID int32, msgType string, duration time.Duration) error {
	return nil
}
func (m *mockAutoDeleteService) ScheduleFilterDelete(chatID int64, messageID int32) error {
	return nil
}
func (m *mockAutoDeleteService) ScheduleFileDelete(chatID int64, messageID int32) error {
	return nil
}
func (m *mockAutoDeleteService) ScheduleFastDownloadDelete(chatID int64, messageID int32) error {
	return nil
}
func (m *mockAutoDeleteService) TrackSentMessage(chatID int64, messageID int32, msgType string) error {
	return nil
}
func (m *mockAutoDeleteService) GetSettings() coreAutoDel.Settings {
	return m.settings
}
func (m *mockAutoDeleteService) SetFilterDuration(ctx context.Context, d time.Duration) error {
	m.settings.FilterDeleteDuration = d
	return nil
}
func (m *mockAutoDeleteService) SetFileDuration(ctx context.Context, d time.Duration) error {
	m.settings.FileDeleteDuration = d
	return nil
}
func (m *mockAutoDeleteService) SetFastDownloadDuration(ctx context.Context, d time.Duration) error {
	m.settings.FastDownloadDeleteDuration = d
	return nil
}
func (m *mockAutoDeleteService) SetWeeklyTrash(ctx context.Context, enabled bool) error {
	m.settings.WeeklyTrashEnabled = enabled
	return nil
}
func (m *mockAutoDeleteService) SetTrashRetention(ctx context.Context, days int) error {
	m.settings.TrashRetentionDays = days
	return nil
}
func (m *mockAutoDeleteService) RunWeeklyTrash(ctx context.Context) (int64, error) {
	return 42, nil
}
func (m *mockAutoDeleteService) GetStats(ctx context.Context) (coreAutoDel.TrashStats, error) {
	return coreAutoDel.TrashStats{
		PendingDeletionsCount: 5,
		TrackedMessagesCount:  20,
		LastCleanedAt:         time.Now(),
		LastDeletedCount:      10,
	}, nil
}

func TestAutoDeletePluginRegistration(t *testing.T) {
	cfg := &config.Config{
		OwnerID: 111111,
		Admins:  []int64{222222},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockSvc := &mockAutoDeleteService{
		settings: coreAutoDel.Settings{
			FilterDeleteDuration: 15 * time.Minute,
			FileDeleteDuration:   30 * time.Minute,
			WeeklyTrashEnabled:   true,
			TrashRetentionDays:   7,
		},
	}

	appCtx := &plugins.AppContext{
		Config:     cfg,
		Logger:     logger,
		AutoDelete: mockSvc,
	}

	plugin := New(appCtx, mockSvc)
	if plugin.Name() != "autodelete" {
		t.Fatalf("expected plugin name 'autodelete', got %s", plugin.Name())
	}

	cmds := plugin.Commands()
	if len(cmds) != 4 {
		t.Fatalf("expected 4 commands, got %d", len(cmds))
	}

	expectedCmds := map[string]bool{
		"autodel":        true,
		"set_filter_del": true,
		"set_file_del":   true,
		"clean_trash":    true,
	}

	for _, cmd := range cmds {
		if !expectedCmds[cmd.Name] {
			t.Errorf("unexpected command: %s", cmd.Name)
		}
		if !cmd.AdminOnly {
			t.Errorf("expected command %s to be AdminOnly", cmd.Name)
		}
		if cmd.Handler == nil {
			t.Errorf("command %s has nil handler", cmd.Name)
		}
	}

	// Register in Registry and validate
	reg := plugins.NewRegistry(appCtx)
	if err := reg.Register(plugin); err != nil {
		t.Fatalf("failed to register plugin: %v", err)
	}
	if err := reg.Validate(); err != nil {
		t.Fatalf("failed to validate registry: %v", err)
	}
}
