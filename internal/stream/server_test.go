package stream

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/MYTGBOTZ/internal/delivery"
)

type mockMediaProvider struct {
	records map[string]*delivery.MediaDetails
}

func (m *mockMediaProvider) GetMedia(ctx context.Context, id string) (*delivery.MediaDetails, error) {
	if rec, ok := m.records[id]; ok {
		return rec, nil
	}
	return nil, delivery.ErrMediaNotFound
}

type mockTelegramDownloader struct {
	fileData []byte
}

func (m *mockTelegramDownloader) DownloadMedia(file any, opts ...*telegram.DownloadOptions) (string, error) {
	return "mock_file.mp4", nil
}

func (m *mockTelegramDownloader) DownloadChunk(media any, start int, end int, chunkSize int) ([]byte, string, error) {
	if start < 0 || start >= len(m.fileData) {
		return []byte{}, "mock_file.mp4", nil
	}
	chunkEnd := end
	if chunkEnd > len(m.fileData) {
		chunkEnd = len(m.fileData)
	}
	res := make([]byte, chunkEnd-start)
	copy(res, m.fileData[start:chunkEnd])
	return res, "mock_file.mp4", nil
}

func TestParseRangeHeader(t *testing.T) {
	const size = 1000

	tests := []struct {
		name      string
		header    string
		wantStart int64
		wantEnd   int64
		wantRange bool
		wantErr   bool
	}{
		{
			name:      "Empty header",
			header:    "",
			wantStart: 0,
			wantEnd:   999,
			wantRange: false,
			wantErr:   false,
		},
		{
			name:      "Standard range",
			header:    "bytes=0-499",
			wantStart: 0,
			wantEnd:   499,
			wantRange: true,
			wantErr:   false,
		},
		{
			name:      "Open ended range",
			header:    "bytes=500-",
			wantStart: 500,
			wantEnd:   999,
			wantRange: true,
			wantErr:   false,
		},
		{
			name:      "Suffix range",
			header:    "bytes=-200",
			wantStart: 800,
			wantEnd:   999,
			wantRange: true,
			wantErr:   false,
		},
		{
			name:      "Invalid range start > end",
			header:    "bytes=500-100",
			wantStart: 0,
			wantEnd:   0,
			wantRange: false,
			wantErr:   true,
		},
		{
			name:      "Out of bounds range",
			header:    "bytes=1500-2000",
			wantStart: 0,
			wantEnd:   0,
			wantRange: false,
			wantErr:   true,
		},
		{
			name:      "Malformed header",
			header:    "bytes=abc-xyz",
			wantStart: 0,
			wantEnd:   0,
			wantRange: false,
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			start, end, hasRange, err := parseRangeHeader(tc.header, size)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if start != tc.wantStart || end != tc.wantEnd || hasRange != tc.wantRange {
				t.Errorf("got (%d, %d, %v), want (%d, %d, %v)",
					start, end, hasRange, tc.wantStart, tc.wantEnd, tc.wantRange)
			}
		})
	}
}

func setupTestServer() (*Server, []byte) {
	// Create 2000 bytes test payload
	testBytes := make([]byte, 2000)
	for i := range testBytes {
		testBytes[i] = byte(i % 256)
	}

	provider := &mockMediaProvider{
		records: map[string]*delivery.MediaDetails{
			"valid_video": {
				FileID:    "valid_video",
				FileName:  "Avengers.mp4",
				FileSize:  int64(len(testBytes)),
				FileType:  "video",
				MimeType:  "video/mp4",
				Size:      "2.0 KB",
			},
			"valid_mkv": {
				FileID:    "valid_mkv",
				FileName:  "Kurup 2021 Malayalam 1080p 10bit NF WEBDL AAC 5 1 @MNTGX.mkv",
				FileSize:  int64(len(testBytes)),
				FileType:  "document",
				MimeType:  "video/x-matroska",
				Size:      "1.77 GB",
			},
			"valid_doc": {
				FileID:    "valid_doc",
				FileName:  "document.pdf",
				FileSize:  int64(len(testBytes)),
				FileType:  "document",
				MimeType:  "application/pdf",
				Size:      "2.0 KB",
			},
		},
	}

	downloader := &mockTelegramDownloader{
		fileData: testBytes,
	}

	cfg := Config{
		BaseURL:     "http://localhost:8080",
		Port:        8080,
		BindAddr:    "127.0.0.1",
		BotUsername: "FastTestBot",
		Enabled:     true,
	}

	srv := NewServer(cfg, provider, downloader, nil)
	return srv, testBytes
}

func TestHandleHealthz(t *testing.T) {
	srv, _ := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var res map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}
	if res["status"] != "ok" {
		t.Errorf("expected status ok, got %v", res["status"])
	}
}

func TestHandleWatch(t *testing.T) {
	srv, _ := setupTestServer()

	// 1. Existing video
	req := httptest.NewRequest(http.MethodGet, "/watch/valid_video", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Avengers.mp4") {
		t.Errorf("expected watch page to include filename, got: %s", body)
	}
	if !strings.Contains(body, "Fast Direct Download") {
		t.Errorf("expected watch page to include fast download button, got: %s", body)
	}
	if !strings.Contains(body, "<video") {
		t.Errorf("expected watch page to include HTML5 video element")
	}

	// 2. Non-existent file
	req404 := httptest.NewRequest(http.MethodGet, "/watch/nonexistent", nil)
	rec404 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec404, req404)

	if rec404.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", rec404.Code)
	}
}

