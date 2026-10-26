// Package snapchat imports the Snapchat "My Data" export: Memories with
// their overlays, dates and locations, and saved chats.
package snapchat

import (
	"context"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rudyh0/afterfeed/internal/archive"
	"github.com/rudyh0/afterfeed/internal/importer"
	"github.com/rudyh0/afterfeed/internal/library"
	"github.com/rudyh0/afterfeed/internal/media"
)

// Importer reads Snapchat exports.
type Importer struct{}

// New returns the Snapchat importer.
func New() *Importer { return &Importer{} }

// Platform implements importer.Importer.
func (*Importer) Platform() string { return "snapchat" }

// Detect implements importer.Importer.
func (*Importer) Detect(names []string, fileName string) float64 {
	score := 0.0
	if strings.HasPrefix(strings.ToLower(fileName), "mydata") {
		score = 0.7
	}
	for _, n := range names {
		lower := strings.ToLower(n)
		switch {
		case strings.HasSuffix(lower, "json/memories_history.json"), strings.HasSuffix(lower, "json/chat_history.json"),
			strings.HasSuffix(lower, "json/snap_history.json"):
			return 0.95
		case memoryFile.MatchString(path.Base(n)) && strings.Contains(lower, "memories/"):
			score = max(score, 0.9)
		case strings.Contains(lower, "chat_media/"):
			score = max(score, 0.85)
		}
	}
	return score
}

// Memory files are named like 2019-06-01_<id>-main.jpg and
// 2019-06-01_<id>-overlay.png.
var (
	memoryFile = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})_(.+?)(?:-(main|overlay))?\.[A-Za-z0-9]+$`)
	uuidRe     = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	chatFile   = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})_(.+)\.[A-Za-z0-9]+$`)
	numbers    = regexp.MustCompile(`-?\d+(?:\.\d+)?`)
)

// ErrNoMemories is reported when the export lists Memories but does not
// contain them, because "Export your Memories" was not selected.
var ErrNoMemories = errors.New("the export lists Memories but does not contain them; request the export again with Export your Memories turned on")

// Read implements importer.Importer.
func (im *Importer) Read(ctx context.Context, tree *archive.Tree, sink importer.Sink) error {
	if err := readMemories(tree, sink); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return readChats(ctx, tree, sink)
}

type entry struct {
	at       time.Time
	kind     media.Kind
	lat, lon *float64
	ids      []string
	used     bool
	source   string
	// dup numbers entries that are otherwise the same, such as photos taken
	// in the same second in an export without ids.
	dup int
}

func (e *entry) key() string {
	ids := append([]string(nil), e.ids...)
	sort.Strings(ids)
	k := "memory:" + importer.Hash(e.at.Format(time.RFC3339), string(e.kind), strings.Join(ids, ","))
	if e.dup > 0 {
		k += fmt.Sprintf("#%d", e.dup+1)
	}
	return k
}

type matched struct {
	g *memGroup
	e *entry
}

// pairDay pairs the files of one day with the entries of that day that have
// no file yet. When the zip file times are the times the memories were
// taken, each file gets the nearest entry within a few minutes. Otherwise,
// files and entries are paired in order, but only when their numbers agree.
func pairDay(es []*entry, gs []*memGroup) ([]matched, []*memGroup) {
	var free []*entry
	for _, e := range es {
		if !e.used {
			free = append(free, e)
		}
	}
	if len(free) == 0 || len(gs) == 0 {
		return nil, gs
	}
	sort.SliceStable(free, func(i, j int) bool { return free[i].at.Before(free[j].at) })
	var out []matched
	rest := gs
	if timed(gs) {
		const window = 5 * time.Minute
		rest = nil
		for _, g := range gs {
			var best *entry
			var bestDiff time.Duration
			for _, e := range free {
				if e.used {
					continue
				}
				d := g.main.Modified.Sub(e.at)
				if d < 0 {
					d = -d
				}
				if d <= window && (best == nil || d < bestDiff) {
					best, bestDiff = e, d
				}
			}
			if best != nil {
				best.used = true
				out = append(out, matched{g, best})
			} else {
				rest = append(rest, g)
			}
		}
		var still []*entry
		for _, e := range free {
			if !e.used {
				still = append(still, e)
			}
		}
		free = still
	}
	if len(rest) > 0 && len(rest) == len(free) {
		sort.SliceStable(rest, func(i, j int) bool { return groupTime(rest[i]).Before(groupTime(rest[j])) })
		for i, g := range rest {
			free[i].used = true
			out = append(out, matched{g, free[i]})
		}
		rest = nil
	}
	return out, rest
}

