// Package x imports the X (formerly Twitter) archive.
package x

import (
	"context"
	"html"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/rudyh0/afterfeed/internal/archive"
	"github.com/rudyh0/afterfeed/internal/importer"
	"github.com/rudyh0/afterfeed/internal/library"
	"github.com/rudyh0/afterfeed/internal/media"
)

// Importer reads X archives.
type Importer struct{}

// New returns the X importer.
func New() *Importer { return &Importer{} }

// Platform implements importer.Importer.
func (*Importer) Platform() string { return "x" }

var (
	tweetsFile = regexp.MustCompile(`(?:^|/)data/tweets?(?:-part\d+)?\.js$`)
	notesFile  = regexp.MustCompile(`(?:^|/)data/note-tweet(?:-part\d+)?\.js$`)
)

// Detect implements importer.Importer.
func (*Importer) Detect(names []string, fileName string) float64 {
	score := 0.0
	if strings.HasPrefix(strings.ToLower(fileName), "twitter-") {
		score = 0.7
	}
	for _, n := range names {
		switch {
		case strings.HasSuffix(n, "data/manifest.js"), tweetsFile.MatchString(n):
			return 0.95
		case strings.Contains(n, "data/tweets_media/"), strings.Contains(n, "data/direct_messages_media/"):
			score = max(score, 0.85)
		}
	}
	return score
}

type account struct {
	Account struct {
		AccountID   string `json:"accountId"`
		Username    string `json:"username"`
		DisplayName string `json:"accountDisplayName"`
	} `json:"account"`
}

type tweetMedia struct {
	URL           string `json:"url"`
	MediaURLHTTPS string `json:"media_url_https"`
	MediaURL      string `json:"media_url"`
	Type          string `json:"type"`
}

type tweet struct {
	IDStr               string `json:"id_str"`
	CreatedAt           string `json:"created_at"`
	FullText            string `json:"full_text"`
	InReplyToUserIDStr  string `json:"in_reply_to_user_id_str"`
	InReplyToScreenName string `json:"in_reply_to_screen_name"`
	Entities            struct {
		URLs []struct {
			URL         string `json:"url"`
			ExpandedURL string `json:"expanded_url"`
		} `json:"urls"`
		Media        []tweetMedia `json:"media"`
		UserMentions []struct {
			ScreenName string `json:"screen_name"`
			IDStr      string `json:"id_str"`
		} `json:"user_mentions"`
	} `json:"entities"`
	ExtendedEntities struct {
		Media []tweetMedia `json:"media"`
	} `json:"extended_entities"`
	Geo *struct {
		Coordinates []any `json:"coordinates"`
	} `json:"geo"`
	Coordinates *struct {
		Coordinates []any `json:"coordinates"`
	} `json:"coordinates"`
}

type noteTweet struct {
	NoteTweet struct {
		CreatedAt string `json:"createdAt"`
		Core      struct {
			Text string `json:"text"`
			URLs []struct {
				ShortURL    string `json:"shortUrl"`
				ExpandedURL string `json:"expandedUrl"`
			} `json:"urls"`
		} `json:"core"`
	} `json:"noteTweet"`
}

// Read implements importer.Importer.
func (im *Importer) Read(ctx context.Context, tree *archive.Tree, sink importer.Sink) error {
	r := &reader{tree: tree, sink: sink, handles: map[string]string{}}
	r.readAccount()
	r.indexMedia()
	if err := r.readTweets(ctx); err != nil {
		return err
	}
	return r.readDMs(ctx)
}

type reader struct {
	tree     *archive.Tree
	sink     importer.Sink
	selfID   string
	selfName string
	handles  map[string]string // user id → @handle
	media    map[string][]*archive.File
	dmMedia  map[string][]*archive.File
}

func (r *reader) readAccount() {
	for _, f := range r.tree.Match(func(f *archive.File) bool { return strings.HasSuffix(f.Path, "data/account.js") }) {
		var list []account
		if err := archive.ReadJS(f, &list); err != nil {
			r.sink.Problem(f.Path, err)
			continue
		}
		if len(list) > 0 {
			r.selfID = list[0].Account.AccountID
			if u := list[0].Account.Username; u != "" {
				r.selfName = "@" + u
				r.handles[r.selfID] = r.selfName
			}
		}
	}
}

