package server

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/rudyh0/afterfeed/internal/library"
)

// addConversation stores a conversation with n messages, one minute apart.
func addConversation(t *testing.T, lib *library.Library, key string, n int) string {
	t.Helper()
	start := time.Date(2020, 3, 1, 9, 0, 0, 0, time.UTC)
	c := &library.Conversation{
		Version: 1, ID: library.ConversationID(key), Platform: "facebook", Key: key,
		Title: "Sanne", Participants: []string{"Rudy", "Sanne"}, Self: "Rudy",
		CreatedAt: start, UpdatedAt: start,
	}
	for i := range n {
		sender := "Sanne"
		if i%2 == 1 {
			sender = "Rudy"
		}
		c.Messages = append(c.Messages, library.Message{
			Key: fmt.Sprintf("%s:%d", key, i), At: start.Add(time.Duration(i) * time.Minute).UnixMilli(),
			Sender: sender, Self: sender == "Rudy", Kind: "text", Text: fmt.Sprintf("bericht nummer %d", i),
		})
	}
	c.Messages[n/2].Text = "zullen we naar Texel gaan"
	rel, err := lib.CreateConversationDir(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := lib.SaveConversation(rel, c); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tx, err := lib.Store().W.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := lib.IndexConversation(ctx, tx, rel, c); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return c.ID
}

type page struct {
	Conversation conversationJSON `json:"conversation"`
	Messages     []messageJSON    `json:"messages"`
	MoreBefore   bool             `json:"more_before"`
	MoreAfter    bool             `json:"more_after"`
}

func TestConversationPages(t *testing.T) {
	c, deps := newTestServer(t)
	c.do("POST", "/api/setup", map[string]string{"username": "rudy", "password": "a long enough password"}, nil)
	id := addConversation(t, deps.Library, "fb:sanne", 250)

	var list struct {
		Platforms     []map[string]any    `json:"platforms"`
		Conversations []*conversationJSON `json:"conversations"`
	}
	if code := c.do("GET", "/api/conversations?platform=facebook", nil, &list); code != 200 || len(list.Conversations) != 1 {
		t.Fatalf("list: %d %+v", code, list)
	}
	if list.Conversations[0].Messages != 250 {
		t.Fatalf("message count %d", list.Conversations[0].Messages)
	}

	// The newest page ends with the last message.
	var p page
	c.do("GET", "/api/conversations/"+id+"?limit=100", nil, &p)
	if len(p.Messages) != 100 || !p.MoreBefore || p.MoreAfter || p.Messages[99].Text != "bericht nummer 249" {
		t.Fatalf("newest page: %d %v %v", len(p.Messages), p.MoreBefore, p.MoreAfter)
	}
	// Earlier pages continue without gaps or repeats.
	first := p.Messages[0]
	var earlier page
	c.do("GET", fmt.Sprintf("/api/conversations/%s?limit=100&before=%d&before_id=%d", id, first.At, first.ID), nil, &earlier)
	if len(earlier.Messages) != 100 || earlier.Messages[99].Text != "bericht nummer 149" || !earlier.MoreBefore {
		t.Fatalf("earlier page: %d %q", len(earlier.Messages), earlier.Messages[99].Text)
	}

	// Search finds one message; around shows it with context on both sides.
	var hits struct {
		Messages []messageHit `json:"messages"`
	}
	c.do("GET", "/api/conversations/"+id+"/search?q=texel", nil, &hits)
	if len(hits.Messages) != 1 {
		t.Fatalf("search: %+v", hits)
	}
	var around page
	c.do("GET", fmt.Sprintf("/api/conversations/%s?limit=20&around=%d", id, hits.Messages[0].ID), nil, &around)
	if len(around.Messages) != 20 || around.Messages[10].Text != "zullen we naar Texel gaan" || !around.MoreBefore || !around.MoreAfter {
		t.Fatalf("around: %d %q", len(around.Messages), around.Messages[10].Text)
	}
	last := around.Messages[19]
	var later page
	c.do("GET", fmt.Sprintf("/api/conversations/%s?limit=20&after=%d&after_id=%d", id, last.At, last.ID), nil, &later)
	if later.Messages[0].Text != "bericht nummer 135" {
		t.Fatalf("later: %q", later.Messages[0].Text)
	}

	// Go to a moment: the first message from then on.
	from := time.Date(2020, 3, 1, 10, 0, 0, 0, time.UTC).UnixMilli()
	var at page
	c.do("GET", fmt.Sprintf("/api/conversations/%s?limit=5&from=%d", id, from), nil, &at)
	if at.Messages[0].Text != "bericht nummer 60" || !at.MoreBefore {
		t.Fatalf("from: %q", at.Messages[0].Text)
	}
}

func TestVaultConversation(t *testing.T) {
	c, deps := newTestServer(t)
	c.do("POST", "/api/setup", map[string]string{"username": "rudy", "password": "a long enough password"}, nil)
	id := addConversation(t, deps.Library, "fb:sanne", 10)

	var status map[string]any
	c.do("GET", "/api/vault", nil, &status)
	if status["pin_set"] != false || status["open"] != false {
		t.Fatalf("initial status %v", status)
	}
	if code := c.do("POST", "/api/vault/pin", map[string]string{"pin": "12ab56"}, nil); code != 400 {
		t.Fatalf("invalid pin: %d", code)
	}
	if code := c.do("POST", "/api/vault/pin", map[string]string{"pin": "135790"}, &status); code != 200 || status["open"] != true {
		t.Fatalf("set pin: %d %v", code, status)
	}
	if code := c.do("PATCH", "/api/conversations/"+id, map[string]any{"vault": true}, nil); code != 200 {
		t.Fatalf("move to vault: %d", code)
	}
	c.do("POST", "/api/vault/lock", nil, nil)

	// A locked vault hides the conversation everywhere.
	var list struct {
		Conversations []*conversationJSON `json:"conversations"`
	}
	c.do("GET", "/api/conversations", nil, &list)
	if len(list.Conversations) != 0 {
		t.Fatal("conversation in the vault is listed")
	}
	if code := c.do("GET", "/api/conversations/"+id, nil, nil); code != 404 {
		t.Fatalf("locked conversation: %d", code)
	}
	if code := c.do("GET", "/api/conversations?vault=1", nil, nil); code != 403 {
		t.Fatalf("locked vault list: %d", code)
	}
	if code := c.do("POST", "/api/vault/unlock", map[string]string{"pin": "000000"}, nil); code != 400 {
		t.Fatalf("wrong pin: %d", code)
	}
	if code := c.do("POST", "/api/vault/unlock", map[string]string{"pin": "135790"}, nil); code != 200 {
		t.Fatalf("unlock: %d", code)
	}
	c.do("GET", "/api/conversations?vault=1", nil, &list)
	if len(list.Conversations) != 1 || !list.Conversations[0].Vault {
		t.Fatalf("vault list: %+v", list)
	}

	// The vault flag is kept in conversation.json, so a rebuild keeps it.
	var rel string
	deps.Library.Store().R.QueryRow(`SELECT path FROM conversations WHERE id = ?`, id).Scan(&rel)
	conv, err := deps.Library.LoadConversation(rel)
	if err != nil || !conv.Vault {
		t.Fatalf("conversation.json not updated: %v", err)
	}

	// Changing the PIN needs the current one; a forgotten PIN is reset with the password.
	if code := c.do("POST", "/api/vault/pin/change", map[string]string{"current": "111111", "pin": "246802"}, nil); code != 400 {
		t.Fatalf("change with wrong pin: %d", code)
	}
	if code := c.do("POST", "/api/vault/pin/change", map[string]string{"current": "135790", "pin": "246802"}, nil); code != 204 {
		t.Fatalf("change pin: %d", code)
	}
	if code := c.do("POST", "/api/vault/reset", map[string]string{"password": "wrong password!!"}, nil); code != 400 {
		t.Fatalf("reset with wrong password: %d", code)
	}
	if code := c.do("POST", "/api/vault/reset", map[string]string{"password": "a long enough password"}, &status); code != 200 || status["pin_set"] != false || status["open"] != false {
		t.Fatalf("reset: %d %v", code, status)
	}
}
