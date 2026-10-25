// Package instagram imports the Instagram "Download your information"
// export in JSON format.
package instagram

import (
	"context"
	"regexp"
	"sort"
	"strings"

	"github.com/rudyh0/afterfeed/internal/archive"
	"github.com/rudyh0/afterfeed/internal/importer"
	"github.com/rudyh0/afterfeed/internal/importer/meta"
)

// Importer reads Instagram exports.
type Importer struct{}

// New returns the Instagram importer.
func New() *Importer { return &Importer{} }

// Platform implements importer.Importer.
func (*Importer) Platform() string { return "instagram" }

// Detect implements importer.Importer.
func (*Importer) Detect(names []string, fileName string) float64 {
	score := 0.0
	if strings.HasPrefix(strings.ToLower(fileName), "instagram-") {
		score = 0.7
	}
	mediaPosts, contentPosts := false, false
	for _, n := range names {
		switch {
		case strings.Contains(n, "your_instagram_activity/"):
			return 0.95
		case strings.Contains(n, "your_facebook_activity/"):
			return 0
		case strings.HasPrefix(n, "media/posts/") || strings.Contains(n, "/media/posts/"):
			mediaPosts = true
		case strings.HasSuffix(n, "content/posts_1.json"), strings.HasSuffix(n, "content/stories.json"):
			contentPosts = true
		}
	}
	if mediaPosts && contentPosts {
		score = max(score, 0.85)
	} else if mediaPosts || contentPosts {
		score = max(score, 0.6)
	}
	return score
}

var (
	postsFile    = regexp.MustCompile(`(?:^|/)(?:media|content)/(?:posts_\d+|archived_posts)\.json$`)
	storiesFile  = regexp.MustCompile(`(?:^|/)(?:media|content)/stories\.json$`)
	reelsFile    = regexp.MustCompile(`(?:^|/)(?:media|content)/reels\.json$`)
	igtvFile     = regexp.MustCompile(`(?:^|/)(?:media|content)/igtv_videos\.json$`)
	profileFile  = regexp.MustCompile(`(?:^|/)(?:media|content)/profile_photos\.json$`)
	otherContent = regexp.MustCompile(`(?:^|/)(?:media|content)/other_content\.json$`)
)

// Read implements importer.Importer.
func (im *Importer) Read(ctx context.Context, tree *archive.Tree, sink importer.Sink) error {
	r := &reader{tree: tree, sink: sink, seen: map[string]bool{}}
	steps := []struct {
		re   *regexp.Regexp
		kind string
	}{
		{postsFile, importer.KindPost},
		{reelsFile, importer.KindReel},
		{igtvFile, importer.KindVideo},
		{storiesFile, importer.KindStory},
		{profileFile, importer.KindPhoto},
		{otherContent, importer.KindPhoto},
	}
	for i, step := range steps {
		if err := ctx.Err(); err != nil {
			return err
		}
		sink.Progress(i, len(steps)+1)
		files := tree.Match(func(f *archive.File) bool { return step.re.MatchString(f.Path) })
		sort.Slice(files, func(a, b int) bool { return files[a].Path < files[b].Path })
		for _, f := range files {
			var v any
			if err := archive.ReadJSON(f, &v); err != nil {
				sink.Problem(f.Path, err)
				continue
			}
			r.read(v, f.Path, step.kind)
		}
	}
	_, err := meta.ReadThreads(ctx, tree, sink, "")
	return err
}

type reader struct {
	tree *archive.Tree
	sink importer.Sink
	seen map[string]bool
}

type igPost struct {
	Media             []meta.MediaItem `json:"media"`
	Title             string           `json:"title"`
	CreationTimestamp float64          `json:"creation_timestamp"`
}

// read handles both shapes Instagram uses: posts with a media list, and
// lists of single media items (stories, profile photos).
func (r *reader) read(v any, source, kind string) {
	meta.FindArrays(v, func(objs []map[string]any) {
		for _, o := range objs {
			switch {
			case o["media"] != nil:
				var p igPost
				if err := meta.Decode(o, &p); err != nil {
					r.sink.Problem(source, err)
					continue
				}
				r.post(&p, source, kind)
			case o["uri"] != nil:
				var it meta.MediaItem
				if err := meta.Decode(o, &it); err != nil {
					r.sink.Problem(source, err)
					continue
				}
				r.post(&igPost{Media: []meta.MediaItem{it}}, source, kind)
			}
		}
	})
}

func (r *reader) post(p *igPost, source, kind string) {
	if len(p.Media) == 0 {
		return
	}
	first := p.Media[0]
	key := first.Key()
	if key == "" || r.seen[key] {
		return
	}
	r.seen[key] = true
	// Posts with one photo have the date and caption on the photo, posts
	// with several on the post.
	ts := p.CreationTimestamp
	if ts == 0 {
		for _, it := range p.Media {
			if it.CreationTimestamp > 0 && (ts == 0 || it.CreationTimestamp < ts) {
				ts = it.CreationTimestamp
			}
		}
	}
	text := meta.Text(p.Title)
	if text == "" {
		text = meta.Text(first.Title)
	}
	m := &importer.Memory{
		Key:     kind + ":" + key,
		Kind:    kind,
		TakenAt: importer.Unix(ts),
		Text:    text,
		Source:  source,
	}
	for i, it := range p.Media {
		md := it.Media(r.tree)
		if md.Ref == "" {
			continue
		}
		// On a single photo the title is the caption, already used above.
		if t := meta.Text(it.Title); t != "" && t != text && i > 0 {
			md.Text = t
		}
		if md.Text == text {
			md.Text = ""
		}
		m.Media = append(m.Media, md)
		r.seen[it.Key()] = true
	}
	if len(m.Media) == 0 {
		return
	}
	r.sink.Memory(m)
}