func TestHandleStream_FullContent(t *testing.T) {
	srv, expectedBytes := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/stream/valid_video/Avengers.mp4", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if rec.Header().Get("Accept-Ranges") != "bytes" {
		t.Error("expected Accept-Ranges: bytes header")
	}
	if rec.Header().Get("Content-Type") != "video/mp4" {
		t.Errorf("expected Content-Type video/mp4, got %s", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "inline") {
		t.Errorf("expected inline disposition, got %s", rec.Header().Get("Content-Disposition"))
	}
	if !bytes.Equal(rec.Body.Bytes(), expectedBytes) {
		t.Errorf("streamed body does not match expected full content")
	}
}

func TestHandleStream_PartialContent(t *testing.T) {
	srv, expectedBytes := setupTestServer()

	// Request bytes 100-299 (200 bytes total)
	req := httptest.NewRequest(http.MethodGet, "/stream/valid_video", nil)
	req.Header.Set("Range", "bytes=100-299")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusPartialContent {
		t.Fatalf("expected 206 Partial Content, got %d", rec.Code)
	}
	if rec.Header().Get("Content-Range") != "bytes 100-299/2000" {
		t.Errorf("expected Content-Range bytes 100-299/2000, got %s", rec.Header().Get("Content-Range"))
	}
	if rec.Header().Get("Content-Length") != "200" {
		t.Errorf("expected Content-Length 200, got %s", rec.Header().Get("Content-Length"))
	}

	expectedSlice := expectedBytes[100:300]
	if !bytes.Equal(rec.Body.Bytes(), expectedSlice) {
		t.Errorf("streamed range slice does not match expected bytes")
	}
}

func TestHandleDownload_Attachment(t *testing.T) {
	srv, expectedBytes := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/dl/valid_video/Avengers.mp4", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	disp := rec.Header().Get("Content-Disposition")
	if !strings.Contains(disp, "attachment") {
		t.Errorf("expected attachment disposition for download endpoint, got %s", disp)
	}
	if !strings.Contains(disp, "Avengers.mp4") {
		t.Errorf("expected filename in disposition, got %s", disp)
	}
	if !bytes.Equal(rec.Body.Bytes(), expectedBytes) {
		t.Errorf("download body does not match expected full content")
	}
}

func TestHandleStream_RangeNotSatisfiable(t *testing.T) {
	srv, _ := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/stream/valid_video", nil)
	req.Header.Set("Range", "bytes=5000-6000") // out of bounds (file size 2000)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("expected 416 StatusRequestedRangeNotSatisfiable, got %d", rec.Code)
	}
	if rec.Header().Get("Content-Range") != "bytes */2000" {
		t.Errorf("expected Content-Range bytes */2000, got %s", rec.Header().Get("Content-Range"))
	}
}

func TestServerURLs(t *testing.T) {
	srv, _ := setupTestServer()

	streamURL := srv.GetStreamURL("file123", "Iron Man.mp4")
	if streamURL != "http://localhost:8080/stream/file123/Iron%20Man.mp4" {
		t.Errorf("unexpected StreamURL: %s", streamURL)
	}

	dlURL := srv.GetDownloadURL("file123", "Iron Man.mp4")
	if dlURL != "http://localhost:8080/dl/file123/Iron%20Man.mp4" {
		t.Errorf("unexpected DownloadURL: %s", dlURL)
	}

	watchURL := srv.GetWatchURL("file123")
	if watchURL != "http://localhost:8080/watch/file123" {
		t.Errorf("unexpected WatchURL: %s", watchURL)
	}

	playlistURL := srv.GetPlaylistURL("file123", "Iron Man.mp4")
	if playlistURL != "http://localhost:8080/playlist/file123/Iron%20Man.mp4.m3u" {
		t.Errorf("unexpected PlaylistURL: %s", playlistURL)
	}
}

func TestHandleWatch_MKV(t *testing.T) {
	srv, _ := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/watch/valid_mkv", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Kurup 2021") {
		t.Errorf("expected watch page to include Kurup title, got: %s", body)
	}
	if !strings.Contains(body, "MKV / 10-Bit Container Detected") {
		t.Errorf("expected watch page to include MKV format banner")
	}
	if !strings.Contains(body, "Open in VLC") {
		t.Errorf("expected watch page to include Open in VLC button")
	}
	if !strings.Contains(body, "PotPlayer") {
		t.Errorf("expected watch page to include PotPlayer for Windows")
	}
	if !strings.Contains(body, "MX Player") {
		t.Errorf("expected watch page to include MX Player for Android")
	}
	if !strings.Contains(body, "Outplayer") {
		t.Errorf("expected watch page to include Outplayer for iOS")
	}
}

func TestHandlePlaylist(t *testing.T) {
	srv, _ := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/playlist/valid_mkv/Kurup.m3u", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "audio/x-mpegurl; charset=utf-8" {
		t.Errorf("unexpected Content-Type: %s", rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("expected CORS header on playlist endpoint")
	}
	body := rec.Body.String()
	if !strings.Contains(body, "#EXTM3U") {
		t.Errorf("expected #EXTM3U header in playlist, got: %s", body)
	}
	if !strings.Contains(body, "Kurup 2021") {
		t.Errorf("expected movie title in playlist, got: %s", body)
	}
	if !strings.Contains(body, "/stream/valid_mkv/") {
		t.Errorf("expected stream URL in playlist, got: %s", body)
	}
}

func TestHandleStream_OPTIONS_CORS(t *testing.T) {
	srv, _ := setupTestServer()

	req := httptest.NewRequest(http.MethodOptions, "/stream/valid_video/Avengers.mp4", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for OPTIONS preflight, got %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("expected Access-Control-Allow-Origin header")
	}
	if !strings.Contains(rec.Header().Get("Access-Control-Allow-Methods"), "GET") {
		t.Error("expected GET in Access-Control-Allow-Methods")
	}
}
