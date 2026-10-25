package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/rudyh0/afterfeed/internal/store"
)

// Conversation is the content of conversation.json.
type Conversation struct {
	Version  int    `json:"version"`
	ID       string `json:"id"`
	Platform string `json:"platform"`
	// Key identifies the conversation on its platform.
	Key string `json:"key"`
	// MergeKey groups conversations from different exports of the same chat,
	// such as the normal and the end-to-end encrypted Messenger export.
	MergeKey     string    `json:"merge_key,omitempty"`
	Title        string    `json:"title"`
	Participants []string  `json:"participants"`
	IsGroup      bool      `json:"is_group,omitempty"`
	Self         string    `json:"self,omitempty"`
	Vault        bool      `json:"vault,omitempty"`
	Messages     []Message `json:"messages"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Message is one message in a conversation.
type Message struct {
	Key string `json:"key"`
	// At is the time the message was sent, in milliseconds since 1970.
	At        int64         `json:"at"`
	Sender    string        `json:"sender,omitempty"`
	Self      bool          `json:"self,omitempty"`
	Kind      string        `json:"kind"`
	Text      string        `json:"text,omitempty"`
	Link      string        `json:"link,omitempty"`
	Files     []MessageFile `json:"files,omitempty"`
	Reactions []Reaction    `json:"reactions,omitempty"`
}

// MessageFile is a file sent in a message.
type MessageFile struct {
	ID string `json:"id"`
	// File is the path relative to the conversation folder.
	File     string  `json:"file"`
	Kind     string  `json:"kind"`
	Name     string  `json:"name,omitempty"`
	Mime     string  `json:"mime,omitempty"`
	Size     int64   `json:"size,omitempty"`
	Width    int     `json:"width,omitempty"`
	Height   int     `json:"height,omitempty"`
	Duration float64 `json:"duration,omitempty"`
	Source   string  `json:"source,omitempty"`
}

// Reaction is an emoji reaction to a message.
type Reaction struct {
	Actor    string `json:"actor,omitempty"`
	Reaction string `json:"reaction"`
}

// ConversationID returns the identifier of the conversation with this key.
func ConversationID(key string) string { return HashID("conversation", key) }

// MessageFileID returns the identifier of a file in a message.
func MessageFileID(conversationID, messageKey, fileKey string) string {
	return HashID("message-file", conversationID, messageKey, fileKey)
}

// ConversationDir returns the absolute path of a conversation folder.
func (l *Library) ConversationDir(rel string) string {
	return filepath.Join(l.messages, filepath.FromSlash(rel))
}

// CreateConversationDir creates a folder for a conversation and returns its
// path relative to the messages folder.
func (l *Library) CreateConversationDir(c *Conversation) (string, error) {
	rel := c.Platform + "/" + Slug(c.Title, 40) + "-" + c.ID[:8]
	if err := os.MkdirAll(filepath.Join(l.ConversationDir(rel), "files"), 0o755); err != nil {
		return "", err
	}
	return rel, nil
}

// SaveConversation writes conversation.json atomically, with the messages in order.
func (l *Library) SaveConversation(rel string, c *Conversation) error {
	c.Version = FormatVersion
	sort.SliceStable(c.Messages, func(i, j int) bool { return c.Messages[i].At < c.Messages[j].At })
	return writeJSONFile(filepath.Join(l.ConversationDir(rel), "conversation.json"), c)
}

// LoadConversation reads conversation.json.
func (l *Library) LoadConversation(rel string) (*Conversation, error) {
	var c Conversation
	if err := readJSONFile(filepath.Join(l.ConversationDir(rel), "conversation.json"), &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// FindConversation returns the folder of the conversation with this key, or
// of a conversation on the same platform with the same merge key.
func (l *Library) FindConversation(ctx context.Context, platform, key, mergeKey string) (rel string, err error) {
	err = l.st.R.QueryRowContext(ctx, `SELECT path FROM conversations WHERE source_key = ?`, key).Scan(&rel)
	if err == nil || !errors.Is(err, sql.ErrNoRows) || mergeKey == "" {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return rel, err
	}
	rows, err := l.st.R.QueryContext(ctx, `SELECT path FROM conversations WHERE platform = ? AND merge_key = ? LIMIT 2`, platform, mergeKey)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var found []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return "", err
		}
		found = append(found, p)
	}
	// Only merge when it is unambiguous.
	if len(found) == 1 {
		return found[0], rows.Err()
	}
	return "", rows.Err()
}

// IndexConversation adds or updates a conversation in the index. Messages
// that are already indexed are left alone.
func (l *Library) IndexConversation(ctx context.Context, tx *sql.Tx, rel string, c *Conversation) error {
	var first, last any
	lastText := ""
	files := 0
	if n := len(c.Messages); n > 0 {
		first, last = c.Messages[0].At, c.Messages[n-1].At
		for i := n - 1; i >= 0 && i >= n-50; i-- {
			if t := strings.TrimSpace(c.Messages[i].Text); t != "" {
				lastText = truncate(t, 200)
				break
			}
		}
	}
	for _, m := range c.Messages {
		files += len(m.Files)
	}
	participants, _ := json.Marshal(c.Participants)
	now := store.Now()
	_, err := tx.ExecContext(ctx, `INSERT INTO conversations (
			id, source_key, merge_key, platform, title, is_group, participants_json, self_name,
			first_at, last_at, last_text, message_count, file_count, vault, path, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			merge_key = excluded.merge_key, title = excluded.title, is_group = excluded.is_group,
			participants_json = excluded.participants_json, self_name = excluded.self_name,
			first_at = excluded.first_at, last_at = excluded.last_at, last_text = excluded.last_text,
			message_count = excluded.message_count, file_count = excluded.file_count,
			vault = excluded.vault, path = excluded.path, updated_at = excluded.updated_at`,
		c.ID, c.Key, c.MergeKey, c.Platform, c.Title, boolInt(c.IsGroup), string(participants), c.Self,
		first, last, lastText, len(c.Messages), files, boolInt(c.Vault), rel, store.FormatTime(c.CreatedAt), now)
	if err != nil {
		return fmt.Errorf("index conversation %s: %w", c.ID, err)
	}
	msgStmt, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO messages
		(conversation_id, source_key, sender, is_self, sent_at, kind, text, link, files_json, reactions_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer msgStmt.Close()
	fileStmt, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO message_files
		(id, conversation_id, message_key, sent_at, kind, file, name, mime, size, width, height, duration)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer fileStmt.Close()
	for _, m := range c.Messages {
		filesJSON := "[]"
		if len(m.Files) > 0 {
			b, _ := json.Marshal(m.Files)
			filesJSON = string(b)
		}
		reactions := "[]"
		if len(m.Reactions) > 0 {
			b, _ := json.Marshal(m.Reactions)
			reactions = string(b)
		}
		if _, err := msgStmt.ExecContext(ctx, c.ID, m.Key, m.Sender, boolInt(m.Self), m.At, m.Kind, m.Text, m.Link, filesJSON, reactions); err != nil {
			return fmt.Errorf("index message: %w", err)
		}
		for _, f := range m.Files {
			if _, err := fileStmt.ExecContext(ctx, f.ID, c.ID, m.Key, m.At, f.Kind, f.File, f.Name, f.Mime, f.Size, f.Width, f.Height, f.Duration); err != nil {
				return fmt.Errorf("index message file: %w", err)
			}
		}
	}
	return nil
}

// MergeKey returns a key that is the same for conversations with the same
// participants, regardless of order, case or accents.
func MergeKey(participants []string) string {
	names := make([]string, 0, len(participants))
	for _, p := range participants {
		p = strings.ToLower(strings.Join(strings.Fields(p), " "))
		if p != "" {
			names = append(names, p)
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	return HashID(names...)
}

// Slug turns a title into a safe folder name.
func Slug(s string, maxLen int) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if a, ok := asciiFold[r]; ok {
			b.WriteString(a)
			dash = false
			continue
		}
		switch {
		case r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
		if b.Len() >= maxLen {
			break
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "chat"
	}
	return out
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// asciiFold maps common accented letters to plain ASCII for folder names.
var asciiFold = func() map[rune]string {
	m := map[rune]string{}
	groups := map[string]string{
		"a": "àáâãäåāăą", "c": "çćĉċč", "d": "ďđ", "e": "èéêëēĕėęě", "g": "ĝğġģ", "h": "ĥħ",
		"i": "ìíîïĩīĭįı", "j": "ĵ", "k": "ķ", "l": "ĺļľŀł", "n": "ñńņňŉ", "o": "òóôõöøōŏő",
		"r": "ŕŗř", "s": "śŝşšș", "t": "ţťŧț", "u": "ùúûüũūŭůűų", "w": "ŵ", "y": "ýÿŷ", "z": "źżž",
		"ae": "æ", "oe": "œ", "ss": "ß", "th": "þ",
	}
	for ascii, letters := range groups {
		for _, r := range letters {
			m[r] = ascii
		}
	}
	return m
}()
