package channel

import (
	"testing"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/MYTGBOTZ/internal/config"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
)

func TestChannelPlugin_Metadata(t *testing.T) {
	cfg := &config.Config{
		Channels: []int64{-1001234567890},
	}
	appCtx := &plugins.AppContext{
		Config: cfg,
	}

	p := New(appCtx)
	if p.Name() != "channel" {
		t.Errorf("expected plugin name 'channel', got %s", p.Name())
	}
	if len(p.Commands()) != 0 {
		t.Errorf("expected 0 commands, got %d", len(p.Commands()))
	}
	handlers := p.MessageHandlers()
	if len(handlers) != 1 {
		t.Fatalf("expected 1 message handler, got %d", len(handlers))
	}

	// Test nil message filter
	if handlers[0].Filter(nil) {
		t.Error("expected filter to reject nil message")
	}
}

func TestExtractFileName_Attributes(t *testing.T) {
	doc := &telegram.DocumentObj{
		Attributes: []telegram.DocumentAttribute{
			&telegram.DocumentAttributeFilename{
				FileName: "Inception.2010.1080p.mkv",
			},
		},
	}
	name := extractFileName(doc)
	if name != "Inception.2010.1080p.mkv" {
		t.Errorf("expected 'Inception.2010.1080p.mkv', got %s", name)
	}

	// Audio attributes fallback
	audioDoc := &telegram.DocumentObj{
		Attributes: []telegram.DocumentAttribute{
			&telegram.DocumentAttributeAudio{
				Title:     "Song Title",
				Performer: "Artist",
			},
		},
	}
	audioName := extractFileName(audioDoc)
	if audioName != "Artist - Song Title" {
		t.Errorf("expected 'Artist - Song Title', got %s", audioName)
	}
}

func TestExtractMediaFromMessage_Nil(t *testing.T) {
	res := ExtractMediaFromMessage(nil)
	if res != nil {
		t.Errorf("expected nil for nil message, got %+v", res)
	}
}
