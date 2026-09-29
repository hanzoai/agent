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

	openai "github.com/hanzoai/go-openai"
	luxlog "github.com/luxfi/log"
	fiber "github.com/zap-proto/fiber/v3"
	"github.com/zap-proto/zip"
)

type audited struct{ action, conversation, share string }

// shareApp mounts the service over dir with an audit hook that records each call.
func shareApp(t *testing.T, dir string) (*zip.App, *[]audited) {
	t.Helper()
	app, trail, _ := shareAppWith(t, dir, &stubCompleter{})
	return app, trail
}

func shareAppWith(t *testing.T, dir string, completer *stubCompleter) (*zip.App, *[]audited, *Service) {
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
	}, completer, &stubPlane{})
	if err != nil {
		t.Fatalf("Mount: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return app, &trail, svc
}

// who is a caller as the gateway names one: org, user, email, and whether they
// administer the org. The zero value is nobody at all.
type who struct {
	org, user, email string
	admin            bool
}

var (
	alice  = who{org: "acme", user: "alice", email: "alice@acme.test"}
	bob    = who{org: "acme", user: "bob", email: "bob@acme.test"}
	carol  = who{org: "acme", user: "carol", email: "carol@acme.test"}
	boss   = who{org: "acme", user: "boss", email: "boss@acme.test", admin: true}
	zed    = who{org: "other", user: "zed", email: "zed@other.test"}
	yan    = who{org: "other", user: "yan", email: "yan@other.test"}
	quinn  = who{org: "third", user: "quinn", email: "quinn@third.test"}
	rival  = who{org: "other", user: "rival", email: "rival@other.test", admin: true}
	nobody = who{}
)

// call issues one request as w; the zero who sends no identity at all.
func call(t *testing.T, app *zip.App, method, path string, w who, body any) (int, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	rq := httptest.NewRequest(method, path, r)
	rq.Header.Set("Content-Type", "application/json")
	if w.org != "" {
		rq.Header.Set("X-Org-Id", w.org)
		rq.Header.Set("X-User-Id", w.user)
		rq.Header.Set("X-User-Email", w.email)
		if w.admin {
			rq.Header.Set("X-User-IsOrgAdmin", "true")
		}
	}
	resp, err := app.Fiber().Test(rq, fiber.TestConfig{Timeout: 0})
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

// as issues one request as user in org; org == "" sends no identity at all.
func as(t *testing.T, app *zip.App, method, path, org, user string, body any) (int, []byte) {
	t.Helper()
	return call(t, app, method, path, who{org: org, user: user}, body)
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
	Share    string       `json:"share"`
	Title    string       `json:"title"`
	Access   string       `json:"access"`
	Full     bool         `json:"full"`
	Messages []sharedTurn `json:"messages"`
}

// openAs opens a link as w.
func openAs(t *testing.T, app *zip.App, w who, token string) (int, opened, []byte) {
	t.Helper()
	status, raw := call(t, app, http.MethodPost, "/v1/agent/shares/read", w, openRequest{Token: token})
	var out opened
	_ = json.Unmarshal(raw, &out)
	return status, out, raw
}

func open(t *testing.T, app *zip.App, org, user, token string) (int, opened, []byte) {
	t.Helper()
	return openAs(t, app, who{org: org, user: user}, token)
}

// sharedWith is the list of chats shared with w.
func sharedWith(t *testing.T, app *zip.App, w who) []sharedItem {
	t.Helper()
	status, raw := call(t, app, http.MethodGet, "/v1/agent/shared", w, nil)
	if status != http.StatusOK {
		t.Fatalf("shared with %s: %d %s", w.user, status, raw)
	}
	var out struct {
		Shared []sharedItem `json:"shared"`
	}
	_ = json.Unmarshal(raw, &out)
	return out.Shared
}

// readShared reads a chat shared with w by its share id, without the link.
func readShared(t *testing.T, app *zip.App, w who, id string) (int, opened) {
	t.Helper()
	status, raw := call(t, app, http.MethodGet, "/v1/agent/shared/"+id, w, nil)
	var out opened
	_ = json.Unmarshal(raw, &out)
	return status, out
}

func linksOf(t *testing.T, app *zip.App, w who, conv string) []shareOut {
	t.Helper()
	status, raw := call(t, app, http.MethodGet, "/v1/agent/conversations/"+conv+"/shares", w, nil)
	if status != http.StatusOK {
		t.Fatalf("links: %d %s", status, raw)
	}
	var out struct {
		Shares []shareOut `json:"shares"`
	}
	_ = json.Unmarshal(raw, &out)
	return out.Shares
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
	open(t, app, "other", "zed", a.Token)
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

// Nobody signed in gets the title and not one word of the transcript.
func TestSignedOutGetsTheTitleOnly(t *testing.T) {
	app, _ := shareApp(t, t.TempDir())
	conv := recordTurns(t, app, "acme", "alice", "",
		inMessage{"user", "first question"}, inMessage{"assistant", "FIRST ANSWER"},
		inMessage{"user", "second question"}, inMessage{"assistant", "second answer"},
	)
	s := share(t, app, "acme", "alice", conv)
	status, got, raw := openAs(t, app, nobody, s.Token)
	if status != http.StatusOK {
		t.Fatalf("open: %d %s", status, raw)
	}
	if got.Full || got.Title != "first question" || len(got.Messages) != 0 || got.Share != "" {
		t.Fatalf("signed-out open = %+v", got)
	}
	for _, leak := range []string{"FIRST ANSWER", "second", "turns", s.Share.ID} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("the signed-out answer carries %q: %s", leak, raw)
		}
	}
	// Nor does a principal that names no user.
	if _, got, raw := openAs(t, app, who{org: "other"}, s.Token); got.Full || len(got.Messages) != 0 {
		t.Fatalf("a principal with no user read the transcript: %s", raw)
	}
	if links := linksOf(t, app, alice, conv); len(links[0].Viewers) != 0 {
		t.Fatalf("a signed-out open was recorded as a viewer: %+v", links[0].Viewers)
	}
}

// A caller that is not a signed-in person — an application, an API key — is
// told the title and nothing else, is not recorded, and lists nothing.
func TestOnlyAPersonIsAViewer(t *testing.T) {
	var trail []audited
	app := zip.New(zip.Config{Logger: luxlog.New("test")})
	svc, err := Mount(app, Deps{
		Logger:  luxlog.New("test"),
		DataDir: t.TempDir(),
		Principal: func(c *zip.Ctx) (Principal, bool) {
			p, ok := headerPrincipal(c)
			p.Person = ok && !strings.HasPrefix(p.User, "app:")
			return p, ok
		},
		Audit: func(_ *zip.Ctx, action, conversation, share string) {
			trail = append(trail, audited{action, conversation, share})
		},
	}, &stubCompleter{}, &stubPlane{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	conv := recordTurns(t, app, "acme", "alice", "", inMessage{"user", "q"}, inMessage{"assistant", "SECRET ANSWER"})
	s := share(t, app, "acme", "alice", conv)
	bot := who{org: "other", user: "app:other/bot"}
	if status, got, raw := openAs(t, app, bot, s.Token); status != http.StatusOK || got.Full || strings.Contains(string(raw), "SECRET") {
		t.Fatalf("an application opened the transcript: %d %s", status, raw)
	}
	if status, _ := call(t, app, http.MethodGet, "/v1/agent/shared", bot, nil); status != http.StatusForbidden {
		t.Fatalf("an application listed chats shared with it: %d", status)
	}
	if links := linksOf(t, app, alice, conv); len(links[0].Viewers) != 0 {
		t.Fatalf("an application was recorded as a viewer: %+v", links[0].Viewers)
	}
}

// A signed-in reader who opens the link is recorded as a viewer, reads the
// snapshot, and then finds it among the chats shared with them and reads it by
// its share id without the link. An account that never opened the link can
// neither list nor read it.
func TestARecipientFindsItUnderSharedWithThem(t *testing.T) {
	app, _ := shareApp(t, t.TempDir())
	conv := recordTurns(t, app, "acme", "alice", "", inMessage{"user", "what is a hash table"}, inMessage{"assistant", "a coat check"})
	s := share(t, app, "acme", "alice", conv)

	if got := sharedWith(t, app, zed); len(got) != 0 {
		t.Fatalf("zed lists %+v before opening anything", got)
	}
	status, got, raw := openAs(t, app, zed, s.Token)
	if status != http.StatusOK || !got.Full || len(got.Messages) != 2 || got.Share != s.Share.ID {
		t.Fatalf("zed opens: %d %s", status, raw)
	}
	list := sharedWith(t, app, zed)
	if len(list) != 1 || list[0].Share != s.Share.ID || list[0].Title != "what is a hash table" {
		t.Fatalf("zed's shared list = %+v", list)
	}
	if status, read := readShared(t, app, zed, s.Share.ID); status != http.StatusOK || len(read.Messages) != 2 || read.Messages[1].Content != "a coat check" {
		t.Fatalf("zed reads by share id: %d %+v", status, read)
	}
	// Opening again records no second viewer.
	openAs(t, app, zed, s.Token)
	if links := linksOf(t, app, alice, conv); len(links) != 1 || len(links[0].Viewers) != 1 || links[0].Viewers[0].Name != "zed@other.test" {
		t.Fatalf("alice sees viewers %+v", links)
	}

	// Quinn has no link: nothing to list, nothing to read by id, conversation id or secret hash.
	if got := sharedWith(t, app, quinn); len(got) != 0 {
		t.Fatalf("quinn lists %+v", got)
	}
	for _, guess := range []string{s.Share.ID, conv, seal(s.Token), list[0].Share + "x"} {
		if status, read := readShared(t, app, quinn, guess); status != http.StatusNotFound || len(read.Messages) != 0 {
			t.Fatalf("quinn read %q: %d %+v", guess, status, read)
		}
	}
	// Nor can a signed-out caller list or read by id.
	if status, _ := call(t, app, http.MethodGet, "/v1/agent/shared", nobody, nil); status != http.StatusForbidden {
		t.Fatalf("signed-out list: %d", status)
	}
	if status, _ := readShared(t, app, nobody, s.Share.ID); status != http.StatusForbidden {
		t.Fatalf("signed-out read by id: %d", status)
	}
	// The owner opening their own link is not a viewer of it.
	openAs(t, app, alice, s.Token)
	if links := linksOf(t, app, alice, conv); len(links[0].Viewers) != 1 {
		t.Fatalf("the owner was recorded as a viewer: %+v", links[0].Viewers)
	}
	if got := sharedWith(t, app, alice); len(got) != 0 {
		t.Fatalf("the owner lists their own chat as shared with them: %+v", got)
	}
	// The owner reads their own share by its id; another member of their org does not.
	if status, read := readShared(t, app, alice, s.Share.ID); status != http.StatusOK || len(read.Messages) != 2 {
		t.Fatalf("the owner reads their share by id: %d %+v", status, read)
	}
	if status, _ := readShared(t, app, bob, s.Share.ID); status != http.StatusNotFound {
		t.Fatalf("another member reads alice's share by id: %d", status)
	}
}

// Revoking a link ends it for every viewer and makes the secret read as never
// having existed.
func TestRevokeRemovesEveryViewer(t *testing.T) {
	app, trail := shareApp(t, t.TempDir())
	conv := recordTurns(t, app, "acme", "alice", "", inMessage{"user", "q"}, inMessage{"assistant", "a"})
	s := share(t, app, "acme", "alice", conv)
	for _, w := range []who{zed, yan, bob} {
		if status, got, raw := openAs(t, app, w, s.Token); status != http.StatusOK || !got.Full {
			t.Fatalf("%s opens: %d %s", w.user, status, raw)
		}
	}
	status, raw := call(t, app, http.MethodDelete, "/v1/agent/conversations/"+conv+"/shares/"+s.Share.ID, alice, nil)
	if status != http.StatusOK {
		t.Fatalf("revoke: %d %s", status, raw)
	}
	for _, w := range []who{zed, yan, bob} {
		if got := sharedWith(t, app, w); len(got) != 0 {
			t.Fatalf("%s still lists %+v after revoke", w.user, got)
		}
		if status, _ := readShared(t, app, w, s.Share.ID); status != http.StatusNotFound {
			t.Fatalf("%s reads by id after revoke: %d", w.user, status)
		}
		status, _, raw := openAs(t, app, w, s.Token)
		if status != http.StatusNotFound || !strings.Contains(string(raw), "no longer shared") {
			t.Fatalf("%s opens after revoke: %d %s", w.user, status, raw)
		}
	}
	if status, _, _ := openAs(t, app, nobody, s.Token); status != http.StatusNotFound {
		t.Fatalf("signed-out open after revoke: %d", status)
	}
	if links := linksOf(t, app, alice, conv); len(links) != 0 {
		t.Fatalf("a revoked share is still listed: %+v", links)
	}
	want := []audited{{"agent.share.create", conv, s.Share.ID}, {"agent.share.revoke", conv, s.Share.ID}}
	if len(*trail) != 2 || (*trail)[0] != want[0] || (*trail)[1] != want[1] {
		t.Fatalf("audit trail = %+v, want %+v", *trail, want)
	}
}

// The owner sees who opened a link and removes one of them; that viewer loses it
// and cannot open it again with the link, and the others keep it.
func TestTheOwnerRemovesOneViewer(t *testing.T) {
	app, trail := shareApp(t, t.TempDir())
	conv := recordTurns(t, app, "acme", "alice", "", inMessage{"user", "q"}, inMessage{"assistant", "a"})
	s := share(t, app, "acme", "alice", conv)
	openAs(t, app, zed, s.Token)
	openAs(t, app, yan, s.Token)
	links := linksOf(t, app, alice, conv)
	if len(links) != 1 || len(links[0].Viewers) != 2 {
		t.Fatalf("viewers = %+v", links)
	}
	var zedRow string
	for _, v := range links[0].Viewers {
		if v.Name == "zed@other.test" {
			zedRow = v.ID
		}
	}
	path := "/v1/agent/conversations/" + conv + "/shares/" + s.Share.ID + "/viewers/" + zedRow
	for _, w := range []who{bob, zed, rival} {
		if status, raw := call(t, app, http.MethodDelete, path, w, nil); status != http.StatusNotFound {
			t.Fatalf("%s removed a viewer of alice's link: %d %s", w.user, status, raw)
		}
	}
	if status, raw := call(t, app, http.MethodDelete, path, alice, nil); status != http.StatusOK {
		t.Fatalf("remove zed: %d %s", status, raw)
	}
	if got := sharedWith(t, app, zed); len(got) != 0 {
		t.Fatalf("a removed viewer still lists %+v", got)
	}
	if status, _ := readShared(t, app, zed, s.Share.ID); status != http.StatusNotFound {
		t.Fatalf("a removed viewer reads by id: %d", status)
	}
	if status, got, raw := openAs(t, app, zed, s.Token); status != http.StatusNotFound || got.Full {
		t.Fatalf("a removed viewer reopens the link: %d %s", status, raw)
	}
	if got := sharedWith(t, app, yan); len(got) != 1 {
		t.Fatalf("yan lost the chat when zed was removed: %+v", got)
	}
	if links := linksOf(t, app, alice, conv); len(links[0].Viewers) != 1 || links[0].Viewers[0].Name != "yan@other.test" {
		t.Fatalf("viewers after removing zed = %+v", links[0].Viewers)
	}
	if last := (*trail)[len(*trail)-1]; last != (audited{"agent.share.unview", conv, s.Share.ID}) {
		t.Fatalf("the removal was not audited: %+v", *trail)
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
	if _, raw := as(t, app, http.MethodGet, "/v1/agent/conversations", "other", "zed", nil); strings.Contains(string(raw), conv) || strings.Contains(string(raw), private) {
		t.Fatalf("the viewer lists the owner's threads: %s", raw)
	}
	if _, raw := as(t, app, http.MethodGet, "/v1/agent/conversations/"+conv, "other", "zed", nil); strings.Contains(string(raw), "hash table") {
		t.Fatalf("the viewer opens the owner's thread by id: %s", raw)
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
	if got := sharedWith(t, app, zed); len(got) != 0 {
		t.Fatalf("failed guesses recorded zed as a viewer: %+v", got)
	}
}

// Only the member who owns a conversation shares it, lists its links or revokes one.
func TestOnlyTheOwnerShares(t *testing.T) {
	app, _ := shareApp(t, t.TempDir())
	conv := recordTurns(t, app, "acme", "alice", "", inMessage{"user", "q"}, inMessage{"assistant", "a"})
	s := share(t, app, "acme", "alice", conv)
	for _, w := range []who{bob, boss, {org: "other", user: "alice"}} {
		if status, raw := call(t, app, http.MethodPost, "/v1/agent/conversations/"+conv+"/shares", w, nil); status != http.StatusNotFound {
			t.Fatalf("%s/%s shared alice's thread: %d %s", w.org, w.user, status, raw)
		}
		if status, raw := call(t, app, http.MethodGet, "/v1/agent/conversations/"+conv+"/shares", w, nil); status != http.StatusNotFound {
			t.Fatalf("%s/%s listed alice's links: %d %s", w.org, w.user, status, raw)
		}
		if status, raw := call(t, app, http.MethodDelete, "/v1/agent/conversations/"+conv+"/shares/"+s.Share.ID, w, nil); status != http.StatusNotFound {
			t.Fatalf("%s/%s revoked alice's link: %d %s", w.org, w.user, status, raw)
		}
	}
	if status, _ := call(t, app, http.MethodPost, "/v1/agent/conversations/"+conv+"/shares", nobody, nil); status != http.StatusForbidden {
		t.Fatalf("an anonymous caller made a share: %d", status)
	}
	if status, _, _ := open(t, app, "other", "zed", s.Token); status != http.StatusOK {
		t.Fatal("the link stopped opening after refused revokes")
	}
}

// A thread recorded with no member — before users were kept, or by a principal
// that names none — is nobody's to share: not by any member, and not by the
// empty user it was recorded under. (Regression: owns(cv, "") admitted every
// member of the org.)
func TestAUserlessConversationIsNobodysToShare(t *testing.T) {
	app, _ := shareApp(t, t.TempDir())
	conv := recordTurns(t, app, "acme", "", "", inMessage{"user", "internal q"}, inMessage{"assistant", "internal a"})
	for _, w := range []who{bob, carol, boss, {org: "acme"}} {
		if status, raw := call(t, app, http.MethodPost, "/v1/agent/conversations/"+conv+"/shares", w, nil); status != http.StatusNotFound && status != http.StatusForbidden {
			t.Fatalf("%q shared a userless thread: %d %s", w.user, status, raw)
		}
		if status, _ := call(t, app, http.MethodGet, "/v1/agent/conversations/"+conv+"/shares", w, nil); status != http.StatusNotFound && status != http.StatusForbidden {
			t.Fatalf("%q listed a userless thread's links: %d", w.user, status)
		}
	}
}

// An assistant turn a caller wrote through the record endpoint reaches a reader
// with no model: it is the sharer's text, not an answer this server received
// from a model. A turn the round stored carries the model that produced it.
// (Regression: a recorded turn was shared as indistinguishable from an answer.)
func TestARecordedAssistantTurnIsNotAnAnswer(t *testing.T) {
	completer := &stubCompleter{resp: openai.ChatCompletionResponse{
		Model:   "zen-4",
		Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{Role: "assistant", Content: "the model's answer"}}},
	}}
	app, _, _ := shareAppWith(t, t.TempDir(), completer)
	conv := recordTurns(t, app, "acme", "mallory", "", inMessage{"user", "is this legit?"}, inMessage{"assistant", "FORGED: written by the sharer"})
	status, raw := call(t, app, http.MethodPost, "/v1/agent", who{org: "acme", user: "mallory"}, map[string]any{
		"preset": "graph", "conversationId": conv, "messages": []inMessage{{"user", "and now?"}},
	})
	if status != http.StatusOK {
		t.Fatalf("round: %d %s", status, raw)
	}
	s := share(t, app, "acme", "mallory", conv)
	_, got, _ := openAs(t, app, zed, s.Token)
	if len(got.Messages) != 4 {
		t.Fatalf("shared turns = %+v", got.Messages)
	}
	forged, answered := got.Messages[1], got.Messages[3]
	if forged.Role != "assistant" || forged.Model != "" || forged.Content != "FORGED: written by the sharer" {
		t.Fatalf("the recorded turn reached the reader as %+v", forged)
	}
	if answered.Role != "assistant" || answered.Model != "zen-4" || answered.Content != "the model's answer" {
		t.Fatalf("the round's turn reached the reader as %+v", answered)
	}
	if _, raw := as(t, app, http.MethodGet, "/v1/agent/conversations/"+conv, "acme", "mallory", nil); !strings.Contains(string(raw), `"model":"zen-4"`) {
		t.Fatalf("the owner's own read does not carry the model: %s", raw)
	}
}

// An org's admin lists every live link in the org — all of a member's with
// ?user= — and revokes any of them, which ends it for its viewers. A member who
// is not an admin cannot, and neither can another org's admin.
func TestAnOrgAdminManagesTheOrgsLinks(t *testing.T) {
	app, trail := shareApp(t, t.TempDir())
	ac := recordTurns(t, app, "acme", "alice", "", inMessage{"user", "alice asks"}, inMessage{"assistant", "a"})
	bc := recordTurns(t, app, "acme", "bob", "", inMessage{"user", "bob asks"}, inMessage{"assistant", "b"})
	as1 := share(t, app, "acme", "alice", ac)
	bs := share(t, app, "acme", "bob", bc)
	openAs(t, app, zed, bs.Token)

	list := func(w who, q string) (int, []orgShareOut) {
		status, raw := call(t, app, http.MethodGet, "/v1/agent/shares"+q, w, nil)
		var out struct {
			Shares []orgShareOut `json:"shares"`
		}
		_ = json.Unmarshal(raw, &out)
		return status, out.Shares
	}
	if status, got := list(boss, ""); status != http.StatusOK || len(got) != 2 {
		t.Fatalf("admin lists: %d %+v", status, got)
	}
	status, got := list(boss, "?user=bob")
	if status != http.StatusOK || len(got) != 1 || got[0].ID != bs.Share.ID || got[0].Title != "bob asks" || got[0].Viewers != 1 {
		t.Fatalf("admin lists bob's: %d %+v", status, got)
	}
	for _, w := range []who{alice, bob} {
		if status, _ := list(w, ""); status != http.StatusForbidden {
			t.Fatalf("member %s listed the org's links: %d", w.user, status)
		}
		if status, _ := call(t, app, http.MethodDelete, "/v1/agent/shares/"+bs.Share.ID, w, nil); status != http.StatusForbidden {
			t.Fatalf("member %s revoked through the admin route: %d", w.user, status)
		}
	}
	if status, got := list(rival, ""); status != http.StatusOK || len(got) != 0 {
		t.Fatalf("another org's admin lists: %d %+v", status, got)
	}
	if status, _ := call(t, app, http.MethodDelete, "/v1/agent/shares/"+bs.Share.ID, rival, nil); status != http.StatusNotFound {
		t.Fatalf("another org's admin revoked: %d", status)
	}
	if status, raw := call(t, app, http.MethodDelete, "/v1/agent/shares/"+bs.Share.ID, boss, nil); status != http.StatusOK {
		t.Fatalf("admin revokes bob's link: %d %s", status, raw)
	}
	if status, _, _ := openAs(t, app, zed, bs.Token); status != http.StatusNotFound {
		t.Fatalf("bob's link opens after the admin revoked it: %d", status)
	}
	if got := sharedWith(t, app, zed); len(got) != 0 {
		t.Fatalf("zed keeps a link the admin revoked: %+v", got)
	}
	if status, _, _ := openAs(t, app, zed, as1.Token); status != http.StatusOK {
		t.Fatalf("alice's link stopped when bob's was revoked: %d", status)
	}
	if last := (*trail)[len(*trail)-1]; last != (audited{"agent.share.revoke", bc, bs.Share.ID}) {
		t.Fatalf("the admin's revoke was not audited: %+v", *trail)
	}
}
