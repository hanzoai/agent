package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/hanzoai/orm"
)

// A thread lists and opens for the member who opened it; a thread from before
// users were kept stays the org's.
func TestConversationsAreTheMembers(t *testing.T) {
	ctx := context.Background()
	s := newStore(t.TempDir())
	defer s.closeAll()
	const org = "acme"
	alice, err := s.loadOrCreateConversation(ctx, org, "alice", "", "alice asks")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.loadOrCreateConversation(ctx, org, "bob", "", "bob asks"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.loadOrCreateConversation(ctx, org, "", "", "before users were kept"); err != nil {
		t.Fatal(err)
	}
	mine, err := s.listConversations(ctx, org, "bob")
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) != 2 {
		t.Fatalf("bob lists %d; want his own and the org's", len(mine))
	}
	for _, cv := range mine {
		if cv.Id() == alice.Id() {
			t.Fatalf("bob lists alice's thread %q", cv.Title)
		}
	}
	if _, err := s.conversationMessages(ctx, org, "bob", alice.Id()); !errors.Is(err, orm.ErrNotFound) {
		t.Fatalf("bob reads alice's thread: err=%v", err)
	}
	if _, err := s.loadOrCreateConversation(ctx, org, "bob", alice.Id(), "x"); !errors.Is(err, orm.ErrNotFound) {
		t.Fatalf("bob appends to alice's thread: err=%v", err)
	}
	if _, err := s.conversationMessages(ctx, org, "alice", alice.Id()); err != nil {
		t.Fatalf("alice reads her own: %v", err)
	}
}