// timed reports whether the zip times of these files fall on the day in
// their names and differ from each other, so they can be the times the
// memories were taken rather than the time the export was made.
func timed(gs []*memGroup) bool {
	seen := map[int64]bool{}
	for _, g := range gs {
		m := g.main.Modified
		if m.IsZero() || m.UTC().Format("2006-01-02") != g.date {
			return false
		}
		seen[m.Unix()] = true
	}
	return len(gs) == 1 || len(seen) > 1
}

// groupTime orders files of one day: by zip time when it is on that day,
// otherwise by name.
func groupTime(g *memGroup) time.Time {
	if m := g.main.Modified; !m.IsZero() && m.UTC().Format("2006-01-02") == g.date {
		return m
	}
	return time.Time{}
}

type memGroup struct {
	base    string
	date    string
	id      string
	main    *archive.File
	overlay *archive.File
}

func readMemories(tree *archive.Tree, sink importer.Sink) error {
	var entries []*entry
	for _, f := range tree.Match(func(f *archive.File) bool { return strings.EqualFold(f.Name(), "memories_history.json") }) {
		var v any
		if err := archive.ReadJSON(f, &v); err != nil {
			sink.Problem(f.Path, err)
			continue
		}
		for _, rec := range findRecords(v, 0) {
			e := &entry{source: f.Path}
			e.at = parseTime(rec["Date"])
			if e.at.IsZero() {
				continue
			}
			if s, ok := rec["Media Type"].(string); ok {
				if strings.Contains(strings.ToLower(s), "video") {
					e.kind = media.Video
				} else {
					e.kind = media.Photo
				}
			}
			if s, ok := rec["Location"].(string); ok {
				if nums := numbers.FindAllString(s, -1); len(nums) >= 2 {
					lat, _ := strconv.ParseFloat(nums[len(nums)-2], 64)
					lon, _ := strconv.ParseFloat(nums[len(nums)-1], 64)
					e.lat, e.lon = importer.Coordinates(lat, lon)
				}
			}
			seen := map[string]bool{}
			for _, val := range rec {
				if s, ok := val.(string); ok {
					for _, id := range uuidRe.FindAllString(s, -1) {
						id = strings.ToLower(id)
						if !seen[id] {
							seen[id] = true
							e.ids = append(e.ids, id)
						}
					}
				}
			}
			entries = append(entries, e)
		}
	}
	seenKeys := map[string]int{}
	for _, e := range entries {
		k := e.key()
		e.dup = seenKeys[k]
		seenKeys[k]++
	}

	// Group the media files with their overlays. Overlays can be in another
	// part than their photo, so groups use the name without folder.
	groups := map[string]*memGroup{}
	for _, f := range tree.Files() {
		if !strings.Contains(strings.ToLower(f.Path), "memories/") {
			continue
		}
		m := memoryFile.FindStringSubmatch(f.Name())
		if m == nil {
			continue
		}
		base := strings.ToLower(m[1] + "_" + m[2])
		g := groups[base]
		if g == nil {
			g = &memGroup{base: base, date: m[1], id: strings.ToLower(m[2])}
			groups[base] = g
		}
		if m[3] == "overlay" {
			g.overlay = f
		} else if t := media.Sniff(nil, f.Name()); t.Kind == media.Photo || t.Kind == media.Video || m[3] == "main" {
			g.main = f
		}
	}
	if len(groups) == 0 && len(entries) > 0 {
		sink.Problem(entries[0].source, ErrNoMemories)
		return nil
	}

	byID := map[string]*entry{}
	for _, e := range entries {
		for _, id := range e.ids {
			byID[id] = e
		}
	}
	bases := make([]string, 0, len(groups))
	for b := range groups {
		bases = append(bases, b)
	}
	sort.Strings(bases)

	var out []matched
	var unmatched []*memGroup
	for _, b := range bases {
		g := groups[b]
		if g.main == nil {
			continue // an overlay without its photo
		}
		var e *entry
		for _, id := range uuidRe.FindAllString(g.id, -1) {
			if c := byID[strings.ToLower(id)]; c != nil && !c.used {
				e = c
				break
			}
		}
		if e != nil {
			e.used = true
			out = append(out, matched{g, e})
		} else {
			unmatched = append(unmatched, g)
		}
	}
	// Files without a matching id are paired with the remaining entries of
	// the same day: by time when the files carry it, otherwise in order but
	// only when the numbers agree, so a pairing is never a guess.
	type dayKind struct {
		day  string
		kind media.Kind
	}
	pool := map[dayKind][]*entry{}
	for _, e := range entries {
		if !e.used {
			k := dayKind{e.at.Format("2006-01-02"), e.kind}
			pool[k] = append(pool[k], e)
		}
	}
	files := map[dayKind][]*memGroup{}
	for _, g := range unmatched {
		k := dayKind{g.date, media.Sniff(nil, g.main.Name()).Kind}
		files[k] = append(files[k], g)
	}
	var left []*memGroup
	for k, gs := range files {
		paired, rest := pairDay(pool[k], gs)
		for _, p := range paired {
			out = append(out, matched{p.g, p.e})
		}
		left = append(left, rest...)
	}
	// What is left of a day may differ in type between the list and the
	// files, such as a video saved as a photo: pair it per day only.
	byDay := map[string][]*memGroup{}
	for _, g := range left {
		byDay[g.date] = append(byDay[g.date], g)
	}
	dayPool := map[string][]*entry{}
	for _, e := range entries {
		if !e.used {
			d := e.at.Format("2006-01-02")
			dayPool[d] = append(dayPool[d], e)
		}
	}
	for d, gs := range byDay {
		paired, rest := pairDay(dayPool[d], gs)
		for _, p := range paired {
			out = append(out, matched{p.g, p.e})
		}
		for _, g := range rest {
			out = append(out, matched{g, nil})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].g.base < out[j].g.base })

	for _, mt := range out {
		g := mt.g
		mem := &importer.Memory{Kind: importer.KindSnap, Source: g.main.Path}
		md := importer.Media{File: g.main, Ref: g.main.Path, Overlay: g.overlay}
		if mt.e != nil {
			mem.Key = mt.e.key()
			mem.TakenAt = mt.e.at
			mem.Source = mt.e.source
			if mt.e.lat != nil {
				mem.Place = &library.Place{Lat: mt.e.lat, Lon: mt.e.lon}
			}
		} else {
			mem.Key = "file:" + g.base
			mem.TakenAt = fileTime(g)
		}
		mem.Media = []importer.Media{md}
		sink.Memory(mem)
	}
	// Memories listed in the JSON whose file is not in these parts.
	for _, e := range entries {
		if e.used {
			continue
		}
		mem := &importer.Memory{Key: e.key(), Kind: importer.KindSnap, TakenAt: e.at, Source: e.source}
		if e.lat != nil {
			mem.Place = &library.Place{Lat: e.lat, Lon: e.lon}
		}
		ref := "memories/" + e.at.Format("2006-01-02")
		if len(e.ids) > 0 {
			ref += "_" + e.ids[len(e.ids)-1]
		}
		mem.Media = []importer.Media{{Ref: ref}}
		sink.Memory(mem)
	}
	return nil
}

