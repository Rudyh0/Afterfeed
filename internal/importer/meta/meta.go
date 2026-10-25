// Package meta holds what the Facebook and Instagram importers share: the
// way Meta exports describe media, and the format of Messenger and
// Instagram chats.
package meta

import (
	"encoding/json"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/rudyh0/afterfeed/internal/archive"
	"github.com/rudyh0/afterfeed/internal/importer"
)

// Text repairs and trims text from a Meta export.
func Text(s string) string { return importer.Clean(importer.FixMojibake(s)) }

// MediaItem is how Meta exports describe a photo or video.
type MediaItem struct {
	URI               string         `json:"uri"`
	CreationTimestamp float64        `json:"creation_timestamp"`
	Title             string         `json:"title"`
	Description       string         `json:"description"`
	MediaMetadata     *MediaMetadata `json:"media_metadata"`
}

// MediaMetadata holds the EXIF data Meta keeps from the original file.
type MediaMetadata struct {
	PhotoMetadata *struct {
		ExifData []ExifData `json:"exif_data"`
	} `json:"photo_metadata"`
	VideoMetadata *struct {
		ExifData []ExifData `json:"exif_data"`
	} `json:"video_metadata"`
}

// ExifData is one block of EXIF values. Fields can be numbers or strings.
type ExifData struct {
	TakenTimestamp   any `json:"taken_timestamp"`
	DateTimeOriginal any `json:"date_time_original"`
	Latitude         any `json:"latitude"`
	Longitude        any `json:"longitude"`
}

// Exif returns the capture time and location from the metadata, when known.
func (it *MediaItem) Exif() (taken time.Time, lat, lon *float64) {
	if it.MediaMetadata == nil {
		return
	}
	var blocks []ExifData
	if pm := it.MediaMetadata.PhotoMetadata; pm != nil {
		blocks = append(blocks, pm.ExifData...)
	}
	if vm := it.MediaMetadata.VideoMetadata; vm != nil {
		blocks = append(blocks, vm.ExifData...)
	}
	for _, b := range blocks {
		if taken.IsZero() {
			if ts := importer.ParseNumber(b.TakenTimestamp); ts > 0 {
				taken = importer.Unix(ts)
			} else if s, ok := b.DateTimeOriginal.(string); ok {
				if t, err := time.Parse("2006:01:02 15:04:05", s); err == nil && t.Year() > 1900 {
					taken = t
				}
			}
		}
		if lat == nil && b.Latitude != nil && b.Longitude != nil {
			lat, lon = importer.Coordinates(importer.ParseNumber(b.Latitude), importer.ParseNumber(b.Longitude))
		}
	}
	return
}

// Media resolves the item in the tree.
func (it *MediaItem) Media(tree *archive.Tree) importer.Media {
	md := importer.Media{Text: Text(it.Description)}
	// A URI with a scheme links to a website, not to a file in the export.
	if it.URI != "" && !strings.Contains(it.URI, "://") {
		md.Ref = it.URI
		md.File = tree.Resolve(it.URI)
	}
	md.TakenAt, md.Lat, md.Lon = it.Exif()
	return md
}

// Created returns the upload time.
func (it *MediaItem) Created() time.Time { return importer.Unix(it.CreationTimestamp) }

// Key returns the lower-case file name, which is Meta's id of the file.
func (it *MediaItem) Key() string {
	return strings.ToLower(path.Base(strings.ReplaceAll(it.URI, "\\", "/")))
}

// FindArrays walks a decoded JSON value and calls fn for every array of
// objects. Meta changes the names of top-level keys between export
// versions, so importers look for the shape of the data instead.
func FindArrays(v any, fn func([]map[string]any)) {
	switch x := v.(type) {
	case []any:
		objs := make([]map[string]any, 0, len(x))
		for _, e := range x {
			if m, ok := e.(map[string]any); ok {
				objs = append(objs, m)
			}
		}
		if len(objs) > 0 {
			fn(objs)
		}
		for _, e := range x {
			if _, ok := e.(map[string]any); !ok {
				FindArrays(e, fn)
			}
		}
	case map[string]any:
		for _, e := range x {
			FindArrays(e, fn)
		}
	}
}

// Decode converts a generic JSON value into a typed struct.
func Decode(v any, out any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

var threadFile = regexp.MustCompile(`(?:^|/)messages/(inbox|archived_threads|filtered_threads|message_requests|e2ee_cutover)/([^/]+)/message_\d+\.json$`)

// ThreadFiles returns the chat files of a "Download your information"
// export, grouped by conversation folder.
func ThreadFiles(tree *archive.Tree) map[string][]*archive.File {
	out := map[string][]*archive.File{}
	for _, f := range tree.Files() {
		m := threadFile.FindStringSubmatch(f.Path)
		if m == nil {
			continue
		}
		out[m[2]] = append(out[m[2]], f)
	}
	return out
}
