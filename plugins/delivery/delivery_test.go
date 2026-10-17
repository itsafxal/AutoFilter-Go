package delivery

import (
	"context"
	"strings"
	"testing"
	"time"

	deliveryCore "github.com/itsafxal/MYTGBOTZ/internal/delivery"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
)

// mockDeliveryService implements deliveryCore.MediaDeliveryService for plugin unit testing.
type mockDeliveryService struct {
	lastChatID     int64
	lastIdentifier string
	lastOpts       deliveryCore.DeliveryOptions
	returnResult   *deliveryCore.DeliveryResult
	returnErr      error
}

func (m *mockDeliveryService) GetMedia(ctx context.Context, identifier string) (*deliveryCore.MediaDetails, error) {
	if m.returnErr != nil {
		return nil, m.returnErr
	}
	return &deliveryCore.MediaDetails{
		FileID:   identifier,
		FileName: "test.pdf",
		FileSize: 1024,
		FileType: "document",
	}, nil
}

func (m *mockDeliveryService) ValidateMediaRecord(record *deliveryCore.MediaDetails) error {
	return nil
}

func (m *mockDeliveryService) Deliver(ctx context.Context, chatID int64, identifier string, opts deliveryCore.DeliveryOptions) (*deliveryCore.DeliveryResult, error) {
	m.lastChatID = chatID
	m.lastIdentifier = identifier
	m.lastOpts = opts
	if m.returnErr != nil {
		return nil, m.returnErr
	}
	return &deliveryCore.DeliveryResult{
		Success:        true,
		MessageID:      42,
		ChatID:         chatID,
		FileID:         identifier,
		FileName:       "test.pdf",
		FileType:       "document",
		DeliveryMethod: "SendDocument",
		Duration:       5 * time.Millisecond,
	}, nil
}

func (m *mockDeliveryService) SetSender(sender deliveryCore.TelegramSender) {}

func (m *mockDeliveryService) SetCaptionFormatter(cf deliveryCore.CaptionFormatter) {}

func (m *mockDeliveryService) SetAdminConfig(ownerID int64, admins []int64) {}

func (m *mockDeliveryService) RefreshFileRef(ctx context.Context, fileID string) (any, error) {
	return nil, nil
}

func (m *mockDeliveryService) SaveMedia(ctx context.Context, media *deliveryCore.MediaDetails) error {
	return nil
}


func TestDeliveryPlugin_Metadata(t *testing.T) {
	appCtx := &plugins.AppContext{}
	p := New(appCtx)

	if p.Name() != "delivery" {
		t.Errorf("expected plugin name 'delivery', got %q", p.Name())
	}

	cmds := p.Commands()
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}

	if cmds[0].Name != "getfile" {
		t.Errorf("expected command 'getfile', got %q", cmds[0].Name)
	}

	if len(cmds[0].Aliases) == 0 || cmds[0].Aliases[0] != "file" {
		t.Errorf("expected alias 'file', got %v", cmds[0].Aliases)
	}

	cbs := p.CallbackHandlers()
	if len(cbs) != 2 {
		t.Fatalf("expected 2 callback handlers, got %d", len(cbs))
	}

	prefixes := []string{cbs[0].Prefix, cbs[1].Prefix}
	if !strings.Contains(strings.Join(prefixes, ","), "file#") || !strings.Contains(strings.Join(prefixes, ","), "checksub#") {
		t.Errorf("expected file# and checksub# callback prefixes, got %v", prefixes)
	}
}

func TestDeliveryPlugin_CommandsInterface(t *testing.T) {
	mockDelivery := &mockDeliveryService{}
	appCtx := &plugins.AppContext{
		Delivery: mockDelivery,
	}

	p := New(appCtx)
	cmds := p.Commands()
	if cmds[0].Handler == nil {
		t.Fatal("expected non-nil handler for getfile")
	}

	// Verify Category and Description
	if cmds[0].Category != "Media" {
		t.Errorf("expected Category 'Media', got %q", cmds[0].Category)
	}
	if !strings.Contains(cmds[0].Description, "deliver") {
		t.Errorf("expected Description to mention deliver, got %q", cmds[0].Description)
	}
}