// fileTime is the best time for a memory without a JSON entry: the time in
// the zip file when it is on the date in the name, otherwise noon on that date.
func fileTime(g *memGroup) time.Time {
	day, err := time.Parse("2006-01-02", g.date)
	if err != nil {
		return g.main.Modified
	}
	if m := g.main.Modified; !m.IsZero() && m.UTC().Format("2006-01-02") == g.date {
		return m.UTC()
	}
	return day.Add(12 * time.Hour)
}

// findRecords returns the first array of objects that have a "Date" field,
// whatever the key around it is called.
func findRecords(v any, depth int) []map[string]any {
	if depth > 4 {
		return nil
	}
	switch x := v.(type) {
	case []any:
		var out []map[string]any
		for _, e := range x {
			if m, ok := e.(map[string]any); ok {
				if _, ok := m["Date"]; ok {
					out = append(out, m)
				}
			}
		}
		if len(out) > 0 {
			return out
		}
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var all []map[string]any
		for _, k := range keys {
			all = append(all, findRecords(x[k], depth+1)...)
		}
		return all
	}
	return nil
}

func parseTime(v any) time.Time {
	s, ok := v.(string)
	if !ok {
		return time.Time{}
	}
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "UTC"))
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05Z", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// --- chats ---

type chatRecord struct {
	From         string `json:"From"`
	To           string `json:"To"`
	MediaType    string `json:"Media Type"`
	Created      string `json:"Created"`
	CreatedMicro any    `json:"Created(microseconds)"`
	Content      any    `json:"Content"`
	Text         any    `json:"Text"`
	Title        any    `json:"Conversation Title"`
	IsSender     bool   `json:"IsSender"`
	MediaIDs     string `json:"Media IDs"`
}

