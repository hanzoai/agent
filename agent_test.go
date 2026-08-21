package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	openai "github.com/hanzoai/go-openai"
	luxlog "github.com/luxfi/log"
	fiber "github.com/zap-proto/fiber/v3"
	"github.com/zap-proto/zip"
)

// ── stubbed dependency seams ─────────────────────────────────────────────────────

// stubCompleter returns a canned completion and records the last request seen.
type stubCompleter struct {
	resp    openai.ChatCompletionResponse
	lastReq openai.ChatCompletionRequest
	err     error
}

func (s *stubCompleter) Complete(_ context.Context, _ map[string]string, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	s.lastReq = req
	return s.resp, s.err
}

// stubPlane is an injectable ToolPlane: a known-set predicate, a dispatch recorder,
// and a fixed listing (empty by default so a round never depends on a real plane).
type stubPlane struct {
	known      map[string]bool
	tools      []Tool
	dispatched []string
}

func (p *stubPlane) List(context.Context, Scope) []Tool { return p.tools }
func (p *stubPlane) Exists(_ context.Context, _ Scope, name string) bool {
	return p.known[name]
}
func (p *stubPlane) Dispatch(_ *zip.Ctx, name string, _ map[string]any) (any, error) {
	p.dispatched = append(p.dispatched, name)
	return map[string]any{"ok": true, "tool": name}, nil
}

// ── harness ──────────────────────────────────────────────────────────────────────

