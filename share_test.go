package agent

import (
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	luxlog "github.com/luxfi/log"
	fiber "github.com/zap-proto/fiber/v3"
	"github.com/zap-proto/zip"
)

type audited struct{ action, conversation, share string }

// shareApp mounts the service over dir with an audit hook that records each call.
func shareApp(t *testing.T, dir string) (*zip.App, *[]audited) {
	t.Helper()
	var trail []audited
	app := zip.New(zip.Config{Logger: luxlog.New("test")})
	svc, err := Mount(app, Deps{
		Logger:  luxlog.New("test"),
		DataDir: dir,
		Model:   "zen",
		Audit: func(_ *zip.Ctx, action, conversation, share string) {
			trail = append(trail, audited{action, conversation, share})
		},
	}, &stubCompleter{}, &stubPlane{})
	if err != nil {
		t.Fatalf("Mount: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return app, &trail
}

// as issues one request as user in org; org == "" sends no identity at all.
func as(t *testing.T, app *zip.App, method, path, org, user string, body any) (int, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	rq := httptest.NewRequest(method, path, r)
	rq.Header.Set("Content-Type", "application/json")
	if org != "" {
		rq.Header.Set("X-Org-Id", org)
		rq.Header.Set("X-User-Id", user)
	}
	resp, err := app.Fiber().Test(rq, fiber.TestConfig{Timeout: 0})
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

// recordTurns writes turns as user in org and returns the conversation id.
func recordTurns(t *testing.T, app *zip.App, org, user, id string, turns ...inMessage) string {
	t.Helper()
	status, raw := as(t, app, http.MethodPost, "/v1/agent/conversations", org, user, recordRequest{ConversationID: id, Messages: turns})
	if status != http.StatusOK {
		t.Fatalf("record: %d %s", status, raw)
	}
	var out struct {
		ConversationID string `json:"conversationId"`
	}
	_ = json.Unmarshal(raw, &out)
	return out.ConversationID
}

type made struct {
	Share shareOut `json:"share"`
	Token string   `json:"token"`
}

func share(t *testing.T, app *zip.App, org, user, conv string) made {
	t.Helper()
	status, raw := as(t, app, http.MethodPost, "/v1/agent/conversations/"+conv+"/shares", org, user, nil)
	if status != http.StatusCreated {
		t.Fatalf("share: %d %s", status, raw)
	}
	var out made
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode share: %v", err)
	}
	return out
}

type opened struct {
	Title    string       `json:"title"`
	Access   string       `json:"access"`
	Full     bool         `json:"full"`
	Turns    int          `json:"turns"`
	Messages []sharedTurn `json:"messages"`
}

func open(t *testing.T, app *zip.App, org, user, token string) (int, opened, []byte) {
	t.Helper()
	status, raw := as(t, app, http.MethodPost, "/v1/agent/shares/read", org, user, openRequest{Token: token})
	var out opened
	_ = json.Unmarshal(raw, &out)
	return status, out, raw
}

// A secret is 256 random bits, differs every time, and neither store holds it:
// only its hash is written.
func TestShareSecretIsUnguessableAndSealed(t *testing.T) {
	dir := t.TempDir()
	app, _ := shareApp(t, dir)
	conv := recordTurns(t, app, "acme", "alice", "", inMessage{"user", "how do tides work"}, inMessage{"assistant", "the moon pulls the sea"})
	a := share(t, app, "acme", "alice", conv)
	b := share(t, app, "acme", "alice", conv)
	if !wellFormed(a.Token) || len(a.Token) != 43 {
		t.Fatalf("token %q is not 32 bytes of base64url", a.Token)
	}
	if a.Token == b.Token || a.Share.ID == b.Share.ID {
		t.Fatal("two shares minted the same token or id")
	}
	if strings.Contains(a.Token, conv) || strings.Contains(a.Token, "acme") {
		t.Fatal("the token carries the conversation id or the org")
	}
	var found struct{ secret, hash bool }
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, _ := os.ReadFile(path)
		if bytes.Contains(raw, []byte(a.Token)) {
			found.secret = true
		}
		if bytes.Contains(raw, []byte(seal(a.Token))) {
			found.hash = true
		}
		return nil
	})
	if found.secret {
		t.Fatal("a store holds the secret itself")
	}
	if !found.hash {
		t.Fatal("no store holds the secret's hash")
	}
}