// indexMedia groups media files by the id at the start of their name
// (the post id for posts, the message id for messages).
func (r *reader) indexMedia() {
	r.media = map[string][]*archive.File{}
	r.dmMedia = map[string][]*archive.File{}
	for _, f := range r.tree.Files() {
		dir := path.Base(f.Dir())
		id, _, ok := strings.Cut(f.Name(), "-")
		if !ok {
			continue
		}
		switch dir {
		case "tweets_media", "tweet_media":
			r.media[id] = append(r.media[id], f)
		case "direct_messages_media", "direct_messages_group_media":
			r.dmMedia[id] = append(r.dmMedia[id], f)
		}
	}
}

func (r *reader) readTweets(ctx context.Context) error {
	notes := r.readNotes()
	files := r.tree.Match(func(f *archive.File) bool { return tweetsFile.MatchString(f.Path) })
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	var tweets []tweet
	for _, f := range files {
		var list []struct {
			Tweet tweet `json:"tweet"`
		}
		if err := archive.ReadJS(f, &list); err != nil {
			r.sink.Problem(f.Path, err)
			continue
		}
		for _, t := range list {
			tweets = append(tweets, t.Tweet)
		}
	}
	// Learn handles first, for naming chats.
	for _, t := range tweets {
		for _, m := range t.Entities.UserMentions {
			if m.IDStr != "" && m.ScreenName != "" {
				r.handles[m.IDStr] = "@" + m.ScreenName
			}
		}
		if t.InReplyToUserIDStr != "" && t.InReplyToScreenName != "" {
			r.handles[t.InReplyToUserIDStr] = "@" + t.InReplyToScreenName
		}
	}
	for i, t := range tweets {
		if err := ctx.Err(); err != nil {
			return err
		}
		if i%500 == 0 {
			r.sink.Progress(i, len(tweets)*2)
		}
		if m := r.tweet(&t, notes); m != nil {
			r.sink.Memory(m)
		}
	}
	return nil
}

func (r *reader) readNotes() map[int64]noteTweet {
	out := map[int64]noteTweet{}
	for _, f := range r.tree.Match(func(f *archive.File) bool { return notesFile.MatchString(f.Path) }) {
		var list []noteTweet
		if err := archive.ReadJS(f, &list); err != nil {
			r.sink.Problem(f.Path, err)
			continue
		}
		for _, n := range list {
			if t, err := time.Parse(time.RFC3339Nano, n.NoteTweet.CreatedAt); err == nil {
				out[t.Unix()] = n
			}
		}
	}
	return out
}

const tweetTime = "Mon Jan 02 15:04:05 -0700 2006"