func newApp(t *testing.T, comp Completer, plane ToolPlane) *zip.App {
	t.Helper()
	app := zip.New(zip.Config{Logger: luxlog.New("test")})
	svc, err := Mount(app, Deps{Logger: luxlog.New("test"), DataDir: t.TempDir(), Model: "zen"}, comp, plane)
	if err != nil {
		t.Fatalf("Mount: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return app
}

func newAppAt(t *testing.T, prefix string, comp Completer, plane ToolPlane) *zip.App {
	t.Helper()
	app := zip.New(zip.Config{Logger: luxlog.New("test")})
	svc, err := MountAt(app, prefix, Deps{Logger: luxlog.New("test"), DataDir: t.TempDir(), Model: "zen"}, comp, plane)
	if err != nil {
		t.Fatalf("MountAt %s: %v", prefix, err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return app
}

// do issues one request. org != "" sets the validated identity headers exactly as
// the gateway would; org == "" sends none — the anonymous path the 403 test needs.
func do(t *testing.T, app *zip.App, method, path, org string, body any) (int, []byte) {
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
		rq.Header.Set("X-User-Id", "u-"+org)
	}
	resp, err := app.Fiber().Test(rq, fiber.TestConfig{Timeout: 0})
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

func completion(content string, toolCalls ...string) openai.ChatCompletionResponse {
	msg := openai.ChatCompletionMessage{Role: openai.ChatMessageRoleAssistant, Content: content}
	for i, name := range toolCalls {
		msg.ToolCalls = append(msg.ToolCalls, openai.ToolCall{
			ID:       fmt.Sprintf("call_%d", i),
			Type:     openai.ToolTypeFunction,
			Function: openai.FunctionCall{Name: name, Arguments: `{"x":1}`},
		})
	}
	return openai.ChatCompletionResponse{Choices: []openai.ChatCompletionChoice{{Message: msg}}}
}

func decodeRun(t *testing.T, raw []byte) runResponse {
	t.Helper()
	var out runResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode run response: %v (%s)", err, raw)
	}
	return out
}

type apiMsg struct {
	Role      string          `json:"role"`
	Content   string          `json:"content"`
	ToolCalls json.RawMessage `json:"toolCalls"`
}

func getMessages(t *testing.T, app *zip.App, org, id string) []apiMsg {
	t.Helper()
	status, raw := do(t, app, http.MethodGet, "/v1/agent/conversations/"+id, org, nil)
	if status != http.StatusOK {
		t.Fatalf("get messages: %d %s", status, raw)
	}
	var body struct {
		Messages []apiMsg `json:"messages"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode messages: %v", err)
	}
	return body.Messages
}

func getConversations(t *testing.T, app *zip.App, org string) []convSummary {
	t.Helper()
	status, raw := do(t, app, http.MethodGet, "/v1/agent/conversations", org, nil)
	if status != http.StatusOK {
		t.Fatalf("get conversations: %d %s", status, raw)
	}
	var body struct {
		Conversations []convSummary `json:"conversations"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode conversations: %v", err)
	}
	return body.Conversations
}

func names[T any](xs []T, name func(T) string) []string {
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		out = append(out, name(x))
	}
	sort.Strings(out)
	return out
}

func eq(a, b []string) bool {
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

// ── the tool-calling round split ─────────────────────────────────────────────────

// TestAgentRound is the table test for the round: a registry tool call under a
// server-executed preset dispatches to actions; an unknown call falls to ops;
// plain content is the reply; a graph-preset call is always an op (advisory).
func TestAgentRound(t *testing.T) {
	cases := []struct {
		name           string
		preset         string
		completion     openai.ChatCompletionResponse
		known          map[string]bool
		wantReply      string
		wantActions    []string
		wantOps        []string
		wantDispatched []string
	}{
		{
			name:           "known registry tool dispatched to actions",
			preset:         "create",
			completion:     completion("", "render_image"),
			known:          map[string]bool{"render_image": true},
			wantActions:    []string{"render_image"},
			wantOps:        []string{},
			wantDispatched: []string{"render_image"},
		},
		{
			name:           "unknown tool falls to ops",
			preset:         "create",
			completion:     completion("", "frobnicate"),
			known:          map[string]bool{},
			wantActions:    []string{},
			wantOps:        []string{"frobnicate"},
			wantDispatched: []string{},
		},
		{
			name:           "plain content is the reply",
			preset:         "graph",
			completion:     completion("here is your graph plan"),
			known:          map[string]bool{},
			wantReply:      "here is your graph plan",
			wantActions:    []string{},
			wantOps:        []string{},
			wantDispatched: []string{},
		},
		{
			name:           "graph preset calls are advisory ops, never dispatched",
			preset:         "graph",
			completion:     completion("", "add_node"),
			known:          map[string]bool{"add_node": true}, // known, but graph never dispatches
			wantActions:    []string{},
			wantOps:        []string{"add_node"},
			wantDispatched: []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plane := &stubPlane{known: tc.known}
			app := newApp(t, &stubCompleter{resp: tc.completion}, plane)

			status, raw := do(t, app, http.MethodPost, "/v1/agent", "acme", map[string]any{
				"preset":   tc.preset,
				"messages": []inMessage{{Role: "user", Content: "do the thing"}},
			})
			if status != http.StatusOK {
				t.Fatalf("status: want 200, got %d (%s)", status, raw)
			}
			out := decodeRun(t, raw)
			if out.Reply != tc.wantReply {
				t.Fatalf("reply: want %q, got %q", tc.wantReply, out.Reply)
			}
			if out.ConversationID == "" {
				t.Fatal("response carried no conversationId")
			}
			gotActions := names(out.Actions, func(a action) string { return a.Name })
			if !eq(gotActions, tc.wantActions) {
				t.Fatalf("actions: want %v, got %v", tc.wantActions, gotActions)
			}
			gotOps := names(out.Ops, func(o op) string { return o.Name })
			if !eq(gotOps, tc.wantOps) {
				t.Fatalf("ops: want %v, got %v", tc.wantOps, gotOps)
			}
			gotDispatched := append([]string{}, plane.dispatched...)
			sort.Strings(gotDispatched)
			if !eq(gotDispatched, tc.wantDispatched) {
				t.Fatalf("dispatched: want %v, got %v", tc.wantDispatched, gotDispatched)
			}
			for _, a := range out.Actions {
				if a.Error != "" || a.Result == nil {
					t.Fatalf("action %s should carry a result, got %+v", a.Name, a)
				}
			}
		})
	}
}

// TestToolCallPersisted proves a dispatched-tool round writes a user message and
// an assistant message that carries the model's tool_calls.
func TestToolCallPersisted(t *testing.T) {
	plane := &stubPlane{known: map[string]bool{"render_image": true}}
	app := newApp(t, &stubCompleter{resp: completion("", "render_image")}, plane)

	status, raw := do(t, app, http.MethodPost, "/v1/agent", "acme", map[string]any{
		"preset":   "create",
		"messages": []inMessage{{Role: "user", Content: "make a shoe"}},
	})
	if status != http.StatusOK {
		t.Fatalf("status: want 200, got %d (%s)", status, raw)
	}
	out := decodeRun(t, raw)

	msgs := getMessages(t, app, "acme", out.ConversationID)
	if len(msgs) != 2 {
		t.Fatalf("want 2 persisted messages, got %d: %+v", len(msgs), msgs)
	}
	if msgs[0].Role != openai.ChatMessageRoleUser || msgs[0].Content != "make a shoe" {
		t.Fatalf("first message should be the user turn, got %+v", msgs[0])
	}
	if msgs[1].Role != openai.ChatMessageRoleAssistant {
		t.Fatalf("second message should be the assistant turn, got %+v", msgs[1])
	}
	if len(msgs[1].ToolCalls) == 0 {
		t.Fatal("assistant tool_calls were not persisted")
	}
}

// TestPersistedHistory proves plain content is written AND a second POST carrying
// the returned conversationId appends to the SAME conversation (history works).
func TestPersistedHistory(t *testing.T) {
	comp := &stubCompleter{resp: completion("first reply")}
	app := newApp(t, comp, &stubPlane{known: map[string]bool{}})

	status, raw := do(t, app, http.MethodPost, "/v1/agent", "acme", map[string]any{
		"preset":   "graph",
		"messages": []inMessage{{Role: "user", Content: "hello"}},
	})
	if status != http.StatusOK {
		t.Fatalf("post 1: %d %s", status, raw)
	}
	r1 := decodeRun(t, raw)
	if r1.Reply != "first reply" || r1.ConversationID == "" {
		t.Fatalf("unexpected first response: %+v", r1)
	}

	if msgs := getMessages(t, app, "acme", r1.ConversationID); len(msgs) != 2 {
		t.Fatalf("after turn 1 want 2 messages, got %d", len(msgs))
	}

	comp.resp = completion("second reply")
	status, raw = do(t, app, http.MethodPost, "/v1/agent", "acme", map[string]any{
		"preset":         "graph",
		"conversationId": r1.ConversationID,
		"messages":       []inMessage{{Role: "user", Content: "again"}},
	})
	if status != http.StatusOK {
		t.Fatalf("post 2: %d %s", status, raw)
	}
	r2 := decodeRun(t, raw)
	if r2.ConversationID != r1.ConversationID {
		t.Fatalf("second turn opened a new conversation: %s != %s", r2.ConversationID, r1.ConversationID)
	}

	msgs := getMessages(t, app, "acme", r1.ConversationID)
	if len(msgs) != 4 {
		t.Fatalf("after 2 turns want 4 messages, got %d: %+v", len(msgs), msgs)
	}
	if msgs[0].Content != "hello" || msgs[2].Content != "again" {
		t.Fatalf("history is out of order: %+v", msgs)
	}

	if convs := getConversations(t, app, "acme"); len(convs) != 1 {
		t.Fatalf("want exactly 1 conversation for the org, got %d", len(convs))
	}
}

// TestConversationsAreOrgScoped proves one org never sees another's conversations.
func TestConversationsAreOrgScoped(t *testing.T) {
	app := newApp(t, &stubCompleter{resp: completion("ok")}, &stubPlane{known: map[string]bool{}})

	if status, raw := do(t, app, http.MethodPost, "/v1/agent", "acme", map[string]any{
		"messages": []inMessage{{Role: "user", Content: "acme thread"}},
	}); status != http.StatusOK {
		t.Fatalf("acme post: %d %s", status, raw)
	}

	if got := getConversations(t, app, "acme"); len(got) != 1 {
		t.Fatalf("acme should see its 1 conversation, got %d", len(got))
	}
	if got := getConversations(t, app, "globex"); len(got) != 0 {
		t.Fatalf("globex must see none of acme's conversations, got %d", len(got))
	}
}

// TestUnknownPreset: an unknown preset id is 400 before any completion.
func TestUnknownPreset(t *testing.T) {
	comp := &stubCompleter{resp: completion("must not run")}
	app := newApp(t, comp, &stubPlane{known: map[string]bool{}})
	status, _ := do(t, app, http.MethodPost, "/v1/agent", "acme", map[string]any{
		"preset":   "bogus",
		"messages": []inMessage{{Role: "user", Content: "hi"}},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("unknown preset: want 400, got %d", status)
	}
}

// TestRequiresPrincipal: a request with no validated identity is refused 403 and
// never runs a completion.
func TestRequiresPrincipal(t *testing.T) {
	comp := &stubCompleter{resp: completion("must not run")}
	app := newApp(t, comp, &stubPlane{known: map[string]bool{}})
	status, _ := do(t, app, http.MethodPost, "/v1/agent", "", map[string]any{
		"preset":   "graph",
		"messages": []inMessage{{Role: "user", Content: "hi"}},
	})
	if status != http.StatusForbidden {
		t.Fatalf("no principal: want 403, got %d", status)
	}
}

// TestRequiresMessages: an empty messages array is 400.
func TestRequiresMessages(t *testing.T) {
	app := newApp(t, &stubCompleter{resp: completion("x")}, &stubPlane{known: map[string]bool{}})
	status, _ := do(t, app, http.MethodPost, "/v1/agent", "acme", map[string]any{
		"preset":   "graph",
		"messages": []inMessage{},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("empty messages: want 400, got %d", status)
	}
}

// TestUpstreamBillingPassthrough: a completion refused for the caller's own reason
// (402 insufficient_balance) reaches the caller VERBATIM — status + body — not a 502.
func TestUpstreamBillingPassthrough(t *testing.T) {
	billing := []byte(`{"error":{"message":"Insufficient balance. Add credits.","code":"insufficient_balance"}}`)
	comp := &stubCompleter{err: &UpstreamError{Status: http.StatusPaymentRequired, Body: billing}}
	app := newApp(t, comp, &stubPlane{known: map[string]bool{}})
	status, raw := do(t, app, http.MethodPost, "/v1/agent", "acme", map[string]any{
		"preset":   "graph",
		"messages": []inMessage{{Role: "user", Content: "hi"}},
	})
	if status != http.StatusPaymentRequired {
		t.Fatalf("billing refusal: want 402 passthrough, got %d: %s", status, raw)
	}
	if !bytes.Equal(raw, billing) {
		t.Fatalf("billing body not verbatim: got %s", raw)
	}
}

// TestUpstreamServerErrorIs502: a non-4xx upstream failure (5xx / transport) IS a
// gateway fault and stays 502 — only a caller-facing 4xx passes through.
func TestUpstreamServerErrorIs502(t *testing.T) {
	comp := &stubCompleter{err: &UpstreamError{Status: http.StatusServiceUnavailable, Body: []byte("upstream down")}}
	app := newApp(t, comp, &stubPlane{known: map[string]bool{}})
	status, _ := do(t, app, http.MethodPost, "/v1/agent", "acme", map[string]any{
		"preset":   "graph",
		"messages": []inMessage{{Role: "user", Content: "hi"}},
	})
	if status != http.StatusBadGateway {
		t.Fatalf("5xx upstream: want 502, got %d", status)
	}
}

// TestPresetsLibrary: the preset library is listed and seeded with graph + create.
func TestPresetsLibrary(t *testing.T) {
	app := newApp(t, &stubCompleter{resp: completion("x")}, &stubPlane{known: map[string]bool{}})
	status, raw := do(t, app, http.MethodGet, "/v1/agent/presets", "acme", nil)
	if status != http.StatusOK {
		t.Fatalf("presets: %d %s", status, raw)
	}
	var body struct {
		Presets []Preset `json:"presets"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode presets: %v", err)
	}
	found := map[string]bool{}
	for _, p := range body.Presets {
		found[p.ID] = true
	}
	if !found["graph"] || !found["create"] {
		t.Fatalf("preset library missing seeds: %+v", body.Presets)
	}
}

// ── the address ──────────────────────────────────────────────────────────────────

// TestFoldedPrefix: a host whose own router already spends /v1/agent gets the whole
// surface — round, presets, threads — under an address it chooses, and gets nothing
// at the default it did not ask for.
func TestFoldedPrefix(t *testing.T) {
	const prefix = "/v1/agents/chat"
	app := newAppAt(t, prefix, &stubCompleter{resp: completion("folded")}, &stubPlane{known: map[string]bool{}})

	status, raw := do(t, app, http.MethodPost, prefix, "acme", map[string]any{
		"messages": []inMessage{{Role: "user", Content: "hi"}},
	})
	if status != http.StatusOK {
		t.Fatalf("round at %s: %d %s", prefix, status, raw)
	}
	out := decodeRun(t, raw)
	if out.Reply != "folded" {
		t.Fatalf("reply: want folded, got %q", out.Reply)
	}
	if out.ConversationID == "" {
		t.Fatal("round persisted no conversation")
	}
	for _, path := range []string{prefix + "/presets", prefix + "/conversations", prefix + "/conversations/" + out.ConversationID} {
		if status, raw := do(t, app, http.MethodGet, path, "acme", nil); status != http.StatusOK {
			t.Fatalf("GET %s: %d %s", path, status, raw)
		}
	}
	if status, _ := do(t, app, http.MethodPost, DefaultPrefix, "acme", map[string]any{
		"messages": []inMessage{{Role: "user", Content: "hi"}},
	}); status != http.StatusNotFound {
		t.Fatalf("folded mount still answers %s: %d", DefaultPrefix, status)
	}
}

// TestPrefixMustBeAbsolute: a prefix that is not a path is refused at mount rather
// than registering routes nothing can reach.
func TestPrefixMustBeAbsolute(t *testing.T) {
	for _, prefix := range []string{"", "/", "v1/agents/chat"} {
		app := zip.New(zip.Config{Logger: luxlog.New("test")})
		svc, err := MountAt(app, prefix, Deps{Logger: luxlog.New("test"), DataDir: t.TempDir()}, &stubCompleter{}, &stubPlane{})
		if err == nil {
			_ = svc.Close()
			t.Fatalf("MountAt(%q): want error, got none", prefix)
		}
	}
}

// TestTrailingSlashIsTheSameAddress: /v1/agents/chat/ and /v1/agents/chat mount the
// same four routes, so a host cannot end up with //presets.
func TestTrailingSlashIsTheSameAddress(t *testing.T) {
	app := newAppAt(t, "/v1/agents/chat/", &stubCompleter{resp: completion("x")}, &stubPlane{known: map[string]bool{}})
	if status, raw := do(t, app, http.MethodGet, "/v1/agents/chat/presets", "acme", nil); status != http.StatusOK {
		t.Fatalf("presets: %d %s", status, raw)
	}
}