// Revoking a link makes it read as never having existed.
func TestRevokeKillsReads(t *testing.T) {
	app, trail := shareApp(t, t.TempDir())
	conv := recordTurns(t, app, "acme", "alice", "", inMessage{"user", "q"}, inMessage{"assistant", "a"})
	s := share(t, app, "acme", "alice", conv)
	if status, got, raw := open(t, app, "other", "zed", s.Token); status != http.StatusOK || !got.Full {
		t.Fatalf("open before revoke: %d %s", status, raw)
	}
	status, raw := as(t, app, http.MethodDelete, "/v1/agent/conversations/"+conv+"/shares/"+s.Share.ID, "acme", "alice", nil)
	if status != http.StatusOK {
		t.Fatalf("revoke: %d %s", status, raw)
	}
	status, _, raw = open(t, app, "other", "zed", s.Token)
	if status != http.StatusNotFound || !strings.Contains(string(raw), "no longer shared") {
		t.Fatalf("open after revoke: %d %s", status, raw)
	}
	if status, _, _ := open(t, app, "", "", s.Token); status != http.StatusNotFound {
		t.Fatalf("signed-out open after revoke: %d", status)
	}
	status, raw = as(t, app, http.MethodGet, "/v1/agent/conversations/"+conv+"/shares", "acme", "alice", nil)
	if status != http.StatusOK || strings.Contains(string(raw), s.Share.ID) {
		t.Fatalf("a revoked share is still listed: %d %s", status, raw)
	}
	want := []audited{{"agent.share.create", conv, s.Share.ID}, {"agent.share.revoke", conv, s.Share.ID}}
	if len(*trail) != 2 || (*trail)[0] != want[0] || (*trail)[1] != want[1] {
		t.Fatalf("audit trail = %+v, want %+v", *trail, want)
	}
}

// A reader in another org sees the shared conversation's turns and nothing
// else of the owner's org: no other conversation, no system turn, no tool turn,
// and not the turns added after the link was made.
func TestCrossOrgViewerReadsOnlyTheShare(t *testing.T) {
	app, _ := shareApp(t, t.TempDir())
	conv := recordTurns(t, app, "acme", "alice", "",
		inMessage{"system", "SECRET BRIEF"},
		inMessage{"user", "what is a hash table"},
		inMessage{"tool", "TOOL OUTPUT"},
		inMessage{"assistant", "a coat check"},
	)
	private := recordTurns(t, app, "acme", "alice", "", inMessage{"user", "PRIVATE THREAD"}, inMessage{"assistant", "x"})
	s := share(t, app, "acme", "alice", conv)
	recordTurns(t, app, "acme", "alice", conv, inMessage{"user", "LATER QUESTION"}, inMessage{"assistant", "LATER ANSWER"})

	status, got, raw := open(t, app, "other", "zed", s.Token)
	if status != http.StatusOK {
		t.Fatalf("open: %d %s", status, raw)
	}
	if !got.Full || got.Access != AccessRead || got.Title != "what is a hash table" {
		t.Fatalf("open answered %+v", got)
	}
	if len(got.Messages) != 2 || got.Messages[0].Content != "what is a hash table" || got.Messages[1].Content != "a coat check" {
		t.Fatalf("shared turns = %+v", got.Messages)
	}
	for _, leak := range []string{"SECRET BRIEF", "TOOL OUTPUT", "LATER", "PRIVATE THREAD", "alice", "acme", conv} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("the shared read carries %q: %s", leak, raw)
		}
	}
	// The viewer's own org is untouched: acme's threads are not theirs to list or open.
	if _, raw := as(t, app, http.MethodGet, "/v1/agent/conversations", "other", "zed", nil); strings.Contains(string(raw), conv) || strings.Contains(string(raw), private) {
		t.Fatalf("the viewer lists the owner's threads: %s", raw)
	}
	if _, raw := as(t, app, http.MethodGet, "/v1/agent/conversations/"+conv, "other", "zed", nil); strings.Contains(string(raw), "hash table") {
		t.Fatalf("the viewer opens the owner's thread by id: %s", raw)
	}
}

