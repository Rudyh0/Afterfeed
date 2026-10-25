// Package media recognises media files, reads their properties and creates
// thumbnails and previews.
package media

import (
	"bytes"
	"path"
	"strings"
)

// Kind is the broad type of a file.
type Kind string

const (
	Photo    Kind = "photo"
	Video    Kind = "video"
	Audio    Kind = "audio"
	Document Kind = "document"
	Other    Kind = "other"
)

// Type describes a recognised file.
type Type struct {
	Kind Kind
	Mime string
	// Ext is the canonical extension, including the dot.
	Ext string
}

// SniffLen is how many bytes Sniff needs.
const SniffLen = 64

// Sniff recognises a file from its first bytes and falls back to the name
// when the content is not conclusive. Exports often use wrong or missing
// extensions, so the content wins.
func Sniff(head []byte, name string) Type {
	ext := strings.ToLower(path.Ext(name))
	h := head
	has := func(off int, sig string) bool {
		return len(h) >= off+len(sig) && string(h[off:off+len(sig)]) == sig
	}
	switch {
	case has(0, "\xff\xd8\xff"):
		return Type{Photo, "image/jpeg", ".jpg"}
	case has(0, "\x89PNG\r\n\x1a\n"):
		return Type{Photo, "image/png", ".png"}
	case has(0, "GIF87a"), has(0, "GIF89a"):
		return Type{Photo, "image/gif", ".gif"}
	case has(0, "RIFF") && has(8, "WEBP"):
		return Type{Photo, "image/webp", ".webp"}
	case has(0, "RIFF") && has(8, "WAVE"):
		return Type{Audio, "audio/wav", ".wav"}
	case has(0, "RIFF") && has(8, "AVI "):
		return Type{Video, "video/x-msvideo", ".avi"}
	case has(0, "BM") && len(h) > 14 && ext == ".bmp":
		return Type{Photo, "image/bmp", ".bmp"}
	case has(0, "II*\x00"), has(0, "MM\x00*"):
		if ext == ".tif" || ext == ".tiff" || ext == "" {
			return Type{Photo, "image/tiff", ".tif"}
		}
		return Type{Other, "application/octet-stream", ext}
	case has(4, "ftyp"):
		return sniffFtyp(h, ext)
	case has(0, "\x1a\x45\xdf\xa3"):
		if bytes.Contains(h, []byte("webm")) {
			return Type{Video, "video/webm", ".webm"}
		}
		return Type{Video, "video/x-matroska", ".mkv"}
	case has(0, "OggS"):
		return Type{Audio, "audio/ogg", ".ogg"}
	case has(0, "ID3"), len(h) > 1 && h[0] == 0xff && (h[1] == 0xfb || h[1] == 0xf3 || h[1] == 0xf2):
		return Type{Audio, "audio/mpeg", ".mp3"}
	case len(h) > 1 && h[0] == 0xff && (h[1] == 0xf1 || h[1] == 0xf9):
		return Type{Audio, "audio/aac", ".aac"}
	case has(0, "#!AMR"):
		return Type{Audio, "audio/amr", ".amr"}
	case has(0, "fLaC"):
		return Type{Audio, "audio/flac", ".flac"}
	case has(0, "%PDF"):
		return Type{Document, "application/pdf", ".pdf"}
	case has(0, "PK\x03\x04"):
		if t, ok := documentTypes[ext]; ok {
			return t
		}
		return Type{Other, "application/zip", ".zip"}
	case has(0, "\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1"):
		if t, ok := documentTypes[ext]; ok {
			return t
		}
		return Type{Document, "application/octet-stream", ext}
	}
	if t, ok := documentTypes[ext]; ok {
		return t
	}
	if t, ok := extTypes[ext]; ok {
		return t
	}
	return Type{Other, "application/octet-stream", ext}
}

func sniffFtyp(h []byte, ext string) Type {
	brand := ""
	if len(h) >= 12 {
		brand = strings.TrimSpace(string(h[8:12]))
	}
	compat := ""
	if len(h) >= 16 {
		compat = string(h[16:min(len(h), 48)])
	}
	switch brand {
	case "heic", "heix", "hevc", "hevx", "heim", "heis", "mif1", "msf1":
		if brand == "mif1" && strings.Contains(compat, "avif") {
			return Type{Photo, "image/avif", ".avif"}
		}
		return Type{Photo, "image/heic", ".heic"}
	case "avif", "avis":
		return Type{Photo, "image/avif", ".avif"}
	case "M4A", "M4B", "M4P", "F4A":
		return Type{Audio, "audio/mp4", ".m4a"}
	case "qt":
		return Type{Video, "video/quicktime", ".mov"}
	case "3gp4", "3gp5", "3gp6", "3gs7", "3g2a", "3ge6":
		if ext == ".m4a" || ext == ".aac" || ext == ".amr" {
			return Type{Audio, "audio/mp4", ".m4a"}
		}
		return Type{Video, "video/3gpp", ".3gp"}
	}
	if ext == ".m4a" || ext == ".aac" {
		return Type{Audio, "audio/mp4", ".m4a"}
	}
	if ext == ".mov" {
		return Type{Video, "video/quicktime", ".mov"}
	}
	return Type{Video, "video/mp4", ".mp4"}
}

var documentTypes = map[string]Type{
	".pdf":     {Document, "application/pdf", ".pdf"},
	".doc":     {Document, "application/msword", ".doc"},
	".docx":    {Document, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".docx"},
	".xls":     {Document, "application/vnd.ms-excel", ".xls"},
	".xlsx":    {Document, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".xlsx"},
	".ppt":     {Document, "application/vnd.ms-powerpoint", ".ppt"},
	".pptx":    {Document, "application/vnd.openxmlformats-officedocument.presentationml.presentation", ".pptx"},
	".odt":     {Document, "application/vnd.oasis.opendocument.text", ".odt"},
	".ods":     {Document, "application/vnd.oasis.opendocument.spreadsheet", ".ods"},
	".odp":     {Document, "application/vnd.oasis.opendocument.presentation", ".odp"},
	".rtf":     {Document, "application/rtf", ".rtf"},
	".txt":     {Document, "text/plain", ".txt"},
	".csv":     {Document, "text/csv", ".csv"},
	".pages":   {Document, "application/vnd.apple.pages", ".pages"},
	".numbers": {Document, "application/vnd.apple.numbers", ".numbers"},
	".key":     {Document, "application/vnd.apple.keynote", ".key"},
}

var extTypes = map[string]Type{
	".jpg":  {Photo, "image/jpeg", ".jpg"},
	".jpeg": {Photo, "image/jpeg", ".jpg"},
	".png":  {Photo, "image/png", ".png"},
	".heic": {Photo, "image/heic", ".heic"},
	".mp4":  {Video, "video/mp4", ".mp4"},
	".mov":  {Video, "video/quicktime", ".mov"},
	".m4a":  {Audio, "audio/mp4", ".m4a"},
	".mp3":  {Audio, "audio/mpeg", ".mp3"},
	".opus": {Audio, "audio/ogg", ".opus"},
}

// BrowserImage reports whether browsers display this image type directly.
func BrowserImage(mime string) bool {
	switch mime {
	case "image/jpeg", "image/png", "image/gif", "image/webp", "image/avif", "image/bmp":
		return true
	}
	return false
}
