package latest

import (
	"context"
	"testing"

	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
	"github.com/itsafxal/MYTGBOTZ/internal/search"
)

type mockSearchService struct {
	latestFiles []search.MediaFile
	searchResults *search.SearchResult
}

func (m *mockSearchService) Search(ctx context.Context, query string, limit, offset int, filter search.SearchFilter) (*search.SearchResult, error) {
	if m.searchResults != nil {
		return m.searchResults, nil
	}
	return &search.SearchResult{}, nil
}

func (m *mockSearchService) GetSuggestions(ctx context.Context, query string, limit int) ([]string, error) {
	return nil, nil
}

func (m *mockSearchService) GetLatest(ctx context.Context, limit int) ([]search.MediaFile, error) {
	return m.latestFiles, nil
}

func TestLatestPlugin_Metadata(t *testing.T) {
	p := New(&plugins.AppContext{})
	if p.Name() != "latest" {
		t.Errorf("expected plugin name 'latest', got %q", p.Name())
	}

	cmds := p.Commands()
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands (/latest, /request), got %d", len(cmds))
	}

	cmdMap := make(map[string]plugins.CommandDefinition)
	for _, c := range cmds {
		cmdMap[c.Name] = c
	}

	if _, ok := cmdMap["latest"]; !ok {
		t.Errorf("missing command 'latest'")
	}
	if _, ok := cmdMap["request"]; !ok {
		t.Errorf("missing command 'request'")
	}
}

func TestLatestPlugin_SearchInteraction(t *testing.T) {
	mockSearch := &mockSearchService{
		latestFiles: []search.MediaFile{
			{FileID: "f1", FileName: "Movie1.mkv", Size: "1.2 GB"},
			{FileID: "f2", FileName: "Movie2.mkv", Size: "2.5 GB"},
		},
	}
	appCtx := &plugins.AppContext{
		Search: mockSearch,
	}
	p := New(appCtx)

	if p.appCtx == nil || p.appCtx.Search == nil {
		t.Fatal("expected appCtx.Search to be non-nil")
	}

	files, err := p.appCtx.Search.GetLatest(context.Background(), 10)
	if err != nil {
		t.Fatalf("GetLatest failed: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 latest files, got %d", len(files))
	}
	if files[0].FileName != "Movie1.mkv" {
		t.Errorf("expected Movie1.mkv, got %s", files[0].FileName)
	}
}
