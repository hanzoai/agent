package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/hanzoai/orm"
)

// legacy writes a conversation as the store kept one before users were: a row
// naming no member, with one turn. It returns the conversation's id.
func legacy(t *testing.T, dir, org, title, turn string) string {
	t.Helper()
	s := newStore(dir)
	defer s.closeAll()
	db, err := s.dbFor(org)
	if err != nil {
		t.Fatal(err)
	}
	cv := orm.New[Conversation](db)
	cv.SetId(newID())
	cv.Org, cv.Title = org, title
	if err := cv.CreateCtx(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.appendMessage(context.Background(), org, cv.Id(), "user", turn, "", nil); err != nil {
		t.Fatal(err)
	}
	return cv.Id()
}

// has reports whether a list holds the conversation id.
func has(list []convSummary, id string) bool {
	for _, c := range list {
		if c.ID == id {
			return true
		}
	}
	return false
}

type thread struct {
	ConversationID string `json:"conversationId"`
	Messages       []struct {
		Content string `json:"content"`
	} `json:"messages"`
}

// A conversation is the person's who opened it. Another member of the same org
// lists none of it, opens it as if it were not there (404, exactly as an id that
// never existed), and a turn they send under its id opens a fresh thread of
// their own instead of landing in it. Nothing about it moves.
func TestAnotherMembersConversationIsNotThere(t *testing.T) {
	app, _, _ := shareAppWith(t, t.TempDir(), &stubCompleter{resp: completion("bob's answer")})
	conv := recordTurns(t, app, "acme", "alice", "", inMessage{"user", "alice's question"}, inMessage{"assistant", "alice's answer"})

	if got := listed(t, app, bob, false); len(got) != 0 {
		t.Fatalf("bob lists %d conversations; want none", len(got))
	}
	foreign, foreignBody := call(t, app, http.MethodGet, "/v1/agent/conversations/"+conv, bob, nil)
	absent, absentBody := call(t, app, http.MethodGet, "/v1/agent/conversations/0000000000000000000-000000", bob, nil)
	if foreign != http.StatusNotFound || absent != http.StatusNotFound || string(foreignBody) != string(absentBody) {
		t.Fatalf("bob opens alice's: %d %s; an absent id: %d %s", foreign, foreignBody, absent, absentBody)
	}
	// The record endpoint and the round each take a turn under alice's id.
	if got := recordTurns(t, app, "acme", "bob", conv, inMessage{"user", "bob writes"}); got == conv || got == "" {
		t.Fatalf("bob's recorded turn landed in %q (alice's is %q)", got, conv)
	}
	status, raw := call(t, app, http.MethodPost, "/v1/agent", bob, map[string]any{"preset": "graph", "conversationId": conv, "messages": []inMessage{{"user", "bob asks"}}})
	var run struct {
		ConversationID string `json:"conversationId"`
	}
	_ = json.Unmarshal(raw, &run)
	if status != http.StatusOK || run.ConversationID == conv || run.ConversationID == "" {
		t.Fatalf("bob's round: %d, landed in %q (alice's is %q)", status, run.ConversationID, conv)
	}
	for _, w := range []who{bob, boss} {
		if status, _, _ := patch(t, app, w, conv, map[string]any{"title": "taken"}); status != http.StatusNotFound {
			t.Fatalf("%s renames alice's: %d", w.user, status)
		}
		if status, _ := call(t, app, http.MethodDelete, "/v1/agent/conversations/"+conv, w, nil); status != http.StatusNotFound {
			t.Fatalf("%s deletes alice's: %d", w.user, status)
		}
		if status, _ := call(t, app, http.MethodPost, "/v1/agent/conversations/"+conv+"/shares", w, nil); status != http.StatusNotFound {
			t.Fatalf("%s shares alice's: %d", w.user, status)
		}
	}
	status, raw = call(t, app, http.MethodGet, "/v1/agent/conversations/"+conv, alice, nil)
	var tr thread
	_ = json.Unmarshal(raw, &tr)
	if status != http.StatusOK || len(tr.Messages) != 2 || tr.Messages[1].Content != "alice's answer" {
		t.Fatalf("alice's own thread moved: %d %s", status, raw)
	}
	if got := listed(t, app, alice, false); len(got) != 1 || !has(got, conv) {
		t.Fatalf("alice lists %v; want her one", ids(got))
	}
}

// A conversation recorded before users were kept names nobody, so it is
// nobody's: no member, and no admin of the org, lists it, opens it or writes into
// it. A SuperAdmin reads it — lists and opens — and still cannot write, change
// or share it. (Regression: every member of a shared org listed, read and wrote
// into every such thread.)
func TestAConversationWithNoMemberIsNobodys(t *testing.T) {
	dir := t.TempDir()
	old := legacy(t, dir, "acme", "With Antje", "a stranger's words")
	app, trail := shareApp(t, dir)
	super := who{org: "acme", user: "root", name: "Root", super: true}

	for _, w := range []who{alice, bob, boss} {
		if has(listed(t, app, w, false), old) {
			t.Fatalf("%s lists the memberless thread", w.user)
		}
		if status, _ := call(t, app, http.MethodGet, "/v1/agent/conversations/"+old, w, nil); status != http.StatusNotFound {
			t.Fatalf("%s opens the memberless thread: %d", w.user, status)
		}
		if got := recordTurns(t, app, w.org, w.user, old, inMessage{"user", "written by " + w.user}); got == old {
			t.Fatalf("%s wrote into the memberless thread", w.user)
		}
		if status, _ := call(t, app, http.MethodGet, "/v1/agent/conversations?memberless=true", w, nil); status != http.StatusForbidden {
			t.Fatalf("%s lists the memberless threads: %d", w.user, status)
		}
	}
	if len(*trail) != 0 {
		t.Fatalf("a member's refused reach was audited: %+v", *trail)
	}

	// A SuperAdmin's own list is their own; the memberless ones answer only when
	// asked for by name, and both that list and the read are on the trail.
	if has(listed(t, app, super, false), old) {
		t.Fatal("a SuperAdmin's own list holds the memberless thread")
	}
	status, raw := call(t, app, http.MethodGet, "/v1/agent/conversations?memberless=true", super, nil)
	var nobodies struct {
		Conversations []convSummary `json:"conversations"`
	}
	_ = json.Unmarshal(raw, &nobodies)
	if status != http.StatusOK || !has(nobodies.Conversations, old) || len(nobodies.Conversations) != 1 {
		t.Fatalf("a SuperAdmin lists the memberless threads: %d %s", status, raw)
	}
	status, raw = call(t, app, http.MethodGet, "/v1/agent/conversations/"+old, super, nil)
	var tr thread
	_ = json.Unmarshal(raw, &tr)
	if status != http.StatusOK || len(tr.Messages) != 1 {
		t.Fatalf("a SuperAdmin reads the memberless thread: %d, %d messages", status, len(tr.Messages))
	}
	want := []audited{{"agent.conversation.list.memberless", "", ""}, {"agent.conversation.read.memberless", old, ""}}
	if len(*trail) != 2 || (*trail)[0] != want[0] || (*trail)[1] != want[1] {
		t.Fatalf("trail %+v; want %+v", *trail, want)
	}
	status, raw = call(t, app, http.MethodPost, "/v1/agent/conversations", super, recordRequest{ConversationID: old, Messages: []inMessage{{"user", "super writes"}}})
	var rec struct {
		ConversationID string `json:"conversationId"`
	}
	_ = json.Unmarshal(raw, &rec)
	if status != http.StatusOK || rec.ConversationID == old {
		t.Fatalf("a SuperAdmin wrote into the memberless thread: %d %s", status, raw)
	}
	for _, m := range []string{http.MethodPatch, http.MethodDelete} {
		if status, _ := call(t, app, m, "/v1/agent/conversations/"+old, super, map[string]any{"title": "x"}); status != http.StatusNotFound {
			t.Fatalf("a SuperAdmin %s the memberless thread: %d", m, status)
		}
	}
	if status, _ := call(t, app, http.MethodPost, "/v1/agent/conversations/"+old+"/shares", super, nil); status != http.StatusNotFound {
		t.Fatalf("a SuperAdmin shared the memberless thread: %d", status)
	}
	status, raw = call(t, app, http.MethodGet, "/v1/agent/conversations/"+old, super, nil)
	tr = thread{}
	_ = json.Unmarshal(raw, &tr)
	if status != http.StatusOK || len(tr.Messages) != 1 || tr.Messages[0].Content != "a stranger's words" {
		t.Fatalf("the memberless thread moved: %d %d messages", status, len(tr.Messages))
	}
}

// A caller that names an org and nobody in it lists, opens, records and runs
// nothing: what it opened would be nobody's.
func TestACallerWhoNamesNobodyIsRefused(t *testing.T) {
	app, _ := shareApp(t, t.TempDir())
	conv := recordTurns(t, app, "acme", "alice", "", inMessage{"user", "alice's"})
	anon := who{org: "acme"}
	for _, r := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/v1/agent/conversations", nil},
		{http.MethodGet, "/v1/agent/conversations/" + conv, nil},
		{http.MethodPost, "/v1/agent/conversations", recordRequest{Messages: []inMessage{{"user", "x"}}}},
		{http.MethodPost, "/v1/agent", map[string]any{"preset": "graph", "messages": []inMessage{{"user", "x"}}}},
	} {
		if status, raw := call(t, app, r.method, r.path, anon, r.body); status != http.StatusForbidden {
			t.Fatalf("%s %s with no user: %d %s", r.method, r.path, status, raw)
		}
	}
}

