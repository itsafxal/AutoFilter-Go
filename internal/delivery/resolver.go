package delivery

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"strings"
	"sync"

	"github.com/amarnathcjd/gogram/telegram"
)

var (
	// FileRefCache stores fresh Telegram FileReference bytes keyed by file_id.
	FileRefCache sync.Map
)

// StoreFreshFileRef caches a fresh Telegram file reference byte slice for a file_id.
func StoreFreshFileRef(fileID string, ref []byte) {
	if fileID != "" && len(ref) > 0 {
		FileRefCache.Store(fileID, ref)
	}
}

// GetFreshFileRef retrieves a cached fresh Telegram file reference byte slice for a file_id.
func GetFreshFileRef(fileID string) ([]byte, bool) {
	if fileID == "" {
		return nil, false
	}
	if val, ok := FileRefCache.Load(fileID); ok {
		if ref, ok := val.([]byte); ok && len(ref) > 0 {
			return ref, true
		}
	}
	return nil, false
}

const (
	webLocationFlag   = 1 << 24
	fileReferenceFlag = 1 << 25
)

// ParsedFileID represents the unpacked parameters of a Telegram Bot API file_id.
type ParsedFileID struct {
	Major         int
	Minor         int
	FileType      int32
	DCID          int32
	MediaID       int64
	AccessHash    int64
	FileReference []byte
}

// rleDecode performs zero-byte Run-Length Decoding on serialized Telegram file_id bytes.
func rleDecode(s []byte) []byte {
	var r []byte
	z := false
	for _, b := range s {
		if b == 0 {
			z = true
			continue
		}
		if z {
			r = append(r, make([]byte, b)...)
			z = false
		} else {
			r = append(r, b)
		}
	}
	return r
}

// decodeBase64FileID decodes a file_id string trying URL-safe and standard base64 formats with padding.
func decodeBase64FileID(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("empty string")
	}
	padLen := (4 - (len(s) % 4)) % 4
	padded := s + strings.Repeat("=", padLen)

	if b, err := base64.URLEncoding.DecodeString(padded); err == nil {
		return b, nil
	}
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.StdEncoding.DecodeString(padded); err == nil {
		return b, nil
	}
	return base64.RawStdEncoding.DecodeString(s)
}

// readTLBytes reads Telegram TL-serialized bytes (length prefix + padding to 4 bytes).
func readTLBytes(buf *bytes.Reader) ([]byte, error) {
	first, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}

	var length int
	if first <= 253 {
		length = int(first)
		pad := (4 - ((length + 1) % 4)) % 4
		data := make([]byte, length)
		if _, err := buf.Read(data); err != nil {
			return nil, err
		}
		if pad > 0 {
			discard := make([]byte, pad)
			_, _ = buf.Read(discard)
		}
		return data, nil
	}

	// 254 indicates 3 bytes length in little-endian
	lenBytes := make([]byte, 3)
	if _, err := buf.Read(lenBytes); err != nil {
		return nil, err
	}
	length = int(lenBytes[0]) | (int(lenBytes[1]) << 8) | (int(lenBytes[2]) << 16)
	pad := (4 - (length % 4)) % 4
	data := make([]byte, length)
	if _, err := buf.Read(data); err != nil {
		return nil, err
	}
	if pad > 0 {
		discard := make([]byte, pad)
		_, _ = buf.Read(discard)
	}
	return data, nil
}

// UnpackFileID unpacks a Telegram Bot API file_id string into MTProto routing components.
func UnpackFileID(fileID string) (*ParsedFileID, error) {
	raw, err := decodeBase64FileID(fileID)
	if err != nil {
		return nil, err
	}

	// Telegram Bot API file IDs use zero-byte RLE encoding
	decoded := rleDecode(raw)
	if len(decoded) < 10 {
		decoded = raw
	}
	if len(decoded) < 10 {
		return nil, errors.New("file_id payload too short")
	}

	// Read version from end
	major := int(decoded[len(decoded)-1])
	var minor int
	body := decoded[:len(decoded)-1]
	if major >= 4 && len(decoded) >= 2 {
		minor = int(decoded[len(decoded)-2])
		body = decoded[:len(decoded)-2]
	}

	buf := bytes.NewReader(body)
	if buf.Len() < 8 {
		return nil, errors.New("insufficient header bytes in file_id")
	}

	var rawFileType, dcID int32
	if err := binary.Read(buf, binary.LittleEndian, &rawFileType); err != nil {
		return nil, err
	}
	if err := binary.Read(buf, binary.LittleEndian, &dcID); err != nil {
		return nil, err
	}

	hasWebLocation := (rawFileType & webLocationFlag) != 0
	hasFileRef := (rawFileType & fileReferenceFlag) != 0
	cleanFileType := rawFileType &^ (webLocationFlag | fileReferenceFlag)

	if hasWebLocation {
		return nil, errors.New("web location file_ids are not supported for MTProto document delivery")
	}

	var fileRef []byte
	if hasFileRef {
		ref, err := readTLBytes(buf)
		if err == nil {
			fileRef = ref
		}
	}

	if buf.Len() < 16 {
		return nil, errors.New("insufficient bytes for media_id and access_hash in file_id")
	}

	var mediaID, accessHash int64
	if err := binary.Read(buf, binary.LittleEndian, &mediaID); err != nil {
		return nil, err
	}
	if err := binary.Read(buf, binary.LittleEndian, &accessHash); err != nil {
		return nil, err
	}

	return &ParsedFileID{
		Major:         major,
		Minor:         minor,
		FileType:      cleanFileType,
		DCID:          dcID,
		MediaID:       mediaID,
		AccessHash:    accessHash,
		FileReference: fileRef,
	}, nil
}

