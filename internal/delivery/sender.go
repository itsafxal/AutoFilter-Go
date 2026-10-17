package delivery

import (
	"github.com/amarnathcjd/gogram/telegram"
)

// TelegramSender abstracts media delivery operations via Telegram.
// *telegram.Client natively implements this interface.
type TelegramSender interface {
	SendMedia(peerID any, media any, opts ...*telegram.MediaOptions) (*telegram.NewMessage, error)
}