// The store's own rule, under the handlers: a member's list and reads are theirs
// alone, an id not theirs opens a fresh thread, and a memberless row is a
// SuperAdmin's to read and nobody's to own.
func TestConversationsAreTheMembers(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	const org = "acme"
	old := legacy(t, dir, org, "before users were kept", "old words")
	s := newStore(dir)
	defer s.closeAll()
	alice, err := s.loadOrCreateConversation(ctx, org, "alice", "", "alice asks")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.loadOrCreateConversation(ctx, org, "bob", "", "bob asks"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.loadOrCreateConversation(ctx, org, "", "", "nobody"); !errors.Is(err, errNoMember) {
		t.Fatalf("a caller naming nobody opened a thread: err=%v", err)
	}
	mine, err := s.listConversations(ctx, org, false, func(cv *Conversation) bool { return owns(cv, "bob") })
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) != 1 || mine[0].User != "bob" {
		t.Fatalf("bob lists %d; want his one", len(mine))
	}
	if all, _ := s.listConversations(ctx, org, false, nobodys); len(all) != 1 || all[0].Id() != old {
		t.Fatalf("the memberless list holds %d; want the one", len(all))
	}
	if _, _, err := s.conversationMessages(ctx, org, "root", true, old); err != nil {
		t.Fatalf("a SuperAdmin reads the memberless one: %v", err)
	}
	if _, _, err := s.conversationMessages(ctx, org, "root", true, alice.Id()); !errors.Is(err, orm.ErrNotFound) {
		t.Fatalf("a SuperAdmin reads alice's own: err=%v", err)
	}
	for _, id := range []string{alice.Id(), old} {
		if _, _, err := s.conversationMessages(ctx, org, "bob", false, id); !errors.Is(err, orm.ErrNotFound) {
			t.Fatalf("bob reads %s: err=%v", id, err)
		}
		fresh, err := s.loadOrCreateConversation(ctx, org, "bob", id, "x")
		if err != nil || fresh.Id() == id || fresh.User != "bob" {
			t.Fatalf("bob's turn under %s: %v, landed in %s", id, err, fresh.Id())
		}
	}
	if _, _, err := s.conversationMessages(ctx, org, "alice", false, alice.Id()); err != nil {
		t.Fatalf("alice reads her own: %v", err)
	}
	if n, err := memberless(mustDB(t, s, org)); err != nil || n != 1 {
		t.Fatalf("memberless = %d, %v; want 1", n, err)
	}
}

func mustDB(t *testing.T, s *store, org string) orm.DB {
	t.Helper()
	db, err := s.dbFor(org)
	if err != nil {
		t.Fatal(err)
	}
	return db
}
