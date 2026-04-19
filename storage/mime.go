// Package storage implements the Forge file storage engine.
// It provides content-addressed file storage, chunked uploads,
// MIME detection, and metadata tracking — all from scratch.
package storage

// mimeTypes maps file extensions to MIME types.
// Covers the most common web file types.
var mimeTypes = map[string]string{
	// Text
	".html": "text/html",
	".htm":  "text/html",
	".css":  "text/css",
	".js":   "application/javascript",
	".mjs":  "application/javascript",
	".json": "application/json",
	".xml":  "application/xml",
	".txt":  "text/plain",
	".csv":  "text/csv",
	".md":   "text/markdown",
	".yaml": "text/yaml",
	".yml":  "text/yaml",
	".svg":  "image/svg+xml",
	".ts":   "application/typescript",
	".tsx":  "application/typescript",

	// Images
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".bmp":  "image/bmp",
	".webp": "image/webp",
	".ico":  "image/x-icon",
	".avif": "image/avif",
	".tif":  "image/tiff",
	".tiff": "image/tiff",

	// Audio
	".mp3":  "audio/mpeg",
	".wav":  "audio/wav",
	".ogg":  "audio/ogg",
	".flac": "audio/flac",
	".aac":  "audio/aac",
	".m4a":  "audio/mp4",
	".weba": "audio/webm",

	// Video
	".mp4":  "video/mp4",
	".webm": "video/webm",
	".avi":  "video/x-msvideo",
	".mov":  "video/quicktime",
	".mkv":  "video/x-matroska",
	".m4v":  "video/mp4",

	// Documents
	".pdf":  "application/pdf",
	".doc":  "application/msword",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".xls":  "application/vnd.ms-excel",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".ppt":  "application/vnd.ms-powerpoint",
	".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",

	// Archives
	".zip":  "application/zip",
	".tar":  "application/x-tar",
	".gz":   "application/gzip",
	".bz2":  "application/x-bzip2",
	".7z":   "application/x-7z-compressed",
	".rar":  "application/vnd.rar",

	// Fonts
	".woff":  "font/woff",
	".woff2": "font/woff2",
	".ttf":   "font/ttf",
	".otf":   "font/otf",
	".eot":   "application/vnd.ms-fontobject",

	// WebAssembly
	".wasm": "application/wasm",
}

// magicBytes maps file signatures (magic bytes) to MIME types.
// Used as a fallback when the extension is missing or unknown.
var magicBytes = []struct {
	offset int
	magic  []byte
	mime   string
}{
	{0, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, "image/png"},
	{0, []byte{0xFF, 0xD8, 0xFF}, "image/jpeg"},
	{0, []byte("GIF87a"), "image/gif"},
	{0, []byte("GIF89a"), "image/gif"},
	{0, []byte("%PDF"), "application/pdf"},
	{0, []byte("PK\x03\x04"), "application/zip"},
	{0, []byte{0x1F, 0x8B}, "application/gzip"},
	{0, []byte("RIFF"), "audio/wav"}, // also used by WebP/AVI, but wav is most common
	{8, []byte("WEBP"), "image/webp"},
	{0, []byte{0x00, 0x00, 0x00}, "video/mp4"}, // ftyp box (rough match)
	{0, []byte{0x1A, 0x45, 0xDF, 0xA3}, "video/webm"},
	{0, []byte("ID3"), "audio/mpeg"},
	{0, []byte{0xFF, 0xFB}, "audio/mpeg"},
	{0, []byte{0xFF, 0xF3}, "audio/mpeg"},
	{0, []byte("OggS"), "audio/ogg"},
	{0, []byte("fLaC"), "audio/flac"},
	{0, []byte("wOFF"), "font/woff"},
	{0, []byte("wOF2"), "font/woff2"},
	{0, []byte{0x00, 0x61, 0x73, 0x6D}, "application/wasm"},
}

// DetectMIME returns the MIME type for a file based on its extension
// and content. Falls back to magic byte detection if extension is unknown.
func DetectMIME(filename string, content []byte) string {
	// Try extension first
	ext := fileExt(filename)
	if mime, ok := mimeTypes[ext]; ok {
		return mime
	}

	// Try magic bytes
	if len(content) > 0 {
		for _, mb := range magicBytes {
			end := mb.offset + len(mb.magic)
			if end <= len(content) {
				match := true
				for i, b := range mb.magic {
					if content[mb.offset+i] != b {
						match = false
						break
					}
				}
				if match {
					return mb.mime
				}
			}
		}
	}

	// Default
	return "application/octet-stream"
}

// IsImage returns true if the MIME type is an image.
func IsImage(mime string) bool {
	return len(mime) > 6 && mime[:6] == "image/"
}

// IsText returns true if the MIME type is text-based.
func IsText(mime string) bool {
	if len(mime) > 5 && mime[:5] == "text/" {
		return true
	}
	switch mime {
	case "application/json", "application/javascript",
		"application/xml", "application/typescript",
		"image/svg+xml":
		return true
	}
	return false
}

// fileExt returns the lowercase extension of a filename including the dot.
func fileExt(name string) string {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '.' {
			ext := name[i:]
			// Lowercase
			result := make([]byte, len(ext))
			for j, c := range ext {
				if c >= 'A' && c <= 'Z' {
					result[j] = byte(c + 32)
				} else {
					result[j] = byte(c)
				}
			}
			return string(result)
		}
		if name[i] == '/' || name[i] == '\\' {
			break
		}
	}
	return ""
}
