package stream

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/MYTGBOTZ/internal/commands"
	"github.com/itsafxal/MYTGBOTZ/internal/delivery"
)

var (
	// ErrInvalidRange is returned when a requested Range header cannot be satisfied.
	ErrInvalidRange = errors.New("range not satisfiable")
)

const (
	defaultChunkSize = 1024 * 1024 // 1 MB per streaming chunk
)

// parseRangeHeader parses an RFC 7233 HTTP Range header.
func parseRangeHeader(header string, size int64) (start, end int64, hasRange bool, err error) {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0, size - 1, false, nil
	}

	if !strings.HasPrefix(header, "bytes=") {
		return 0, size - 1, false, nil
	}

	rangeVal := strings.TrimPrefix(header, "bytes=")
	// If multiple ranges, take the first
	if idx := strings.IndexByte(rangeVal, ','); idx >= 0 {
		rangeVal = rangeVal[:idx]
	}
	rangeVal = strings.TrimSpace(rangeVal)

	parts := strings.SplitN(rangeVal, "-", 2)
	if len(parts) != 2 {
		return 0, 0, false, ErrInvalidRange
	}

	startStr := strings.TrimSpace(parts[0])
	endStr := strings.TrimSpace(parts[1])

	if startStr == "" && endStr == "" {
		return 0, 0, false, ErrInvalidRange
	}

	if startStr == "" {
		// Suffix range: bytes=-500 (last 500 bytes)
		suffix, pErr := strconv.ParseInt(endStr, 10, 64)
		if pErr != nil || suffix <= 0 {
			return 0, 0, false, ErrInvalidRange
		}
		if suffix > size {
			suffix = size
		}
		start = size - suffix
		end = size - 1
		return start, end, true, nil
	}

	start, pErr := strconv.ParseInt(startStr, 10, 64)
	if pErr != nil || start < 0 {
		return 0, 0, false, ErrInvalidRange
	}

	if endStr == "" {
		// Open-ended range: bytes=500-
		end = size - 1
	} else {
		end, pErr = strconv.ParseInt(endStr, 10, 64)
		if pErr != nil || end < start {
			return 0, 0, false, ErrInvalidRange
		}
	}

	if start >= size || end >= size {
		return 0, 0, false, ErrInvalidRange
	}

	return start, end, true, nil
}

// Handler handles stream, download, watch page, and healthcheck requests.
type Handler struct {
	mediaProvider MediaProvider
	downloader    TelegramDownloader
	refresher     FileRefRefresher
	config        Config
	logger        *slog.Logger
	startTime     time.Time
}

// NewHandler creates a new stream HTTP request handler.
func NewHandler(provider MediaProvider, downloader TelegramDownloader, cfg Config, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		mediaProvider: provider,
		downloader:    downloader,
		config:        cfg,
		logger:        logger,
		startTime:     time.Now(),
	}
}

// SetDownloader updates the Telegram downloader client.
func (h *Handler) SetDownloader(d TelegramDownloader) {
	h.downloader = d
}

// SetRefresher updates the on-demand file reference refresher.
func (h *Handler) SetRefresher(r FileRefRefresher) {
	h.refresher = r
}

// HandleHealthz responds with service health status.
func (h *Handler) HandleHealthz(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(h.startTime).Truncate(time.Second)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `{"status":"ok","uptime":"%s","service":"telegram-stream"}`, uptime)
}

