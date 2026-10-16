package delivery

import (
	"github.com/PaulSonOfLars/gotgbot/v2"
)

// TelegramSender abstracts the Telegram Bot API file delivery methods.
// *gotgbot.Bot natively implements this interface.
type TelegramSender interface {
	SendDocument(chatId int64, document gotgbot.InputFileOrString, opts *gotgbot.SendDocumentOpts) (*gotgbot.Message, error)
	SendVideo(chatId int64, video gotgbot.InputFileOrString, opts *gotgbot.SendVideoOpts) (*gotgbot.Message, error)
	SendAudio(chatId int64, audio gotgbot.InputFileOrString, opts *gotgbot.SendAudioOpts) (*gotgbot.Message, error)
	SendPhoto(chatId int64, photo gotgbot.InputFileOrString, opts *gotgbot.SendPhotoOpts) (*gotgbot.Message, error)
	SendAnimation(chatId int64, animation gotgbot.InputFileOrString, opts *gotgbot.SendAnimationOpts) (*gotgbot.Message, error)
	SendVoice(chatId int64, voice gotgbot.InputFileOrString, opts *gotgbot.SendVoiceOpts) (*gotgbot.Message, error)
}
