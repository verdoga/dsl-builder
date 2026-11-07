package resourcefind

import (
	"strings"
	"testing"
)

func TestFormatForFixedTable(t *testing.T) {
	// Ожидания заданы ТЗ 9.3, а не MIME-базой ОС или содержимым файла.
	tests := []struct {
		extension string
		kind      string
		mimeType  string
	}{
		{".mp3", "audio", "audio/mpeg"},
		{".wav", "audio", "audio/wav"},
		{".ogg", "audio", "audio/ogg"},
		{".opus", "audio", "audio/ogg"},
		{".m4a", "audio", "audio/mp4"},
		{".aac", "audio", "audio/aac"},
		{".flac", "audio", "audio/flac"},
		{".mp4", "video", "video/mp4"},
		{".m4v", "video", "video/mp4"},
		{".webm", "video", "video/webm"},
		{".ogv", "video", "video/ogg"},
		{".mov", "video", "video/quicktime"},
		{".avi", "video", "video/x-msvideo"},
		{".mkv", "video", "video/x-matroska"},
		{".wmv", "video", "video/x-ms-wmv"},
		{".jpg", "image", "image/jpeg"},
		{".jpeg", "image", "image/jpeg"},
		{".png", "image", "image/png"},
		{".gif", "image", "image/gif"},
		{".webp", "image", "image/webp"},
		{".bmp", "image", "image/bmp"},
		{".svg", "image", "image/svg+xml"},
		{".tif", "image", "image/tiff"},
		{".tiff", "image", "image/tiff"},
		{".avif", "image", "image/avif"},
		{".txt", "script", "text/plain"},
		{".vtt", "subtitles", "text/vtt"},
		{".srt", "subtitles", "application/x-subrip"},
		{".ttml", "subtitles", "application/ttml+xml"},
		{".dfxp", "subtitles", "application/ttml+xml"},
		{".ass", "subtitles", "text/plain"},
		{".ssa", "subtitles", "text/plain"},
	}
	for _, test := range tests {
		t.Run(test.extension, func(t *testing.T) {
			// Перебираются все комбинации регистра букв, включая смешанные.
			letters := 0
			for _, ch := range test.extension {
				if ch >= 'a' && ch <= 'z' {
					letters++
				}
			}
			for mask := 0; mask < 1<<letters; mask++ {
				extension := []byte(test.extension)
				bit := 0
				for i, ch := range extension {
					if ch >= 'a' && ch <= 'z' {
						if mask&(1<<bit) != 0 {
							extension[i] = ch - ('a' - 'A')
						}
						bit++
					}
				}
				assertFormatFor(t, string(extension), test.kind, test.mimeType, true)
			}
		})
	}
}

func TestFormatForUnknownExtensionReturnsEmpty(t *testing.T) {
	tests := []string{
		"", ".", ".unknown", ".json", ".html", ".pdf", ".zip",
		".aiff", ".aif", ".wma", ".mpeg", ".mpg", ".3gp",
		".ico", ".heic", ".apng", ".sub", ".lrc", ".sbv",
		"mp3", "..mp3", ".mp3x", ".mp3.bak", "track.mp3",
		" .mp3", ".mp3 ", ".mp3\n", ".mp3\x00", ".mр3",
	}
	for _, extension := range tests {
		t.Run(extension, func(t *testing.T) {
			assertFormatFor(t, extension, "", "", false)
			assertFormatFor(t, strings.ToUpper(extension), "", "", false)
		})
	}
}

func TestFormatForCallsRemainIndependent(t *testing.T) {
	for range 3 {
		assertFormatFor(t, ".MP3", "audio", "audio/mpeg", true)
		assertFormatFor(t, ".unknown", "", "", false)
		assertFormatFor(t, ".TTML", "subtitles", "application/ttml+xml", true)
		assertFormatFor(t, "", "", "", false)
		assertFormatFor(t, ".txt", "script", "text/plain", true)
	}
}

func assertFormatFor(t *testing.T, extension, wantKind, wantMIME string, wantOK bool) {
	t.Helper()
	kind, mimeType, ok := formatFor(extension)
	if kind != wantKind || mimeType != wantMIME || ok != wantOK {
		t.Errorf("formatFor(%q) = (%q, %q, %v), want (%q, %q, %v)",
			extension, kind, mimeType, ok, wantKind, wantMIME, wantOK)
	}
}
