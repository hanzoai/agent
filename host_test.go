package agent

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// A host that hands agent its databases (Deps.Open, Deps.Shares) is the only place
// conversations and shares are kept: nothing is written under DataDir, and agent
// never closes a handle it borrowed.
func TestHostDatabasesHoldTheConversations(t *testing.T) {
	ctx := context.Background()
	host, dataDir := t.TempDir(), t.TempDir()
	byName := map[string]*sql.DB{}
	open := func(name string) (*sql.DB, error) {
		if db, ok := byName[name]; ok {
			return db, nil
		}
		db, err := sql.Open("sqlite", filepath.Join(host, name+".db"))
		if err != nil {
			return nil, err
		}
		db.SetMaxOpenConns(1)
		byName[name] = db
		return db, nil
	}
	shares := func() (*sql.DB, error) { return open("shares") }
	s := newStore(dataDir, open, shares)
	cv, err := s.loadOrCreateConversation(ctx, "acme", "alice", "", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.appendMessage(ctx, "acme", cv.Id(), "user", "hi", "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.index(); err != nil {
		t.Fatal(err)
	}
	if err := s.closeAll(); err != nil {
		t.Fatal(err)
	}
	if err := byName["acme"].PingContext(ctx); err != nil {
		t.Fatalf("agent closed the host's database: %v", err)
	}
	s = newStore(dataDir, open, shares)
	defer s.closeAll()
	_, msgs, err := s.conversationMessages(ctx, "acme", "alice", false, cv.Id())
	if err != nil || len(msgs) != 1 || msgs[0].Content != "hi" {
		t.Fatalf("after reopening: %v, %d messages", err, len(msgs))
	}
	if m, _ := filepath.Glob(filepath.Join(dataDir, "*")); len(m) != 0 {
		t.Fatalf("agent wrote under its DataDir: %v", m)
	}
}