// HandleRoot provides a simple info landing page.
func (h *Handler) HandleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>Telegram Fast Stream Server</title>
<style>body{font-family:sans-serif;background:#0b0f19;color:#e2e8f0;display:flex;align-items:center;justify-content:center;height:100vh;margin:0;}</style>
</head>
<body>
<div style="text-align:center;">
<h1>⚡ Telegram Fast Stream & Download Server</h1>
<p style="color:#94a3b8;">High-speed HTTP streaming engine powered by Telegram MTProto.</p>
</div>
</body>
</html>`)
}

// extractFileID extracts the file ID from URL path: /<prefix>/<id> or /<prefix>/<id>/<optional_filename>
func extractFileID(path string, prefix string) (fileID string, fileName string) {
	clean := strings.TrimPrefix(path, prefix)
	clean = strings.TrimPrefix(clean, "/")
	if clean == "" {
		return "", ""
	}
	parts := strings.SplitN(clean, "/", 2)
	fileID = parts[0]
	if len(parts) > 1 {
		fileName, _ = url.PathUnescape(parts[1])
	}
	return fileID, fileName
}

// HandleWatch renders the HTML video/audio player web UI.
func (h *Handler) HandleWatch(w http.ResponseWriter, r *http.Request) {
	fileID, _ := extractFileID(r.URL.Path, "/watch")
	if fileID == "" {
		http.Error(w, "Missing file identifier", http.StatusBadRequest)
		return
	}

	rec, err := h.mediaProvider.GetMedia(r.Context(), fileID)
	if err != nil {
		if errors.Is(err, delivery.ErrMediaNotFound) {
			http.Error(w, "File not found or has been removed", http.StatusNotFound)
			return
		}
		h.logger.Error("failed to resolve media for watch page", "file_id", fileID, "error", err)
		http.Error(w, "Internal server error resolving file metadata", http.StatusInternalServerError)
		return
	}

	fileName := rec.FileName
	if fileName == "" {
		fileName = "Media_" + fileID
	}

	sizeStr := rec.Size
	if sizeStr == "" && rec.FileSize > 0 {
		sizeStr = commands.FormatBytes(rec.FileSize)
	}

	mimeType := DetectMimeType(fileName, rec.MimeType, rec.FileType)
	isVideo := IsVideoType(fileName, mimeType, rec.FileType)
	isAudio := IsAudioType(fileName, mimeType, rec.FileType)

	ext := strings.ToLower(filepath.Ext(fileName))
	isMKV := ext == ".mkv"
	isBrowserPlayable := isVideo && (ext == ".mp4" || ext == ".webm" || ext == ".ogg" || ext == ".mov")

	encodedFileName := url.PathEscape(fileName)
	streamURL := fmt.Sprintf("%s/stream/%s/%s", h.config.BaseURL, fileID, encodedFileName)
	downloadURL := fmt.Sprintf("%s/dl/%s/%s", h.config.BaseURL, fileID, encodedFileName)
	watchURL := fmt.Sprintf("%s/watch/%s", h.config.BaseURL, fileID)
	playlistURL := fmt.Sprintf("%s/playlist/%s/%s.m3u", h.config.BaseURL, fileID, encodedFileName)

	vlcURL := fmt.Sprintf("vlc://%s", streamURL)
	mxPlayerURL := fmt.Sprintf("intent:%s#Intent;package=com.mxtech.videoplayer.ad;type=%s;end", streamURL, mimeType)
	playItURL := fmt.Sprintf("playit://playit.app/video?url=%s", url.QueryEscape(streamURL))
	potPlayerURL := fmt.Sprintf("potplayer://%s", streamURL)
	vlcIOSURL := fmt.Sprintf("vlc-x-callback://x-callback-url/stream?url=%s", url.QueryEscape(streamURL))
	outplayerURL := fmt.Sprintf("outplayer://%s", streamURL)
	infuseURL := fmt.Sprintf("infuse://open?url=%s", url.QueryEscape(streamURL))

	pageData := PageData{
		Title:             fileName + " - Fast Stream & Download",
		FileName:          fileName,
		FileSizeStr:       sizeStr,
		FileSizeRaw:       rec.FileSize,
		MimeType:          mimeType,
		FileType:          strings.ToUpper(rec.FileType),
		IsVideo:           isVideo,
		IsAudio:           isAudio,
		IsMKV:             isMKV,
		IsBrowserPlayable: isBrowserPlayable,
		StreamURL:         streamURL,
		DownloadURL:       downloadURL,
		WatchURL:          watchURL,
		PlaylistURL:       playlistURL,
		BotUsername:       h.config.BotUsername,
		VLCUrl:            vlcURL,
		MXPlayerUrl:       mxPlayerURL,
		PlayItUrl:         playItURL,
		PotPlayerUrl:      potPlayerURL,
		VLCIOSUrl:         vlcIOSURL,
		OutplayerUrl:      outplayerURL,
		InfuseUrl:         infuseURL,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := RenderWatchPage(w, pageData); err != nil {
		h.logger.Error("failed to render watch page template", "error", err)
	}
}

// HandlePlaylist generates a downloadable .m3u playlist for 1-click external playback in VLC, PotPlayer, MX Player, etc.
func (h *Handler) HandlePlaylist(w http.ResponseWriter, r *http.Request) {
	prefix := "/playlist"
	if strings.HasPrefix(r.URL.Path, "/m3u") {
		prefix = "/m3u"
	}
	fileID, _ := extractFileID(r.URL.Path, prefix)
	if fileID == "" {
		http.Error(w, "Missing file identifier", http.StatusBadRequest)
		return
	}

	rec, err := h.mediaProvider.GetMedia(r.Context(), fileID)
	if err != nil {
		if errors.Is(err, delivery.ErrMediaNotFound) {
			http.Error(w, "File not found or has been removed", http.StatusNotFound)
			return
		}
		h.logger.Error("failed to resolve media for playlist", "file_id", fileID, "error", err)
		http.Error(w, "Internal server error resolving file metadata", http.StatusInternalServerError)
		return
	}

	fileName := rec.FileName
	if fileName == "" {
		fileName = "Media_" + fileID
	}

	encodedFileName := url.PathEscape(fileName)
	streamURL := fmt.Sprintf("%s/stream/%s/%s", h.config.BaseURL, fileID, encodedFileName)

	playlist := fmt.Sprintf("#EXTM3U\n#EXTINF:-1,%s\n%s\n", fileName, streamURL)

	baseName := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	if baseName == "" {
		baseName = "stream"
	}
	playlistFileName := baseName + ".m3u"

	w.Header().Set("Content-Type", "audio/x-mpegurl; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q; filename*=UTF-8''%s",
		playlistFileName, url.PathEscape(playlistFileName)))
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(playlist))
}

// HandleStream processes inline media streaming with HTTP Range requests.
func (h *Handler) HandleStream(w http.ResponseWriter, r *http.Request) {
	fileID, _ := extractFileID(r.URL.Path, "/stream")
	h.serveMedia(w, r, fileID, false)
}

// HandleDownload processes direct file download with Content-Disposition: attachment.
func (h *Handler) HandleDownload(w http.ResponseWriter, r *http.Request) {
	fileID, _ := extractFileID(r.URL.Path, "/dl")
	h.serveMedia(w, r, fileID, true)
}

// serveMedia resolves the media and streams the requested byte range.
func (h *Handler) serveMedia(w http.ResponseWriter, r *http.Request, fileID string, isAttachment bool) {
	// Enable CORS for web players and external streaming clients
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Range, Content-Type, Accept, Origin")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Range, Content-Length, Accept-Ranges")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if strings.TrimSpace(fileID) == "" {
		http.Error(w, "Missing file identifier", http.StatusBadRequest)
		return
	}

	if h.mediaProvider == nil {
		http.Error(w, "Media service unavailable", http.StatusServiceUnavailable)
		return
	}

	rec, err := h.mediaProvider.GetMedia(r.Context(), fileID)
	if err != nil {
		if errors.Is(err, delivery.ErrMediaNotFound) {
			http.Error(w, "File not found or has been removed", http.StatusNotFound)
			return
		}
		h.logger.Error("failed to resolve media for stream", "file_id", fileID, "error", err)
		http.Error(w, "Internal server error resolving file metadata", http.StatusInternalServerError)
		return
	}

	fileName := rec.FileName
	if fileName == "" {
		fileName = "file_" + fileID
	}
	totalSize := rec.FileSize
	if totalSize <= 0 {
		http.Error(w, "Invalid media file size", http.StatusInternalServerError)
		return
	}

	mimeType := DetectMimeType(fileName, rec.MimeType, rec.FileType)

	// Resolve native Telegram MTProto media object for download
	var mediaToDownload any = rec.FileID
	if resolvedDownload, rErr := delivery.ResolveDownloadMedia(rec.FileID, rec.FileRef, fileName, totalSize, mimeType); rErr == nil && resolvedDownload != nil {
		mediaToDownload = resolvedDownload
	} else if resolvedMedia, rErr := delivery.ResolveInputMedia(rec.FileID, rec.FileRef); rErr == nil && resolvedMedia != nil {
		mediaToDownload = resolvedMedia
	} else if resolvedBot, rErr := telegram.ResolveBotFileID(rec.FileID); rErr == nil && resolvedBot != nil {
		mediaToDownload = resolvedBot
	}

	// Parse Range header
	rangeHeader := r.Header.Get("Range")
	start, end, hasRange, rErr := parseRangeHeader(rangeHeader, totalSize)
	if errors.Is(rErr, ErrInvalidRange) {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", totalSize))
		http.Error(w, "Requested range not satisfiable", http.StatusRequestedRangeNotSatisfiable)
		return
	}

	// Setup headers
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Type", mimeType)

	dispositionType := "inline"
	if isAttachment {
		dispositionType = "attachment"
	}
	// Safe Content-Disposition header with filename and RFC 5987 utf-8 filename*
	w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename=%q; filename*=UTF-8''%s",
		dispositionType, fileName, url.PathEscape(fileName)))

	statusCode := http.StatusOK
	if hasRange {
		statusCode = http.StatusPartialContent
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, totalSize))
		w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
	} else {
		w.Header().Set("Content-Length", strconv.FormatInt(totalSize, 10))
	}

	// HEAD requests: return headers without body
	if r.Method == http.MethodHead {
		w.WriteHeader(statusCode)
		return
	}

	w.WriteHeader(statusCode)

	flusher, _ := w.(http.Flusher)

	// Stream chunks from Telegram MTProto directly to http.ResponseWriter
	chunkSize := defaultChunkSize
	ctx := r.Context()

	for offset := start; offset <= end; {
		if ctx.Err() != nil {
			// Client disconnected (e.g. paused video, closed tab, seeked to new position)
			return
		}

		reqEnd := offset + int64(chunkSize)
		if reqEnd > end+1 {
			reqEnd = end + 1
		}

		// Align to chunk boundary for Telegram API
		alignedOffset := (offset / int64(chunkSize)) * int64(chunkSize)
		alignedEnd := ((reqEnd + int64(chunkSize) - 1) / int64(chunkSize)) * int64(chunkSize)
		if alignedEnd > totalSize {
			alignedEnd = totalSize
		}

		if h.downloader == nil {
			h.logger.Error("telegram downloader client not configured")
			return
		}

		data, _, err := h.downloader.DownloadChunk(mediaToDownload, int(alignedOffset), int(alignedEnd), chunkSize)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			// Automatic on-demand refresh if Telegram file reference expired
			if isFileRefExpired(err) && h.refresher != nil {
				h.logger.Info("file reference expired, attempting on-demand refresh...", "file_id", fileID)
				refreshed, rErr := h.refresher.RefreshFileRef(ctx, fileID)
				if rErr == nil && refreshed != nil {
					mediaToDownload = refreshed
					data, _, err = h.downloader.DownloadChunk(mediaToDownload, int(alignedOffset), int(alignedEnd), chunkSize)
				} else if rErr != nil {
					h.logger.Warn("file reference refresh failed", "file_id", fileID, "error", rErr)
				}
			}
			if err != nil {
				h.logger.Warn("stream chunk download error from telegram",
					"file_id", fileID,
					"offset", alignedOffset,
					"error", err,
				)
				return
			}
		}

		if len(data) == 0 {
			return
		}

		sliceStart := offset - alignedOffset
		sliceLen := reqEnd - offset
		sliceEnd := sliceStart + sliceLen

		if sliceStart < 0 {
			sliceStart = 0
		}
		if sliceEnd > int64(len(data)) {
			sliceEnd = int64(len(data))
		}

		if sliceStart < int64(len(data)) {
			chunk := data[sliceStart:sliceEnd]
			if _, wErr := w.Write(chunk); wErr != nil {
				// Client disconnected
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}

		offset = reqEnd
	}
}

// isFileRefExpired checks if the error indicates that Telegram's cryptographic file reference has expired.
func isFileRefExpired(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "FILE_REFERENCE_EXPIRED") || strings.Contains(msg, "FILE_REFERENCE_INVALID")
}