// ResolveInputMedia converts a file_id and optional file_ref from the database into a native MTProto InputMedia.
func ResolveInputMedia(fileID string, fileRefStr string) (telegram.InputMedia, error) {
	parsed, err := UnpackFileID(fileID)
	if err != nil {
		// Try Gogram's built-in parser as fallback
		if res, errGogram := telegram.ResolveBotFileID(fileID); errGogram == nil && res != nil {
			switch m := res.(type) {
			case *telegram.MessageMediaDocument:
				if doc, ok := m.Document.(*telegram.DocumentObj); ok {
					return &telegram.InputMediaDocument{
						ID: &telegram.InputDocumentObj{
							ID:            doc.ID,
							AccessHash:    doc.AccessHash,
							FileReference: doc.FileReference,
						},
					}, nil
				}
			case *telegram.MessageMediaPhoto:
				if photo, ok := m.Photo.(*telegram.PhotoObj); ok {
					return &telegram.InputMediaPhoto{
						ID: &telegram.InputPhotoObj{
							ID:            photo.ID,
							AccessHash:    photo.AccessHash,
							FileReference: photo.FileReference,
						},
					}, nil
				}
			}
		}
		return nil, err
	}

	// Determine file reference
	fileRef := parsed.FileReference
	if len(fileRef) == 0 && strings.TrimSpace(fileRefStr) != "" {
		if refBytes, err := decodeBase64FileID(fileRefStr); err == nil {
			fileRef = refBytes
		}
	}

	// Photo type (FileType 2)
	if parsed.FileType == 2 {
		return &telegram.InputMediaPhoto{
			ID: &telegram.InputPhotoObj{
				ID:            parsed.MediaID,
				AccessHash:    parsed.AccessHash,
				FileReference: fileRef,
			},
		}, nil
	}

	// All other types (document, video, audio, voice, sticker, animation) map to InputMediaDocument
	return &telegram.InputMediaDocument{
		ID: &telegram.InputDocumentObj{
			ID:            parsed.MediaID,
			AccessHash:    parsed.AccessHash,
			FileReference: fileRef,
		},
	}, nil
}

// ResolveDownloadMedia converts a file_id, file_ref, and metadata into a native MTProto media object
// (*telegram.DocumentObj or *telegram.PhotoObj) that is natively supported by Gogram's GetFileLocation
// and DownloadChunk for streaming and direct HTTP downloads.
func ResolveDownloadMedia(fileID string, fileRefStr string, fileName string, fileSize int64, mimeType string) (any, error) {
	parsed, err := UnpackFileID(fileID)
	if err != nil {
		// Try Gogram's built-in parser as fallback
		if res, errGogram := telegram.ResolveBotFileID(fileID); errGogram == nil && res != nil {
			switch m := res.(type) {
			case *telegram.MessageMediaDocument:
				if doc, ok := m.Document.(*telegram.DocumentObj); ok {
					if fileSize > 0 {
						doc.Size = fileSize
					}
					if mimeType != "" {
						doc.MimeType = mimeType
					}
					if fileName != "" {
						doc.Attributes = append(doc.Attributes, &telegram.DocumentAttributeFilename{
							FileName: fileName,
						})
					}
					return doc, nil
				}
			case *telegram.MessageMediaPhoto:
				if photo, ok := m.Photo.(*telegram.PhotoObj); ok {
					return photo, nil
				}
			}
			return res, nil
		}
		return nil, err
	}

	// Determine file reference, prioritizing fresh in-memory cache
	var fileRef []byte
	if cachedRef, ok := GetFreshFileRef(fileID); ok && len(cachedRef) > 0 {
		fileRef = cachedRef
	} else {
		fileRef = parsed.FileReference
		if len(fileRef) == 0 && strings.TrimSpace(fileRefStr) != "" {
			if refBytes, err := decodeBase64FileID(fileRefStr); err == nil {
				fileRef = refBytes
			}
		}
	}

	// Photo type (FileType 2)
	if parsed.FileType == 2 {
		return &telegram.PhotoObj{
			ID:            parsed.MediaID,
			AccessHash:    parsed.AccessHash,
			FileReference: fileRef,
			DcID:          parsed.DCID,
			Sizes: []telegram.PhotoSize{
				&telegram.PhotoSizeObj{
					Type: "x",
					Size: int32(fileSize),
				},
			},
		}, nil
	}

	// All other types (document, video, audio, etc.) map to DocumentObj
	var attrs []telegram.DocumentAttribute
	if fileName != "" {
		attrs = append(attrs, &telegram.DocumentAttributeFilename{
			FileName: fileName,
		})
	}

	return &telegram.DocumentObj{
		ID:            parsed.MediaID,
		AccessHash:    parsed.AccessHash,
		FileReference: fileRef,
		DcID:          parsed.DCID,
		Size:          fileSize,
		MimeType:      mimeType,
		Attributes:    attrs,
	}, nil
}
