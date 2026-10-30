package gfilter

import (
	"context"
	"strings"
	"testing"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/MYTGBOTZ/internal/config"
	coreGFilter "github.com/itsafxal/MYTGBOTZ/internal/gfilter"
	"github.com/itsafxal/MYTGBOTZ/internal/plugins"
)

type mockGFilterService struct {
	filters map[string]*coreGFilter.GFilter
}

func newMockGFilterService() *mockGFilterService {
	return &mockGFilterService{
		filters: make(map[string]*coreGFilter.GFilter),
	}
}

func (m *mockGFilterService) MatchAndExecute(msg *telegram.NewMessage) (bool, error) {
	if msg == nil {
		return false, nil
	}
	for kw := range m.filters {
		if strings.EqualFold(msg.Text(), kw) {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockGFilterService) SaveFilter(ctx context.Context, f *coreGFilter.GFilter) error {
	m.filters[coreGFilter.NormalizeKeyword(f.Keyword)] = f
	return nil
}

func (m *mockGFilterService) DeleteFilter(ctx context.Context, keyword string) (bool, error) {
	norm := coreGFilter.NormalizeKeyword(keyword)
	if _, ok := m.filters[norm]; ok {
		delete(m.filters, norm)
		return true, nil
	}
	return false, nil
}

func (m *mockGFilterService) DeleteAllFilters(ctx context.Context) (int64, error) {
	cnt := int64(len(m.filters))
	m.filters = make(map[string]*coreGFilter.GFilter)
	return cnt, nil
}

func (m *mockGFilterService) GetFilter(ctx context.Context, keyword string) (*coreGFilter.GFilter, error) {
	return m.filters[coreGFilter.NormalizeKeyword(keyword)], nil
}

func (m *mockGFilterService) ListFilters(ctx context.Context, limit, offset int) ([]*coreGFilter.GFilter, int64, error) {
	var list []*coreGFilter.GFilter
	for _, f := range m.filters {
		list = append(list, f)
	}
	return list, int64(len(list)), nil
}

func (m *mockGFilterService) Count(ctx context.Context) (int64, error) {
	return int64(len(m.filters)), nil
}

func TestGFilterPlugin_CommandsStructure(t *testing.T) {
	appCtx := &plugins.AppContext{
		Config:  &config.Config{OwnerID: 12345},
		GFilter: newMockGFilterService(),
	}

	p := New(appCtx)
	if p.Name() != "gfilter" {
		t.Errorf("Expected plugin name 'gfilter', got %q", p.Name())
	}

	cmds := p.Commands()
	if len(cmds) != 5 {
		t.Fatalf("Expected 5 commands registered, got %d", len(cmds))
	}

	cmdMap := make(map[string]plugins.CommandDefinition)
	for _, c := range cmds {
		cmdMap[c.Name] = c
		if !c.AdminOnly {
			t.Errorf("Command %s must be AdminOnly", c.Name)
		}
	}

	expectedCmds := []string{"gfilter", "gfilters", "viewgfilter", "delg", "delallg"}
	for _, name := range expectedCmds {
		if _, ok := cmdMap[name]; !ok {
			t.Errorf("Missing expected command: %s", name)
		}
	}

	cbHandlers := p.CallbackHandlers()
	if len(cbHandlers) < 4 {
		t.Errorf("Expected at least 4 callback handlers, got %d", len(cbHandlers))
	}
}

func TestParseKeywordAndResponse(t *testing.T) {
	tests := []struct {
		input       string
		wantKeyword string
		wantResp    string
		wantOK      bool
	}{
		{"Vikram Watch movie here", "Vikram", "Watch movie here", true},
		{"\"The One\" Movie link here", "The One", "Movie link here", true},
		{"SingleWord", "SingleWord", "", false},
		{"", "", "", false},
		{"   ", "", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			kw, resp, ok := parseKeywordAndResponse(tc.input)
			if ok != tc.wantOK {
				t.Fatalf("parseKeywordAndResponse(%q) ok = %v, want %v", tc.input, ok, tc.wantOK)
			}
			if ok {
				if kw != tc.wantKeyword {
					t.Errorf("keyword = %q, want %q", kw, tc.wantKeyword)
				}
				if resp != tc.wantResp {
					t.Errorf("resp = %q, want %q", resp, tc.wantResp)
				}
			}
		})
	}
}

func TestGFilterPlugin_CRUDThroughMock(t *testing.T) {
	mockGF := newMockGFilterService()
	appCtx := &plugins.AppContext{
		Config:  &config.Config{OwnerID: 12345},
		GFilter: mockGF,
	}
	p := New(appCtx)
	if p.Name() != "gfilter" {
		t.Fatalf("Unexpected plugin name: %s", p.Name())
	}
	ctx := context.Background()

	// 1. Add filter
	f := &coreGFilter.GFilter{
		Keyword:   "Vikram",
		ReplyText: "Vikram movie download",
		MediaType: coreGFilter.MediaTypeText,
	}
	_ = mockGF.SaveFilter(ctx, f)

	// 2. Retrieve
	retrieved, _ := mockGF.GetFilter(ctx, "vikram")
	if retrieved == nil || retrieved.Keyword != "Vikram" {
		t.Fatalf("Failed to retrieve filter: %+v", retrieved)
	}

	// 3. Count
	cnt, _ := mockGF.Count(ctx)
	if cnt != 1 {
		t.Errorf("Expected count 1, got %d", cnt)
	}

	// 4. Delete
	deleted, _ := mockGF.DeleteFilter(ctx, "VIKRAM")
	if !deleted {
		t.Errorf("Expected successful delete")
	}

	cntAfter, _ := mockGF.Count(ctx)
	if cntAfter != 0 {
		t.Errorf("Expected count 0, got %d", cntAfter)
	}
}