func (r *reader) tweet(t *tweet, notes map[int64]noteTweet) *importer.Memory {
	if strings.HasPrefix(t.FullText, "RT @") {
		return nil // a repost of someone else's post
	}
	at, err := time.Parse(tweetTime, t.CreatedAt)
	if err != nil || t.IDStr == "" {
		r.sink.Problem("data/tweets.js", err)
		return nil
	}
	text := t.FullText
	mediaList := t.ExtendedEntities.Media
	if len(mediaList) == 0 {
		mediaList = t.Entities.Media
	}
	for _, md := range mediaList {
		if md.URL != "" {
			text = strings.ReplaceAll(text, md.URL, "")
		}
	}
	for _, u := range t.Entities.URLs {
		if u.URL != "" && u.ExpandedURL != "" {
			text = strings.ReplaceAll(text, u.URL, u.ExpandedURL)
		}
	}
	text = importer.Clean(html.UnescapeString(text))

	// Long posts are cut off in tweets.js; the full text is in note-tweet.js.
	for _, d := range []int64{0, -1, 1, -2, 2} {
		n, ok := notes[at.Unix()+d]
		if !ok {
			continue
		}
		full := n.NoteTweet.Core.Text
		for _, u := range n.NoteTweet.Core.URLs {
			full = strings.ReplaceAll(full, u.ShortURL, u.ExpandedURL)
		}
		prefix := strings.TrimRight(strings.TrimSuffix(text, "…"), " ")
		if len([]rune(prefix)) > 40 {
			prefix = string([]rune(prefix)[:40])
		}
		if strings.HasPrefix(full, prefix) {
			text = importer.Clean(html.UnescapeString(full))
			break
		}
	}

	kind := importer.KindPost
	if t.InReplyToUserIDStr != "" && t.InReplyToUserIDStr != r.selfID {
		kind = importer.KindReply
	}
	m := &importer.Memory{
		Key:     "post:" + t.IDStr,
		Kind:    kind,
		TakenAt: at.UTC(),
		Text:    text,
		Link:    "",
		Source:  "data/tweets.js",
	}
	if lat, lon, ok := r.coordinates(t); ok {
		m.Place = &library.Place{Lat: &lat, Lon: &lon}
	}
	files := r.media[t.IDStr]
	used := map[*archive.File]bool{}
	for _, md := range mediaList {
		ref := t.IDStr + "-" + path.Base(md.MediaURLHTTPS)
		if md.MediaURLHTTPS == "" {
			ref = t.IDStr + "-" + path.Base(md.MediaURL)
		}
		var f *archive.File
		if md.Type == "photo" {
			for _, c := range files {
				if c.Name() == ref {
					f = c
				}
			}
		} else {
			// Videos and GIFs are stored under another name: take the next
			// video file of this post.
			for _, c := range files {
				if !used[c] && media.Sniff(nil, c.Name()).Kind == media.Video {
					f = c
					break
				}
			}
			if f == nil {
				ref = t.IDStr + "-" + strings.TrimSuffix(path.Base(md.MediaURLHTTPS), path.Ext(md.MediaURLHTTPS)) + ".mp4"
			}
		}
		if f != nil {
			used[f] = true
		}
		m.Media = append(m.Media, importer.Media{File: f, Ref: "data/tweets_media/" + ref})
	}
	if text == "" && len(m.Media) == 0 {
		return nil
	}
	return m
}

func (r *reader) coordinates(t *tweet) (float64, float64, bool) {
	if t.Geo != nil && len(t.Geo.Coordinates) == 2 {
		lat, lon := importer.ParseNumber(t.Geo.Coordinates[0]), importer.ParseNumber(t.Geo.Coordinates[1])
		if importer.ValidCoordinates(lat, lon) {
			return lat, lon, true
		}
	}
	if t.Coordinates != nil && len(t.Coordinates.Coordinates) == 2 {
		// GeoJSON order: longitude first.
		lon, lat := importer.ParseNumber(t.Coordinates.Coordinates[0]), importer.ParseNumber(t.Coordinates.Coordinates[1])
		if importer.ValidCoordinates(lat, lon) {
			return lat, lon, true
		}
	}
	return 0, 0, false
}

type dmEvent struct {
	MessageCreate *struct {
		ID          string   `json:"id"`
		SenderID    string   `json:"senderId"`
		RecipientID string   `json:"recipientId"`
		Text        string   `json:"text"`
		CreatedAt   string   `json:"createdAt"`
		MediaURLs   []string `json:"mediaUrls"`
		URLs        []struct {
			URL      string `json:"url"`
			Expanded string `json:"expanded"`
		} `json:"urls"`
		Reactions []struct {
			SenderID    string `json:"senderId"`
			ReactionKey string `json:"reactionKey"`
			Emoji       string `json:"emoji"`
		} `json:"reactions"`
	} `json:"messageCreate"`
	ConversationNameUpdate *struct {
		Name string `json:"name"`
	} `json:"conversationNameUpdate"`
	JoinConversation *struct {
		ParticipantsSnapshot []string `json:"participantsSnapshot"`
	} `json:"joinConversation"`
	ParticipantsJoin *struct {
		UserIDs []string `json:"userIds"`
	} `json:"participantsJoin"`
}

type dmConversation struct {
	DMConversation struct {
		ConversationID string    `json:"conversationId"`
		Messages       []dmEvent `json:"messages"`
	} `json:"dmConversation"`
}

var dmFile = regexp.MustCompile(`(?:^|/)data/direct-messages(?:-group)?(?:-part\d+)?\.js$`)

var reactionEmoji = map[string]string{
	"like": "❤️", "funny": "😂", "surprised": "😲", "sad": "😢", "excited": "🔥",
	"agree": "👍", "disagree": "👎",
}

