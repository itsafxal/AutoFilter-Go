package stream

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Server implements the StreamService HTTP server.
type Server struct {
	config        Config
	mediaProvider MediaProvider
	downloader    TelegramDownloader
	downloaderMu  sync.RWMutex
	handler       *Handler
	httpServer    *http.Server
	listener      net.Listener
	logger        *slog.Logger
	running       bool
	runningMu     sync.Mutex
}

// NewServer constructs a new stream and fast download HTTP server.
func NewServer(cfg Config, provider MediaProvider, downloader TelegramDownloader, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	h := NewHandler(provider, downloader, cfg, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.HandleHealthz)
	mux.HandleFunc("/watch/", h.HandleWatch)
	mux.HandleFunc("/stream/", h.HandleStream)
	mux.HandleFunc("/dl/", h.HandleDownload)
	mux.HandleFunc("/", h.HandleRoot)

	srv := &Server{
		config:        cfg,
		mediaProvider: provider,
		downloader:    downloader,
		handler:       h,
		logger:        logger,
	}

	srv.httpServer = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	return srv
}

// SetDownloader safely updates the Telegram MTProto downloader.
func (s *Server) SetDownloader(d TelegramDownloader) {
	s.downloaderMu.Lock()
	defer s.downloaderMu.Unlock()
	s.downloader = d
	if s.handler != nil {
		s.handler.SetDownloader(d)
	}
}

// Handler returns the HTTP handler for testing.
func (s *Server) Handler() http.Handler {
	return s.httpServer.Handler
}

// Start begins listening and serving HTTP traffic.
func (s *Server) Start(ctx context.Context) error {
	s.runningMu.Lock()
	defer s.runningMu.Unlock()

	if s.running {
		return nil
	}

	if !s.config.Enabled {
		s.logger.Info("stream and fast download server is disabled via configuration")
		return nil
	}

	bindAddr := s.config.BindAddr
	if bindAddr == "" {
		bindAddr = "0.0.0.0"
	}
	addr := fmt.Sprintf("%s:%d", bindAddr, s.config.Port)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		s.logger.Error("failed to bind stream server listener", "addr", addr, "error", err)
		return fmt.Errorf("stream server bind error on %s: %w", addr, err)
	}
	s.listener = ln
	s.running = true

	s.logger.Info("stream & fast download server listening",
		"address", addr,
		"public_url", s.config.BaseURL,
	)

	go func() {
		if err := s.httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("stream HTTP server terminated unexpectedly", "error", err)
		}
	}()

	return nil
}

// Stop gracefully shuts down the HTTP server.
func (s *Server) Stop(ctx context.Context) error {
	s.runningMu.Lock()
	defer s.runningMu.Unlock()

	if !s.running {
		return nil
	}

	s.logger.Info("shutting down stream & fast download server...")
	s.running = false

	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// GetStreamURL returns the direct inline streaming URL for a file.
func (s *Server) GetStreamURL(fileID string, fileName string) string {
	encoded := url.PathEscape(fileName)
	if encoded == "" {
		return fmt.Sprintf("%s/stream/%s", s.config.BaseURL, fileID)
	}
	return fmt.Sprintf("%s/stream/%s/%s", s.config.BaseURL, fileID, encoded)
}

// GetDownloadURL returns the direct fast download URL with attachment disposition.
func (s *Server) GetDownloadURL(fileID string, fileName string) string {
	encoded := url.PathEscape(fileName)
	if encoded == "" {
		return fmt.Sprintf("%s/dl/%s", s.config.BaseURL, fileID)
	}
	return fmt.Sprintf("%s/dl/%s/%s", s.config.BaseURL, fileID, encoded)
}

// GetWatchURL returns the web video player & details page URL.
func (s *Server) GetWatchURL(fileID string) string {
	return fmt.Sprintf("%s/watch/%s", s.config.BaseURL, fileID)
}
