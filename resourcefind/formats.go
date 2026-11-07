package resourcefind

import "strings"

// formatFor — единственный источник фиксированных соответствий ТЗ 9.3.
// extension включает начальную точку, сравнение не учитывает регистр.
// kind равен audio, video, image, script или subtitles; ok ложен для прочих
// расширений, тогда обе строки пусты. Никакого определения MIME по байтам,
// реестру Windows, установленным кодекам или внешним настройкам нет.
//
// audio: .mp3 — audio/mpeg; .wav — audio/wav; .ogg, .opus — audio/ogg;
// .m4a — audio/mp4; .aac — audio/aac; .flac — audio/flac.
// video: .mp4, .m4v — video/mp4; .webm — video/webm; .ogv — video/ogg;
// .mov — video/quicktime; .avi — video/x-msvideo; .mkv — video/x-matroska;
// .wmv — video/x-ms-wmv.
// image: .jpg, .jpeg — image/jpeg; .png — image/png; .gif — image/gif;
// .webp — image/webp; .bmp — image/bmp; .svg — image/svg+xml;
// .tif, .tiff — image/tiff; .avif — image/avif.
// script: .txt — text/plain.
// subtitles: .vtt — text/vtt; .srt — application/x-subrip;
// .ttml, .dfxp — application/ttml+xml; .ass, .ssa — text/plain.
func formatFor(extension string) (kind string, mimeType string, ok bool) {
	switch strings.ToLower(extension) {
	case ".mp3":
		return "audio", "audio/mpeg", true
	case ".wav":
		return "audio", "audio/wav", true
	case ".ogg", ".opus":
		return "audio", "audio/ogg", true
	case ".m4a":
		return "audio", "audio/mp4", true
	case ".aac":
		return "audio", "audio/aac", true
	case ".flac":
		return "audio", "audio/flac", true
	case ".mp4", ".m4v":
		return "video", "video/mp4", true
	case ".webm":
		return "video", "video/webm", true
	case ".ogv":
		return "video", "video/ogg", true
	case ".mov":
		return "video", "video/quicktime", true
	case ".avi":
		return "video", "video/x-msvideo", true
	case ".mkv":
		return "video", "video/x-matroska", true
	case ".wmv":
		return "video", "video/x-ms-wmv", true
	case ".jpg", ".jpeg":
		return "image", "image/jpeg", true
	case ".png":
		return "image", "image/png", true
	case ".gif":
		return "image", "image/gif", true
	case ".webp":
		return "image", "image/webp", true
	case ".bmp":
		return "image", "image/bmp", true
	case ".svg":
		return "image", "image/svg+xml", true
	case ".tif", ".tiff":
		return "image", "image/tiff", true
	case ".avif":
		return "image", "image/avif", true
	case ".txt":
		return "script", "text/plain", true
	case ".vtt":
		return "subtitles", "text/vtt", true
	case ".srt":
		return "subtitles", "application/x-subrip", true
	case ".ttml", ".dfxp":
		return "subtitles", "application/ttml+xml", true
	case ".ass", ".ssa":
		return "subtitles", "text/plain", true
	default:
		return "", "", false
	}
}
