package meta

import (
	"context"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/rudyh0/afterfeed/internal/archive"
	"github.com/rudyh0/afterfeed/internal/importer"
	"github.com/rudyh0/afterfeed/internal/library"
	"github.com/rudyh0/afterfeed/internal/media"
)

type attachment struct {
	URI               string  `json:"uri"`
	CreationTimestamp float64 `json:"creation_timestamp"`
}

type dyiMessage struct {
	SenderName  string       `json:"sender_name"`
	TimestampMS float64      `json:"timestamp_ms"`
	Content     *string      `json:"content"`
	Photos      []attachment `json:"photos"`
	Videos      []attachment `json:"videos"`
	AudioFiles  []attachment `json:"audio_files"`
	Gifs        []attachment `json:"gifs"`
	Files       []attachment `json:"files"`
	Sticker     *attachment  `json:"sticker"`
	Share       *struct {
		Link      string `json:"link"`
		ShareText string `json:"share_text"`
	} `json:"share"`
	Reactions []struct {
		Reaction string `json:"reaction"`
		Actor    string `json:"actor"`
	} `json:"reactions"`
	IsUnsent     bool     `json:"is_unsent"`
	CallDuration *float64 `json:"call_duration"`
}

type dyiThread struct {
	Participants []struct {
		Name string `json:"name"`
	} `json:"participants"`
	Messages   []dyiMessage `json:"messages"`
	Title      string       `json:"title"`
	ThreadType string       `json:"thread_type"`
}

// e2eeThread is the format of the end-to-end encrypted Messenger download
// ("secure storage"): one JSON file per chat with a media folder next to it.
type e2eeThread struct {
	Participants []string `json:"participants"`
	ThreadName   string   `json:"threadName"`
	Messages     []struct {
		SenderName string  `json:"senderName"`
		Timestamp  float64 `json:"timestamp"`
		Text       string  `json:"text"`
		Type       string  `json:"type"`
		IsUnsent   bool    `json:"isUnsent"`
		Media      []struct {
			URI string `json:"uri"`
		} `json:"media"`
		Reactions []struct {
			Actor    string `json:"actor"`
			Reaction string `json:"reaction"`
		} `json:"reactions"`
	} `json:"messages"`
}