func (r *reader) readDMs(ctx context.Context) error {
	files := r.tree.Match(func(f *archive.File) bool { return dmFile.MatchString(f.Path) })
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	convs := map[string]*importer.Conversation{}
	var order []string
	for _, f := range files {
		var list []dmConversation
		if err := archive.ReadJS(f, &list); err != nil {
			r.sink.Problem(f.Path, err)
			continue
		}
		group := strings.Contains(f.Name(), "group")
		for _, c := range list {
			id := c.DMConversation.ConversationID
			if id == "" {
				continue
			}
			conv := convs[id]
			if conv == nil {
				conv = &importer.Conversation{Key: "dm:" + id, IsGroup: group, Self: r.selfName, Source: f.Path}
				convs[id] = conv
				order = append(order, id)
			}
			ids := map[string]bool{}
			if !group {
				for _, p := range strings.Split(id, "-") {
					ids[p] = true
				}
			}
			for _, e := range c.DMConversation.Messages {
				switch {
				case e.MessageCreate != nil:
					mc := e.MessageCreate
					ids[mc.SenderID] = true
					at, err := time.Parse(time.RFC3339Nano, mc.CreatedAt)
					if err != nil {
						continue
					}
					text := mc.Text
					for _, u := range mc.URLs {
						if strings.Contains(u.Expanded, "/messages/media/") {
							text = strings.ReplaceAll(text, u.URL, "")
						} else if u.URL != "" && u.Expanded != "" {
							text = strings.ReplaceAll(text, u.URL, u.Expanded)
						}
					}
					msg := importer.Message{
						Key:    "dm:" + mc.ID,
						At:     at.UTC(),
						Sender: r.name(mc.SenderID),
						Self:   mc.SenderID == r.selfID,
						Kind:   importer.MessageText,
						Text:   importer.Clean(html.UnescapeString(text)),
					}
					for _, f := range r.dmMedia[mc.ID] {
						msg.Files = append(msg.Files, importer.Media{File: f, Ref: f.Path})
						msg.Kind = importer.MessageMedia
					}
					if len(msg.Files) == 0 {
						for _, u := range mc.MediaURLs {
							msg.Files = append(msg.Files, importer.Media{Ref: "data/direct_messages_media/" + mc.ID + "-" + path.Base(u)})
							msg.Kind = importer.MessageMedia
						}
					}
					for _, re := range mc.Reactions {
						emoji := re.Emoji
						if emoji == "" {
							emoji = reactionEmoji[re.ReactionKey]
						}
						if emoji == "" {
							emoji = re.ReactionKey
						}
						msg.Reactions = append(msg.Reactions, library.Reaction{Actor: r.name(re.SenderID), Reaction: emoji})
					}
					conv.Messages = append(conv.Messages, msg)
				case e.ConversationNameUpdate != nil && e.ConversationNameUpdate.Name != "":
					conv.Title = e.ConversationNameUpdate.Name
				case e.JoinConversation != nil:
					for _, p := range e.JoinConversation.ParticipantsSnapshot {
						ids[p] = true
					}
				case e.ParticipantsJoin != nil:
					for _, p := range e.ParticipantsJoin.UserIDs {
						ids[p] = true
					}
				}
			}
			for p := range ids {
				name := r.name(p)
				found := false
				for _, e := range conv.Participants {
					if e == name {
						found = true
					}
				}
				if !found {
					conv.Participants = append(conv.Participants, name)
				}
			}
		}
	}
	for i, id := range order {
		if err := ctx.Err(); err != nil {
			return err
		}
		r.sink.Progress(len(order)+i, len(order)*2)
		conv := convs[id]
		if len(conv.Messages) == 0 {
			continue
		}
		sort.Strings(conv.Participants)
		sort.SliceStable(conv.Messages, func(a, b int) bool { return conv.Messages[a].At.Before(conv.Messages[b].At) })
		if conv.Title == "" {
			var others []string
			for _, p := range conv.Participants {
				if p != r.selfName {
					others = append(others, p)
				}
			}
			conv.Title = strings.Join(others, ", ")
		}
		r.sink.Conversation(conv)
	}
	return nil
}

func (r *reader) name(id string) string {
	if h, ok := r.handles[id]; ok {
		return h
	}
	return id
}
