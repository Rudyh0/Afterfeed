// Package facebook imports the Facebook "Download your information" export
// in JSON format, and the end-to-end encrypted Messenger download.
package facebook

import (
	"context"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rudyh0/afterfeed/internal/archive"
	"github.com/rudyh0/afterfeed/internal/importer"
	"github.com/rudyh0/afterfeed/internal/importer/meta"
	"github.com/rudyh0/afterfeed/internal/library"
)

// Importer reads Facebook exports.
type Importer struct{}

// New returns the Facebook importer.
func New() *Importer { return &Importer{} }

// Platform implements importer.Importer.
func (*Importer) Platform() string { return "facebook" }

// Detect implements importer.Importer.
func (*Importer) Detect(names []string, fileName string) float64 {
	lower := strings.ToLower(fileName)
	score := 0.0
	if strings.HasPrefix(lower, "facebook-") {
		score = 0.7
	}
	e2ee := 0
	media := false
	for _, n := range names {
		switch {
		case strings.Contains(n, "your_facebook_activity/"), strings.Contains(n, "your_activity_across_facebook/"):
			return 0.95
		case strings.Contains(n, "your_instagram_activity/"):
			return 0
		case strings.Contains(n, "photos_and_videos/"), strings.HasPrefix(n, "posts/your_posts"):
			score = max(score, 0.8)
		}
		if meta.IsE2EECandidate(&archive.File{Path: n}) {
			e2ee++
		}
		if strings.HasPrefix(n, "media/") || strings.Contains(n, "/media/") {
			media = true
		}
	}
	if e2ee > 0 && (media || lower == "messages.zip" || strings.HasPrefix(lower, "messages")) {
		score = max(score, 0.75)
	}
	return score
}