// ReadThreads reads the chats of a "Download your information" export.
// self is the account owner's name as it appears in chats, if known. It
// returns the name it used, which may have been worked out from the chats.
func ReadThreads(ctx context.Context, tree *archive.Tree, sink importer.Sink, self string) (string, error) {
	threads := ThreadFiles(tree)
	keys := make([]string, 0, len(threads))
	for k := range threads {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if self == "" {
		self = guessSelf(tree, threads)
	}
	for i, key := range keys {
		if err := ctx.Err(); err != nil {
			return self, err
		}
		sink.Progress(i, len(keys))
		files := threads[key]
		sort.Slice(files, func(a, b int) bool { return fileNumber(files[a].Name()) < fileNumber(files[b].Name()) })
		conv := &importer.Conversation{Key: "thread:" + key, Self: self, Source: files[0].Path}
		participants := map[string]bool{}
		for _, f := range files {
			var t dyiThread
			if err := archive.ReadJSON(f, &t); err != nil {
				sink.Problem(f.Path, err)
				continue
			}
			if conv.Title == "" {
				conv.Title = Text(t.Title)
			}
			for _, p := range t.Participants {
				name := Text(p.Name)
				if name != "" && !participants[name] {
					participants[name] = true
					conv.Participants = append(conv.Participants, name)
				}
			}
			if strings.Contains(strings.ToLower(t.ThreadType), "group") {
				conv.IsGroup = true
			}
			for _, m := range t.Messages {
				conv.Messages = append(conv.Messages, dyiToMessage(tree, &m, self))
			}
		}
		if len(conv.Messages) == 0 {
			continue
		}
		if len(conv.Participants) > 2 {
			conv.IsGroup = true
		}
		conv.MergeKey = library.MergeKey(conv.Participants)
		sortMessages(conv.Messages)
		sink.Conversation(conv)
	}
	return self, nil
}

func dyiToMessage(tree *archive.Tree, m *dyiMessage, self string) importer.Message {
	msg := importer.Message{
		At:     importer.Unix(m.TimestampMS),
		Sender: Text(m.SenderName),
		Kind:   importer.MessageText,
	}
	msg.Self = self != "" && msg.Sender == self
	if m.Content != nil {
		msg.Text = Text(*m.Content)
	}
	add := func(list []attachment, kind media.Kind, msgKind string) {
		for _, a := range list {
			if a.URI == "" || strings.Contains(a.URI, "://") {
				continue
			}
			md := importer.Media{Ref: a.URI, File: tree.Resolve(a.URI), Kind: kind}
			msg.Files = append(msg.Files, md)
			msg.Kind = msgKind
		}
	}
	add(m.Photos, "", importer.MessageMedia)
	add(m.Videos, "", importer.MessageMedia)
	add(m.Gifs, "", importer.MessageMedia)
	add(m.AudioFiles, media.Audio, importer.MessageAudio)
	add(m.Files, media.Document, importer.MessageFile)
	if m.Sticker != nil && m.Sticker.URI != "" && !strings.Contains(m.Sticker.URI, "://") {
		msg.Files = append(msg.Files, importer.Media{Ref: m.Sticker.URI, File: tree.Resolve(m.Sticker.URI)})
		msg.Kind = importer.MessageSticker
	}
	if m.Share != nil && m.Share.Link != "" {
		msg.Link = m.Share.Link
		if msg.Kind == importer.MessageText {
			msg.Kind = importer.MessageShare
		}
		if msg.Text == "" {
			msg.Text = Text(m.Share.ShareText)
		}
	}
	if m.CallDuration != nil {
		msg.Kind = importer.MessageCall
	}
	if m.IsUnsent {
		msg.Kind = importer.MessageUnsent
	}
	for _, r := range m.Reactions {
		msg.Reactions = append(msg.Reactions, library.Reaction{Actor: Text(r.Actor), Reaction: Text(r.Reaction)})
	}
	// Files that are not in the export at all (Meta leaves out some) are
	// kept as references, so they are counted as missing.
	for i := range msg.Files {
		if msg.Files[i].File == nil {
			msg.Files[i].Name = path.Base(msg.Files[i].Ref)
		}
	}
	return msg
}

var (
	e2eeName     = regexp.MustCompile(`^(.+)_(\d+)\.json$`)
	threadNumber = regexp.MustCompile(`^(.*)_\d+$`)
)

// IsE2EECandidate reports whether a file may be a chat from the end-to-end
// encrypted Messenger download.
func IsE2EECandidate(f *archive.File) bool {
	if f.Ext() != ".json" || threadFile.MatchString(f.Path) {
		return false
	}
	name := f.Name()
	if strings.HasPrefix(name, "message_") || !e2eeName.MatchString(name) {
		return false
	}
	// Files deep inside the normal export are not chats.
	for _, seg := range strings.Split(f.Path, "/") {
		switch seg {
		case "your_facebook_activity", "your_activity_across_facebook", "personal_information",
			"logged_information", "security_and_login_information", "ads_information", "connections",
			"preferences", "apps_and_websites_off_of_facebook", "your_instagram_activity", "posts":
			return false
		}
	}
	return true
}

// ReadE2EE reads the chats of the end-to-end encrypted Messenger download.
// Several files can belong to one chat (Name_1.json, Name_2.json).
func ReadE2EE(ctx context.Context, tree *archive.Tree, sink importer.Sink, self string) error {
	type group struct {
		files []*archive.File
	}
	groups := map[string]*group{}
	var order []string
	for _, f := range tree.Files() {
		if !IsE2EECandidate(f) {
			continue
		}
		m := e2eeName.FindStringSubmatch(f.Name())
		key := path.Join(f.Dir(), m[1])
		g := groups[key]
		if g == nil {
			g = &group{}
			groups[key] = g
			order = append(order, key)
		}
		g.files = append(g.files, f)
	}
	sort.Strings(order)
	var threads []*e2eeLoaded
	for _, key := range order {
		if err := ctx.Err(); err != nil {
			return err
		}
		g := groups[key]
		sort.Slice(g.files, func(a, b int) bool { return fileNumber(g.files[a].Name()) < fileNumber(g.files[b].Name()) })
		var loaded e2eeLoaded
		for _, f := range g.files {
			var t e2eeThread
			if err := archive.ReadJSON(f, &t); err != nil {
				sink.Problem(f.Path, err)
				continue
			}
			if t.ThreadName == "" && len(t.Participants) == 0 {
				continue // some other JSON file
			}
			loaded.parts = append(loaded.parts, e2eePart{file: f, t: t})
		}
		if len(loaded.parts) > 0 {
			threads = append(threads, &loaded)
		}
	}
	if self == "" {
		counts := map[string]int{}
		for _, t := range threads {
			seen := map[string]bool{}
			for _, p := range t.parts {
				for _, name := range p.t.Participants {
					name = Text(name)
					if !seen[name] {
						seen[name] = true
						counts[name]++
					}
				}
			}
		}
		self = mostCommon(counts, len(threads))
	}
	for i, t := range threads {
		sink.Progress(i, len(threads))
		first := t.parts[0]
		title := Text(first.t.ThreadName)
		if m := threadNumber.FindStringSubmatch(title); m != nil {
			title = m[1]
		}
		conv := &importer.Conversation{
			Key:    "e2ee:" + strings.ToLower(Text(first.t.ThreadName)),
			Title:  title,
			Self:   self,
			Source: first.file.Path,
		}
		seen := map[string]bool{}
		for _, part := range t.parts {
			for _, p := range part.t.Participants {
				name := Text(p)
				if name != "" && !seen[name] {
					seen[name] = true
					conv.Participants = append(conv.Participants, name)
				}
			}
			for _, m := range part.t.Messages {
				msg := importer.Message{
					At:     importer.Unix(m.Timestamp),
					Sender: Text(m.SenderName),
					Text:   Text(m.Text),
					Kind:   importer.MessageText,
				}
				msg.Self = self != "" && msg.Sender == self
				for _, md := range m.Media {
					if md.URI == "" {
						continue
					}
					msg.Files = append(msg.Files, importer.Media{Ref: md.URI, File: tree.ResolveFrom(part.file.Dir(), md.URI)})
					msg.Kind = importer.MessageMedia
				}
				for _, r := range m.Reactions {
					msg.Reactions = append(msg.Reactions, library.Reaction{Actor: Text(r.Actor), Reaction: Text(r.Reaction)})
				}
				if m.IsUnsent {
					msg.Kind = importer.MessageUnsent
				}
				conv.Messages = append(conv.Messages, msg)
			}
		}
		if len(conv.Messages) == 0 {
			continue
		}
		conv.IsGroup = len(conv.Participants) > 2
		conv.MergeKey = library.MergeKey(conv.Participants)
		sortMessages(conv.Messages)
		sink.Conversation(conv)
	}
	return nil
}

type e2eePart struct {
	file *archive.File
	t    e2eeThread
}

type e2eeLoaded struct {
	parts []e2eePart
}

// guessSelf finds the account owner: the participant who is in the most
// chats. This works in every language, unlike reading labels.
func guessSelf(tree *archive.Tree, threads map[string][]*archive.File) string {
	counts := map[string]int{}
	n := 0
	for _, files := range threads {
		var t struct {
			Participants []struct {
				Name string `json:"name"`
			} `json:"participants"`
		}
		// The first file of a thread is enough to know who is in it.
		f := files[0]
		for _, c := range files {
			if fileNumber(c.Name()) < fileNumber(f.Name()) {
				f = c
			}
		}
		if archive.ReadJSON(f, &t) != nil {
			continue
		}
		n++
		for _, p := range t.Participants {
			counts[Text(p.Name)]++
		}
	}
	return mostCommon(counts, n)
}

// mostCommon returns the name that is in every chat, or in clearly the
// most chats when there are several.
func mostCommon(counts map[string]int, total int) string {
	best, bestN, second := "", 0, 0
	for name, n := range counts {
		if name == "" {
			continue
		}
		if n > bestN || (n == bestN && name < best) {
			second = bestN
			best, bestN = name, n
		} else if n > second {
			second = n
		}
	}
	if total >= 2 && bestN > second {
		return best
	}
	return ""
}

func fileNumber(name string) int {
	i := strings.LastIndexByte(name, '_')
	j := strings.LastIndexByte(name, '.')
	if i < 0 || j <= i {
		return 0
	}
	n, _ := strconv.Atoi(name[i+1 : j])
	return n
}

func sortMessages(msgs []importer.Message) {
	sort.SliceStable(msgs, func(i, j int) bool { return msgs[i].At.Before(msgs[j].At) })
}

// ProfileName reads the account owner's name from a profile file, when the
// export has one.
func ProfileName(tree *archive.Tree, names ...string) string {
	for _, n := range names {
		for _, f := range tree.ByName(n) {
			var v any
			if archive.ReadJSON(f, &v) != nil {
				continue
			}
			if name := findFullName(v, 0); name != "" {
				return Text(name)
			}
		}
	}
	return ""
}

func findFullName(v any, depth int) string {
	if depth > 6 {
		return ""
	}
	switch x := v.(type) {
	case map[string]any:
		if s, ok := x["full_name"].(string); ok && s != "" {
			return s
		}
		for _, e := range x {
			if s := findFullName(e, depth+1); s != "" {
				return s
			}
		}
	case []any:
		for _, e := range x {
			if s := findFullName(e, depth+1); s != "" {
				return s
			}
		}
	}
	return ""
}
