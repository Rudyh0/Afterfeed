// Package importer runs imports: it opens the parts of an export, lets the
// platform importer read it, checks that every referenced file is there,
// and stores memories and conversations in the library.
//
// Platform importers live in sub-packages and only implement Importer. They
// never write files themselves.
package importer

import (
	"context"
	"time"

	"github.com/rudyh0/afterfeed/internal/archive"
	"github.com/rudyh0/afterfeed/internal/library"
	"github.com/rudyh0/afterfeed/internal/media"
)

// Importer reads the export of one platform.
type Importer interface {
	// Platform returns the platform name, such as "facebook".
	Platform() string
	// Detect returns how likely it is (0 to 1) that a zip file or folder with
	// these file paths belongs to this platform. fileName is the name of the
	// zip file or folder.
	Detect(names []string, fileName string) float64
	// Read walks the export and reports every memory and conversation to
	// the sink. Read must report the same items every time it reads the same
	// export.
	Read(ctx context.Context, tree *archive.Tree, sink Sink) error
}

// Sink receives what an importer reads.
type Sink interface {
	Memory(*Memory)
	Conversation(*Conversation)
	// Problem reports a file that could not be read. The import continues.
	Problem(path string, err error)
	// Progress reports how far reading is, in any unit.
	Progress(done, total int)
}

// Memory is a memory as read from an export.
type Memory struct {
	// Key identifies the item on the platform. It must stay the same when the
	// same export, or a newer export of the same account, is read again.
	Key     string
	Kind    string
	TakenAt time.Time
	TZ      string
	Text    string
	Link    string
	Album   string
	Place   *library.Place
	Media   []Media
	// Source is the path of the file in the export the memory came from.
	Source string
}

// Media is a file referenced by a memory or message.
type Media struct {
	// File is the file in the export, or nil when it is missing.
	File *archive.File
	// Ref is the reference as written in the export, used to report missing files.
	Ref string
	// Key identifies the file across exports. It defaults to the lower-case file name.
	Key     string
	Overlay *archive.File
	// Kind overrides the detected kind, for example audio for voice messages
	// stored as mp4.
	Kind    media.Kind
	TakenAt time.Time
	Lat     *float64
	Lon     *float64
	Text    string
	// Name is the original file name shown for documents.
	Name string
}

// Missing reports whether the export refers to a file that is not there.
func (m *Media) Missing() bool { return m.File == nil && m.Ref != "" }

// Conversation is a conversation as read from an export, with all its messages.
type Conversation struct {
	Key          string
	MergeKey     string
	Title        string
	Participants []string
	IsGroup      bool
	Self         string
	Messages     []Message
	Source       string
}

// Message is one message.
type Message struct {
	// Key identifies the message. When empty it is derived from the time,
	// sender and content.
	Key       string
	At        time.Time
	Sender    string
	Self      bool
	Kind      string
	Text      string
	Link      string
	Files     []Media
	Reactions []library.Reaction
}

// Message kinds.
const (
	MessageText    = "text"
	MessageMedia   = "media"
	MessageAudio   = "audio"
	MessageFile    = "file"
	MessageSticker = "sticker"
	MessageShare   = "share"
	MessageCall    = "call"
	MessageUnsent  = "unsent"
)

// Memory kinds.
const (
	KindPost    = "post"
	KindPhoto   = "photo"
	KindVideo   = "video"
	KindCheckin = "checkin"
	KindStory   = "story"
	KindReel    = "reel"
	KindSnap    = "snap"
	KindReply   = "reply"
	KindOwn     = "memory"
)
