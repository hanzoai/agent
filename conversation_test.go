package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	openai "github.com/hanzoai/go-openai"
	"github.com/hanzoai/orm"
	luxlog "github.com/luxfi/log"
	"github.com/zap-proto/zip"
)

// listed is the caller's conversations as the list answers them; archived asks
// for the archived ones.
func listed(t *testing.T, app *zip.App, w who, archived bool) []convSummary {
	t.Helper()
	path := "/v1/agent/conversations"
	if archived {
		path += "?archived=true"
	}
	status, raw := call(t, app, http.MethodGet, path, w, nil)
	if status != http.StatusOK {
		t.Fatalf("list as %s: %d %s", w.user, status, raw)
	}
	var out struct {
		Conversations []convSummary `json:"conversations"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	return out.Conversations
}

func ids(list []convSummary) []string {
	out := make([]string, len(list))
	for i, c := range list {
		out[i] = c.ID
	}
	return out
}

func same(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// patch changes one of w's conversations.
func patch(t *testing.T, app *zip.App, w who, conv string, body any) (int, convSummary, []byte) {
	t.Helper()
	status, raw := call(t, app, http.MethodPatch, "/v1/agent/conversations/"+conv, w, body)
	var out convSummary
	_ = json.Unmarshal(raw, &out)
	return status, out, raw
}

// A pinned conversation lists first, and pinning it does not make it recent.
func TestAPinnedConversationListsFirst(t *testing.T) {
	app, trail := shareApp(t, t.TempDir())
	older := recordTurns(t, app, "acme", "alice", "", inMessage{"user", "older"})
	time.Sleep(5 * time.Millisecond)
	newer := recordTurns(t, app, "acme", "alice", "", inMessage{"user", "newer"})
	if got := ids(listed(t, app, alice, false)); !same(got, []string{newer, older}) {
		t.Fatalf("before the pin: %v", got)
	}
	was := listed(t, app, alice, false)[1].UpdatedAt

	status, out, raw := patch(t, app, alice, older, map[string]any{"pinned": true})
	if status != http.StatusOK || !out.Pinned || out.ID != older {
		t.Fatalf("pin: %d %s", status, raw)
	}
	got := listed(t, app, alice, false)
	if !same(ids(got), []string{older, newer}) || !got[0].Pinned || got[1].Pinned {
		t.Fatalf("after the pin: %+v", got)
	}
	if !got[0].UpdatedAt.Equal(was) {
		t.Fatalf("the pin moved its last activity: %v → %v", was, got[0].UpdatedAt)
	}
	if n := len(*trail); n != 1 || (*trail)[0] != (audited{"agent.conversation.update", older, ""}) {
		t.Fatalf("audit: %+v", *trail)
	}

	if status, out, _ := patch(t, app, alice, older, map[string]any{"pinned": false}); status != http.StatusOK || out.Pinned {
		t.Fatalf("unpin: %d %+v", status, out)
	}
	if got := ids(listed(t, app, alice, false)); !same(got, []string{newer, older}) {
		t.Fatalf("after the unpin: %v", got)
	}
}

// An archived conversation leaves the list for the archived one, and comes back.
func TestAnArchivedConversationListsApart(t *testing.T) {
	app, _ := shareApp(t, t.TempDir())
	kept := recordTurns(t, app, "acme", "alice", "", inMessage{"user", "kept"})
	put := recordTurns(t, app, "acme", "alice", "", inMessage{"user", "put away"})

	if status, out, raw := patch(t, app, alice, put, map[string]any{"archived": true}); status != http.StatusOK || !out.Archived {
		t.Fatalf("archive: %d %s", status, raw)
	}
	if got := ids(listed(t, app, alice, false)); !same(got, []string{kept}) {
		t.Fatalf("the list holds %v", got)
	}
	away := listed(t, app, alice, true)
	if !same(ids(away), []string{put}) || !away[0].Archived {
		t.Fatalf("the archived list holds %+v", away)
	}
	// Still the member's own: it reads, and continues, as before.
	if status, raw := as(t, app, http.MethodGet, "/v1/agent/conversations/"+put, "acme", "alice", nil); status != http.StatusOK {
		t.Fatalf("read archived: %d %s", status, raw)
	}

	if status, _, raw := patch(t, app, alice, put, map[string]any{"archived": false}); status != http.StatusOK {
		t.Fatalf("unarchive: %d %s", status, raw)
	}
	if got := listed(t, app, alice, false); len(got) != 2 {
		t.Fatalf("after unarchiving the list holds %d", len(got))
	}
	if got := listed(t, app, alice, true); len(got) != 0 {
		t.Fatalf("after unarchiving the archived list holds %d", len(got))
	}
}

// A rename is one line, never empty, and a change must name something.
func TestRenameAConversation(t *testing.T) {
	app, _ := shareApp(t, t.TempDir())
	conv := recordTurns(t, app, "acme", "alice", "", inMessage{"user", "how do tides work"})

	status, out, raw := patch(t, app, alice, conv, map[string]any{"title": "  Tides\nand the moon "})
	if status != http.StatusOK || out.Title != "Tides and the moon" {
		t.Fatalf("rename: %d %s", status, raw)
	}
	if got := listed(t, app, alice, false); got[0].Title != "Tides and the moon" {
		t.Fatalf("listed as %q", got[0].Title)
	}
	for _, body := range []any{map[string]any{"title": "  \n "}, map[string]any{}} {
		if status, _, raw := patch(t, app, alice, conv, body); status != http.StatusBadRequest {
			t.Fatalf("%v: %d %s", body, status, raw)
		}
	}
	if got := listed(t, app, alice, false); got[0].Title != "Tides and the moon" {
		t.Fatalf("a refused change wrote %q", got[0].Title)
	}
}

// Deleting a conversation takes its turns and every link to it: the link stops
// opening, and it leaves the lists of the people it was shared with.
func TestDeleteTakesTheTurnsAndTheLinks(t *testing.T) {
	dir := t.TempDir()
	app, trail, svc := shareAppWith(t, dir, &stubCompleter{})
	conv := recordTurns(t, app, "acme", "alice", "", inMessage{"user", "how do tides work"}, inMessage{"assistant", "the moon pulls the sea"})
	kept := recordTurns(t, app, "acme", "alice", "", inMessage{"user", "another"})
	link := share(t, app, "acme", "alice", conv)
	if status, _, raw := open(t, app, "acme", "bob", link.Token); status != http.StatusOK {
		t.Fatalf("bob opens: %d %s", status, raw)
	}
	if got := sharedWith(t, app, bob); len(got) != 1 {
		t.Fatalf("shared with bob: %d", len(got))
	}

	status, raw := call(t, app, http.MethodDelete, "/v1/agent/conversations/"+conv, alice, nil)
	if status != http.StatusOK {
		t.Fatalf("delete: %d %s", status, raw)
	}
	if status, raw := call(t, app, http.MethodGet, "/v1/agent/conversations/"+conv, alice, nil); status != http.StatusNotFound {
		t.Fatalf("read after delete: %d %s", status, raw)
	}
	if got := ids(listed(t, app, alice, false)); !same(got, []string{kept}) {
		t.Fatalf("the list holds %v", got)
	}
	if status, _, raw := open(t, app, "acme", "bob", link.Token); status != http.StatusNotFound {
		t.Fatalf("the link still opens: %d %s", status, raw)
	}
	if got := sharedWith(t, app, bob); len(got) != 0 {
		t.Fatalf("bob still lists %d", len(got))
	}
	db, err := svc.store.dbFor("acme")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if turns, _ := orm.TypedQuery[Message](db).Filter("ConversationId=", conv).GetAll(ctx); len(turns) != 0 {
		t.Fatalf("%d turns outlived their conversation", len(turns))
	}
	if links, _ := orm.TypedQuery[Share](db).Filter("ConversationId=", conv).GetAll(ctx); len(links) != 0 {
		t.Fatalf("%d links outlived their conversation", len(links))
	}
	if turns, _ := orm.TypedQuery[Message](db).Filter("ConversationId=", kept).GetAll(ctx); len(turns) != 1 {
		t.Fatalf("the other conversation holds %d turns", len(turns))
	}
	if (*trail)[len(*trail)-1] != (audited{"agent.conversation.delete", conv, ""}) {
		t.Fatalf("audit: %+v", *trail)
	}
	if status, raw := call(t, app, http.MethodDelete, "/v1/agent/conversations/"+conv, alice, nil); status != http.StatusNotFound {
		t.Fatalf("delete again: %d %s", status, raw)
	}
}

// Only the member who opened a conversation changes or deletes it: another
// member's, another org's and one recorded with no member answer 404, and
// nothing about it moves.
func TestOnlyTheOwnerChangesOrDeletes(t *testing.T) {
	app, trail := shareApp(t, t.TempDir())
	conv := recordTurns(t, app, "acme", "alice", "", inMessage{"user", "alice's"})
	theirs := recordTurns(t, app, "other", "zed", "", inMessage{"user", "zed's"})
	nobodys := recordTurns(t, app, "acme", "", "", inMessage{"user", "before users were kept"})

	refused := []struct {
		w    who
		conv string
	}{
		{bob, conv},      // another member of the org
		{zed, conv},      // another org, naming an id it does not hold
		{alice, theirs},  // another org's id, asked in alice's own
		{alice, nobodys}, // recorded with no member: nobody's to change
		{boss, conv},     // an org admin is not the owner
	}
	for _, r := range refused {
		if status, _, raw := patch(t, app, r.w, r.conv, map[string]any{"pinned": true, "archived": true, "title": "taken"}); status != http.StatusNotFound {
			t.Fatalf("%s/%s patches %s: %d %s", r.w.org, r.w.user, r.conv, status, raw)
		}
		if status, raw := call(t, app, http.MethodDelete, "/v1/agent/conversations/"+r.conv, r.w, nil); status != http.StatusNotFound {
			t.Fatalf("%s/%s deletes %s: %d %s", r.w.org, r.w.user, r.conv, status, raw)
		}
	}
	if status, _, _ := patch(t, app, nobody, conv, map[string]any{"pinned": true}); status != http.StatusForbidden {
		t.Fatalf("no principal patches: %d", status)
	}
	if status, _ := call(t, app, http.MethodDelete, "/v1/agent/conversations/"+conv, nobody, nil); status != http.StatusForbidden {
		t.Fatalf("no principal deletes: %d", status)
	}

	mine := listed(t, app, alice, false)
	if len(mine) != 2 {
		t.Fatalf("alice lists %d", len(mine))
	}
	for _, c := range mine {
		if c.Pinned || c.Archived || c.Title == "taken" {
			t.Fatalf("a refused change moved %+v", c)
		}
	}
	if got := ids(listed(t, app, zed, false)); !same(got, []string{theirs}) {
		t.Fatalf("zed lists %v", got)
	}
	if len(*trail) != 0 {
		t.Fatalf("a refused change was audited: %+v", *trail)
	}
}

// during runs a function while the model answers, as a person acting in another
// tab would.
type during struct {
	resp openai.ChatCompletionResponse
	do   func()
}

func (d *during) Complete(context.Context, map[string]string, openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	d.do()
	return d.resp, nil
}

func roundApp(t *testing.T, completer Completer) *zip.App {
	t.Helper()
	app := zip.New(zip.Config{Logger: luxlog.New("test")})
	svc, err := Mount(app, Deps{Logger: luxlog.New("test"), DataDir: t.TempDir(), Model: "zen"}, completer, &stubPlane{})
	if err != nil {
		t.Fatalf("Mount: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return app
}

// A pin made while the model answers stands once the answer is kept, and a
// conversation deleted meanwhile is not brought back by it.
func TestARoundKeepsAChangeMadeWhileItRan(t *testing.T) {
	d := &during{resp: completion("the moon")}
	app := roundApp(t, d)
	conv := recordTurns(t, app, "acme", "alice", "", inMessage{"user", "tides"})

	d.do = func() { patch(t, app, alice, conv, map[string]any{"pinned": true, "title": "Tides"}) }
	body := map[string]any{"conversationId": conv, "messages": []inMessage{{Role: "user", Content: "and the moon?"}}}
	if status, raw := call(t, app, http.MethodPost, "/v1/agent", alice, body); status != http.StatusOK {
		t.Fatalf("round: %d %s", status, raw)
	}
	got := listed(t, app, alice, false)
	if len(got) != 1 || !got[0].Pinned || got[0].Title != "Tides" {
		t.Fatalf("the round wrote over the pin: %+v", got)
	}

	d.do = func() { call(t, app, http.MethodDelete, "/v1/agent/conversations/"+conv, alice, nil) }
	if status, raw := call(t, app, http.MethodPost, "/v1/agent", alice, body); status != http.StatusOK {
		t.Fatalf("round: %d %s", status, raw)
	}
	if got := listed(t, app, alice, false); len(got) != 0 {
		t.Fatalf("the round brought back a deleted conversation: %+v", got)
	}
	if status, raw := call(t, app, http.MethodGet, "/v1/agent/conversations/"+conv, alice, nil); status != http.StatusNotFound {
		t.Fatalf("read after delete: %d %s", status, raw)
	}
}