// Read implements importer.Importer.
func (im *Importer) Read(ctx context.Context, tree *archive.Tree, sink importer.Sink) error {
	r := &reader{tree: tree, sink: sink, seen: map[string]bool{}}
	r.readPosts(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	r.readStories()
	r.readCollections()
	if err := ctx.Err(); err != nil {
		return err
	}
	self, err := meta.ReadThreads(ctx, tree, sink, meta.ProfileName(tree, "profile_information.json"))
	if err != nil {
		return err
	}
	return meta.ReadE2EE(ctx, tree, sink, self)
}

type reader struct {
	tree *archive.Tree
	sink importer.Sink
	// seen holds the media files already used by a post, so albums do not
	// repeat them.
	seen map[string]bool
}

var postsFile = regexp.MustCompile(`(?:^|/)(?:your_posts__check_ins__photos_and_videos|your_posts)(?:_\d+)?\.json$`)

type post struct {
	Timestamp   float64 `json:"timestamp"`
	Attachments []struct {
		Data []attachmentData `json:"data"`
	} `json:"attachments"`
	Data []map[string]any `json:"data"`
}

type attachmentData struct {
	Media *meta.MediaItem `json:"media"`
	Place *struct {
		Name       string `json:"name"`
		Address    string `json:"address"`
		Coordinate *struct {
			Latitude  any `json:"latitude"`
			Longitude any `json:"longitude"`
		} `json:"coordinate"`
	} `json:"place"`
	ExternalContext *struct {
		URL string `json:"url"`
	} `json:"external_context"`
	LifeEvent *struct {
		Title string `json:"title"`
	} `json:"life_event"`
}

func (r *reader) readPosts(ctx context.Context) {
	files := r.tree.Match(func(f *archive.File) bool { return postsFile.MatchString(f.Path) })
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	for i, f := range files {
		if ctx.Err() != nil {
			return
		}
		r.sink.Progress(i, len(files)+1)
		var v any
		if err := archive.ReadJSON(f, &v); err != nil {
			r.sink.Problem(f.Path, err)
			continue
		}
		r.posts(v, f.Path, importer.KindPost)
	}
}

// posts reads every array of post-like objects in a decoded file.
func (r *reader) posts(v any, source, kind string) {
	meta.FindArrays(v, func(objs []map[string]any) {
		for _, o := range objs {
			if _, ok := o["timestamp"]; !ok {
				continue
			}
			if _, ok := o["attachments"]; !ok {
				if _, ok := o["data"]; !ok {
					continue
				}
			}
			var p post
			if err := meta.Decode(o, &p); err != nil {
				r.sink.Problem(source, err)
				continue
			}
			if m := r.post(&p, source, kind); m != nil {
				r.sink.Memory(m)
			}
		}
	})
}

func (r *reader) post(p *post, source, kind string) *importer.Memory {
	m := &importer.Memory{TakenAt: importer.Unix(p.Timestamp), Kind: kind, Source: source}
	if m.TakenAt.IsZero() {
		return nil
	}
	var texts []string
	for _, d := range p.Data {
		if s, ok := d["post"].(string); ok {
			if t := meta.Text(s); t != "" {
				texts = append(texts, t)
			}
		}
	}
	var lifeEvent string
	for _, a := range p.Attachments {
		for _, d := range a.Data {
			switch {
			case d.Media != nil:
				md := d.Media.Media(r.tree)
				if md.Ref == "" {
					continue
				}
				r.seen[d.Media.Key()] = true
				if md.Text != "" && len(texts) == 0 && len(m.Media) == 0 {
					// A photo posted with a caption: the caption is the post text.
					texts = append(texts, md.Text)
					md.Text = ""
				}
				m.Media = append(m.Media, md)
			case d.Place != nil && m.Place == nil:
				pl := &library.Place{Name: meta.Text(d.Place.Name), Address: meta.Text(d.Place.Address)}
				if c := d.Place.Coordinate; c != nil {
					pl.Lat, pl.Lon = importer.Coordinates(importer.ParseNumber(c.Latitude), importer.ParseNumber(c.Longitude))
				}
				if pl.Name != "" || pl.HasCoordinates() {
					m.Place = pl
				}
			case d.ExternalContext != nil && m.Link == "":
				m.Link = d.ExternalContext.URL
			case d.LifeEvent != nil:
				lifeEvent = meta.Text(d.LifeEvent.Title)
			}
		}
	}
	m.Text = strings.Join(texts, "\n\n")
	if m.Text == "" {
		m.Text = lifeEvent
	}
	if len(m.Media) == 0 && m.Text == "" && m.Link == "" && m.Place == nil {
		return nil
	}
	if kind == importer.KindPost && len(m.Media) == 0 && m.Text == "" && m.Link == "" && m.Place != nil {
		m.Kind = importer.KindCheckin
	}
	stamp := strconv.FormatInt(m.TakenAt.Unix(), 10)
	switch {
	case len(m.Media) > 0:
		m.Key = kind + ":" + stamp + ":" + mediaKey(m.Media[0])
	default:
		placeName := ""
		if m.Place != nil {
			placeName = m.Place.Name
		}
		m.Key = kind + ":" + stamp + ":" + importer.Hash(m.Text, m.Link, placeName)
	}
	return m
}

func mediaKey(md importer.Media) string {
	return strings.ToLower(path.Base(strings.ReplaceAll(md.Ref, "\\", "/")))
}

var storiesFile = regexp.MustCompile(`(?:^|/)(?:archived_stories|your_stories|stories)\.json$`)

func (r *reader) readStories() {
	for _, f := range r.tree.Match(func(f *archive.File) bool {
		return storiesFile.MatchString(f.Path) && !strings.Contains(f.Path, "your_instagram_activity")
	}) {
		var v any
		if err := archive.ReadJSON(f, &v); err != nil {
			r.sink.Problem(f.Path, err)
			continue
		}
		r.posts(v, f.Path, importer.KindStory)
	}
}

var collectionFile = regexp.MustCompile(`(?:^|/)(?:album/[^/]+|your_uncategorized_photos|your_videos|uncategorized_photos|videos)\.json$`)

// readCollections reads albums and the lists of uploaded photos and videos.
// Files already in a post are skipped. Photos uploaded together to the same
// album become one memory.
func (r *reader) readCollections() {
	files := r.tree.Match(func(f *archive.File) bool {
		return collectionFile.MatchString(f.Path) && !strings.Contains(f.Path, "your_instagram_activity") &&
			!strings.Contains(f.Path, "/messages/")
	})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	for _, f := range files {
		var v any
		if err := archive.ReadJSON(f, &v); err != nil {
			r.sink.Problem(f.Path, err)
			continue
		}
		album := ""
		if o, ok := v.(map[string]any); ok && strings.Contains(f.Path, "/album/") {
			if s, ok := o["name"].(string); ok {
				album = meta.Text(s)
			}
		}
		var items []meta.MediaItem
		meta.FindArrays(v, func(objs []map[string]any) {
			for _, o := range objs {
				if _, ok := o["uri"]; !ok {
					continue
				}
				var it meta.MediaItem
				if meta.Decode(o, &it) == nil && it.URI != "" {
					items = append(items, it)
				}
			}
		})
		r.collection(items, album, f.Path)
	}
}

func (r *reader) collection(items []meta.MediaItem, album, source string) {
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreationTimestamp < items[j].CreationTimestamp })
	var group []meta.MediaItem
	flush := func() {
		if len(group) == 0 {
			return
		}
		m := &importer.Memory{Kind: importer.KindPhoto, Album: album, Source: source}
		allVideo := true
		for _, it := range group {
			md := it.Media(r.tree)
			m.Media = append(m.Media, md)
			if !isVideo(it.URI) {
				allVideo = false
			}
		}
		if allVideo {
			m.Kind = importer.KindVideo
		}
		first := group[0]
		taken, _, _ := first.Exif()
		if taken.IsZero() {
			taken = first.Created()
		}
		m.TakenAt = taken
		if len(group) == 1 {
			m.Text = m.Media[0].Text
			m.Media[0].Text = ""
		}
		m.Key = "media:" + first.Key()
		r.sink.Memory(m)
		group = nil
	}
	for _, it := range items {
		key := it.Key()
		if r.seen[key] {
			continue
		}
		r.seen[key] = true
		if len(group) > 0 {
			prev := group[len(group)-1]
			gap := time.Duration(it.CreationTimestamp-prev.CreationTimestamp) * time.Second
			if album == "" || gap > 10*time.Minute || len(group) >= 50 {
				flush()
			}
		}
		group = append(group, it)
	}
	flush()
}

func isVideo(uri string) bool {
	switch strings.ToLower(path.Ext(uri)) {
	case ".mp4", ".mov", ".m4v", ".webm", ".3gp", ".avi":
		return true
	}
	return false
}