func readChats(ctx context.Context, tree *archive.Tree, sink importer.Sink) error {
	chatMedia := map[string]*archive.File{}
	for _, f := range tree.Files() {
		if path.Base(f.Dir()) != "chat_media" {
			continue
		}
		if m := chatFile.FindStringSubmatch(f.Name()); m != nil {
			chatMedia[m[2]] = f
		}
	}
	for _, f := range tree.Match(func(f *archive.File) bool { return strings.EqualFold(f.Name(), "chat_history.json") }) {
		var raw map[string][]chatRecord
		if err := archive.ReadJSON(f, &raw); err != nil {
			sink.Problem(f.Path, err)
			continue
		}
		convs := map[string][]chatRecord{}
		// Old exports have two lists, received and sent, instead of one list
		// per conversation.
		if len(raw) <= 2 && hasOldLists(raw) {
			for _, list := range raw {
				for _, r := range list {
					other := r.From
					if other == "" {
						other = r.To
						r.IsSender = true
					}
					convs[other] = append(convs[other], r)
				}
			}
		} else {
			convs = raw
		}
		self := ""
		for _, list := range convs {
			for _, r := range list {
				if r.IsSender && r.From != "" {
					self = r.From
					break
				}
			}
			if self != "" {
				break
			}
		}
		keys := make([]string, 0, len(convs))
		for k := range convs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, key := range keys {
			if err := ctx.Err(); err != nil {
				return err
			}
			sink.Progress(i, len(keys))
			conv := &importer.Conversation{Key: "chat:" + key, Self: self, Source: f.Path}
			names := map[string]bool{}
			for _, r := range convs[key] {
				if t, ok := r.Title.(string); ok && t != "" {
					conv.Title = t
				}
				sender := r.From
				if r.IsSender && sender == "" {
					sender = self
				}
				if sender != "" && !names[sender] {
					names[sender] = true
					conv.Participants = append(conv.Participants, sender)
				}
				msg := importer.Message{
					At:     chatTime(&r),
					Sender: sender,
					Self:   r.IsSender,
					Kind:   chatKind(r.MediaType),
					Text:   firstString(r.Content, r.Text),
				}
				if msg.At.IsZero() {
					continue
				}
				for _, id := range strings.Split(r.MediaIDs, "|") {
					id = strings.TrimSpace(id)
					if id == "" {
						continue
					}
					md := importer.Media{Ref: "chat_media/" + id, File: chatMedia[id]}
					if msg.Kind == importer.MessageAudio {
						md.Kind = media.Audio
					}
					if md.File == nil {
						// Snaps that were not saved are not in the export: not a missing part.
						md.Ref = ""
						continue
					}
					msg.Files = append(msg.Files, md)
					if msg.Kind == importer.MessageText {
						msg.Kind = importer.MessageMedia
					}
				}
				if msg.Text == "" && len(msg.Files) == 0 {
					continue
				}
				conv.Messages = append(conv.Messages, msg)
			}
			if len(conv.Messages) == 0 {
				continue
			}
			if self != "" && !names[self] {
				conv.Participants = append(conv.Participants, self)
			}
			conv.IsGroup = len(conv.Participants) > 2
			if conv.Title == "" {
				conv.Title = key
			}
			sort.SliceStable(conv.Messages, func(a, b int) bool { return conv.Messages[a].At.Before(conv.Messages[b].At) })
			sink.Conversation(conv)
		}
	}
	return nil
}

func hasOldLists(raw map[string][]chatRecord) bool {
	for k := range raw {
		if strings.Contains(k, "Saved Chat History") {
			return true
		}
	}
	return false
}

func chatTime(r *chatRecord) time.Time {
	if v := importer.ParseNumber(r.CreatedMicro); v > 0 {
		return importer.Unix(v)
	}
	return parseTime(r.Created)
}

func chatKind(t string) string {
	switch strings.ToUpper(strings.TrimSpace(t)) {
	case "MEDIA", "IMAGE", "VIDEO", "SNAP":
		return importer.MessageMedia
	case "NOTE", "AUDIO":
		return importer.MessageAudio
	case "STICKER":
		return importer.MessageSticker
	case "SHARE", "SHARESAVEDSTORY", "STORY":
		return importer.MessageShare
	}
	return importer.MessageText
}

func firstString(vals ...any) string {
	for _, v := range vals {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return importer.Clean(s)
		}
	}
	return ""
}