// Nobody signed in gets the title and the first two turns, clipped.
func TestSignedOutGetsPreviewOnly(t *testing.T) {
	app, _ := shareApp(t, t.TempDir())
	long := strings.Repeat("tide ", 200)
	conv := recordTurns(t, app, "acme", "alice", "",
		inMessage{"user", "first question"}, inMessage{"assistant", long},
		inMessage{"user", "second question"}, inMessage{"assistant", "second answer"},
	)
	s := share(t, app, "acme", "alice", conv)
	status, got, raw := open(t, app, "", "", s.Token)
	if status != http.StatusOK {
		t.Fatalf("open: %d %s", status, raw)
	}
	if got.Full || got.Title != "first question" || got.Turns != 4 {
		t.Fatalf("preview = %+v", got)
	}
	if len(got.Messages) != previewTurns {
		t.Fatalf("preview carries %d turns", len(got.Messages))
	}
	if n := utf8.RuneCountInString(got.Messages[1].Content); n > previewRunes+1 || !strings.HasSuffix(got.Messages[1].Content, "…") {
		t.Fatalf("preview turn is %d runes: %q", n, got.Messages[1].Content)
	}
	if strings.Contains(string(raw), "second question") {
		t.Fatalf("the preview carries turns past the first two: %s", raw)
	}
}

// A conversation id is not a key: no spelling of one opens a share.
func TestThreadIDAloneNeverReads(t *testing.T) {
	app, _ := shareApp(t, t.TempDir())
	conv := recordTurns(t, app, "acme", "alice", "", inMessage{"user", "q"}, inMessage{"assistant", "a"})
	s := share(t, app, "acme", "alice", conv)
	for _, guess := range []string{conv, s.Share.ID, seal(s.Token), "", strings.Repeat("A", 43), s.Token[:42], s.Token + "A"} {
		if status, _, raw := open(t, app, "other", "zed", guess); status != http.StatusNotFound {
			t.Fatalf("token %q opened: %d %s", guess, status, raw)
		}
	}
}

// Only the member who owns a conversation shares it, lists its links or revokes one.
func TestOnlyTheOwnerShares(t *testing.T) {
	app, _ := shareApp(t, t.TempDir())
	conv := recordTurns(t, app, "acme", "alice", "", inMessage{"user", "q"}, inMessage{"assistant", "a"})
	s := share(t, app, "acme", "alice", conv)
	for _, who := range []struct{ org, user string }{{"acme", "bob"}, {"other", "alice"}} {
		if status, raw := as(t, app, http.MethodPost, "/v1/agent/conversations/"+conv+"/shares", who.org, who.user, nil); status != http.StatusNotFound {
			t.Fatalf("%s/%s shared alice's thread: %d %s", who.org, who.user, status, raw)
		}
		if status, raw := as(t, app, http.MethodGet, "/v1/agent/conversations/"+conv+"/shares", who.org, who.user, nil); status != http.StatusNotFound {
			t.Fatalf("%s/%s listed alice's links: %d %s", who.org, who.user, status, raw)
		}
		if status, raw := as(t, app, http.MethodDelete, "/v1/agent/conversations/"+conv+"/shares/"+s.Share.ID, who.org, who.user, nil); status != http.StatusNotFound {
			t.Fatalf("%s/%s revoked alice's link: %d %s", who.org, who.user, status, raw)
		}
	}
	if status, _ := as(t, app, http.MethodPost, "/v1/agent/conversations/"+conv+"/shares", "", "", nil); status != http.StatusForbidden {
		t.Fatalf("an anonymous caller made a share: %d", status)
	}
	if status, _, _ := open(t, app, "other", "zed", s.Token); status != http.StatusOK {
		t.Fatal("the link stopped opening after refused revokes")
	}
}
