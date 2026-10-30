package caption

import (
	"context"
	"testing"

	"github.com/itsafxal/AutoFilter-Go/internal/caption"
	"github.com/itsafxal/AutoFilter-Go/internal/plugins"
)

type mockCaptionService struct {
	caption string
	custom  bool
}

func (m *mockCaptionService) GetCaption(ctx context.Context) string {
	if m.custom && m.caption != "" {
		return m.caption
	}
	return "{filename}\n\nFile Size : {file size}"
}

func (m *mockCaptionService) GetCustomCaption(ctx context.Context) (string, bool) {
	return m.caption, m.custom
}

func (m *mockCaptionService) SetCaption(ctx context.Context, template string) error {
	m.caption = template
	m.custom = true
	return nil
}

func (m *mockCaptionService) DeleteCaption(ctx context.Context) error {
	m.caption = ""
	m.custom = false
	return nil
}

func (m *mockCaptionService) IsCustom() bool {
	return m.custom
}

func (m *mockCaptionService) FormatMedia(ctx context.Context, fileName, sizeStr, originalCaption string) string {
	return caption.Format(m.GetCaption(ctx), fileName, sizeStr, originalCaption)
}

func (m *mockCaptionService) Format(template, fileName, sizeStr, originalCaption string) string {
	return caption.Format(template, fileName, sizeStr, originalCaption)
}

func (m *mockCaptionService) Init(ctx context.Context) error {
	return nil
}

func TestCaptionPlugin_CommandsMetadata(t *testing.T) {
	p := New(&plugins.AppContext{})
	if p.Name() != "caption" {
		t.Errorf("expected plugin name 'caption', got %q", p.Name())
	}

	cmds := p.Commands()
	if len(cmds) != 3 {
		t.Fatalf("expected 3 commands, got %d", len(cmds))
	}

	for _, cmd := range cmds {
		if !cmd.AdminOnly {
			t.Errorf("expected command %s to be AdminOnly", cmd.Name)
		}
		if cmd.Category != "Admin" {
			t.Errorf("expected command %s category Admin, got %s", cmd.Name, cmd.Category)
		}
		if cmd.Handler == nil {
			t.Errorf("expected non-nil handler for %s", cmd.Name)
		}
	}
}

func TestCaptionPlugin_ServiceInteraction(t *testing.T) {
	mockSvc := &mockCaptionService{}
	appCtx := &plugins.AppContext{
		Caption: mockSvc,
	}
	_ = New(appCtx)

	ctx := context.Background()

	// Initial check
	if mockSvc.IsCustom() {
		t.Error("expected IsCustom to be false initially")
	}

	// Set caption
	_ = mockSvc.SetCaption(ctx, "🎬 {filename} - {file sise}")
	if !mockSvc.IsCustom() {
		t.Error("expected IsCustom true after SetCaption")
	}
	out := mockSvc.FormatMedia(ctx, "Movie.mkv", "1.5 GB", "")
	if out != "🎬 Movie.mkv - 1.5 GB" {
		t.Errorf("unexpected FormatMedia output: %s", out)
	}

	// Delete caption
	_ = mockSvc.DeleteCaption(ctx)
	if mockSvc.IsCustom() {
		t.Error("expected IsCustom false after DeleteCaption")
	}
	outDefault := mockSvc.FormatMedia(ctx, "Movie.mkv", "1.5 GB", "")
	if outDefault != "Movie.mkv\n\nFile Size : 1.5 GB" {
		t.Errorf("unexpected default output: %s", outDefault)
	}
}
