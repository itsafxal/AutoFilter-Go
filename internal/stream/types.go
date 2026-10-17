package stream

import (
	"context"
	"net/http"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/itsafxal/MYTGBOTZ/internal/delivery"
)

// MediaProvider defines the interface to resolve media records by unique identifier.
type MediaProvider interface {
	GetMedia(ctx context.Context, identifier string) (*delivery.MediaDetails, error)
}

// TelegramDownloader defines Telegram MTProto media downloading operations.
// *telegram.Client natively implements this interface.
type TelegramDownloader interface {
	DownloadMedia(file any, opts ...*telegram.DownloadOptions) (string, error)
	DownloadChunk(media any, start int, end int, chunkSize int) ([]byte, string, error)
}

// FileRefRefresher defines an on-demand refresher for expired Telegram file references.
type FileRefRefresher interface {
	RefreshFileRef(ctx context.Context, fileID string) (any, error)
}

// Config defines settings for the stream and download server.
type Config struct {
	BaseURL     string
	Port        int
	BindAddr    string
	BotUsername string
	Enabled     bool
}

// StreamService defines operations exposed by the streaming engine.
type StreamService interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	GetStreamURL(fileID string, fileName string) string
	GetDownloadURL(fileID string, fileName string) string
	GetWatchURL(fileID string) string
	GetPlaylistURL(fileID string, fileName string) string
	Handler() http.Handler
}
