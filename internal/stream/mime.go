package stream

import (
	"mime"
	"path/filepath"
	"strings"
)

// DetectMimeType determines the appropriate HTTP Content-Type for a file.
func DetectMimeType(fileName, dbMime, fileType string) string {
	if clean := strings.TrimSpace(dbMime); clean != "" && clean != "application/octet-stream" {
		return clean
	}

	ext := strings.ToLower(filepath.Ext(fileName))
	if ext != "" {
		switch ext {
		case ".mp4":
			return "video/mp4"
		case ".mkv":
			return "video/x-matroska"
		case ".webm":
			return "video/webm"
		case ".mov":
			return "video/quicktime"
		case ".avi":
			return "video/x-msvideo"
		case ".flv":
			return "video/x-flv"
		case ".ts":
			return "video/mp2t"
		case ".m4v":
			return "video/x-m4v"
		case ".mp3":
			return "audio/mpeg"
		case ".m4a":
			return "audio/mp4"
		case ".flac":
			return "audio/flac"
		case ".ogg":
			return "audio/ogg"
		case ".wav":
			return "audio/wav"
		case ".aac":
			return "audio/aac"
		case ".pdf":
			return "application/pdf"
		case ".apk":
			return "application/vnd.android.package-archive"
		case ".zip":
			return "application/zip"
		case ".tar":
			return "application/x-tar"
		case ".gz":
			return "application/gzip"
		case ".7z":
			return "application/x-7z-compressed"
		}

		if detected := mime.TypeByExtension(ext); detected != "" {
			return detected
		}
	}

	switch strings.ToLower(fileType) {
	case "video":
		return "video/mp4"
	case "audio":
		return "audio/mpeg"
	case "photo":
		return "image/jpeg"
	default:
		return "application/octet-stream"
	}
}

// IsVideoType returns true if the file or MIME represents a video file.
func IsVideoType(fileName, mimeType, fileType string) bool {
	if strings.HasPrefix(strings.ToLower(mimeType), "video/") {
		return true
	}
	if strings.EqualFold(fileType, "video") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(fileName))
	switch ext {
	case ".mp4", ".mkv", ".webm", ".mov", ".avi", ".flv", ".ts", ".m4v", ".wmv", ".3gp":
		return true
	}
	return false
}

// IsAudioType returns true if the file or MIME represents an audio file.
func IsAudioType(fileName, mimeType, fileType string) bool {
	if strings.HasPrefix(strings.ToLower(mimeType), "audio/") {
		return true
	}
	if strings.EqualFold(fileType, "audio") || strings.EqualFold(fileType, "voice") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(fileName))
	switch ext {
	case ".mp3", ".m4a", ".flac", ".ogg", ".wav", ".aac", ".opus", ".wma":
		return true
	}
	return false
}
