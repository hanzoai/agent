// Package agent is a reusable agentic-chat orchestrator: one POST runs a single
// LLM tool-calling round that lets a model manage a system through tools, and it
// PERSISTS conversation history (per-org SQLite via hanzoai/orm). It is a
// library — a host (hanzoai/ai / hanzoai/cloud) mounts it on its OWN zip router so
// the round registers in the SAME router as /v1/chat/completions (a distinct path,
// no route-precedence gamble), and injects the two dependency seams:
//
//   - Completer — the host's in-process LLM completion (the ONLY path that both
//     returns tool_calls and carries per-org billing);
//   - ToolPlane — the host's unified tool registry (list / exists / dispatch).
//
// A round yields four things: the assistant's text (reply), the tool calls the
// server executed against the registry (actions), the tool calls the client must
// apply itself — a graph/UI mutation the server cannot run (ops), and the id of
// the conversation the turn was appended to (conversationId).
//
// This package DELIBERATELY does not import hanzoai/cloud or hanzoai/ai — that
// coupling is the thing it exists to break. It builds directly on zip (routing),
// hanzoai/orm (per-org SQLite persistence) and go-openai (the request/response
// shapes); the host wraps Mount with a thin adapter that supplies the seams.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	openai "github.com/hanzoai/go-openai"
	luxlog "github.com/luxfi/log"
	"github.com/zap-proto/zip"
)

// Completer runs one chat completion (with tools) and returns the parsed
// response. It is the seam onto the host's in-process LLM path: the real impl
// replays the request carrying the caller's credential (so the host's per-org
// billing runs); tests inject a fake so the round is exercised without a live
// model. agent never imports the package that provides the real Completer.
type Completer interface {
	Complete(ctx context.Context, cred map[string]string, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
}

// UpstreamError is returned by a Completer when the completion service refuses the
// request for the CALLER's OWN reason — a 4xx like 402 insufficient_balance, 429
// rate-limit, or 403 — that must reach the caller intact, not be masked as a
// gateway 502. Body is the completion's verbatim error payload; the round passes
// Status + Body straight through so a client shows the real message ("add credits")
// instead of an opaque "agent: completion" wrapper. A non-4xx completion failure
// (5xx, transport) stays a 502 — that IS a gateway fault.
type UpstreamError struct {
	Status int
	Body   []byte
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("upstream completion %d: %s", e.Status, e.Body)
}

// Tool is the agent-facing projection of one registered tool offered to the
// model. Schema is the JSON-Schema of the call arguments; Dispatchable is false
// for a listing-only entry; Activated is true only for tools the org turned on.
type Tool struct {
	Name         string
	Description  string
	Schema       json.RawMessage
	Activated    bool
	Dispatchable bool
}

// Scope is the (org, project) a tool listing is resolved for.
type Scope struct {
	Org     string
	Project string
}

// ToolPlane is the org's registered tool registry seam: list the tools offered to
// the model, test whether a call is server-known, and dispatch a call. Dispatch
// takes the live request so the host resolves the caller the ONE canonical way (its
// PrincipalFrom) — the credential is never reconstructed or passed as a value, only
// read from the validated request. List/Exists take a plain (org, project) Scope;
// that carries no credential, so it is safe by value. The host injects its unified
// tool plane; tests inject a stub.
type ToolPlane interface {
	List(ctx context.Context, scope Scope) []Tool
	Exists(ctx context.Context, scope Scope, name string) bool
	Dispatch(c *zip.Ctx, name string, args map[string]any) (any, error)
}

// Principal is the VALIDATED caller a round runs as. Org scopes persistence AND
// the tool listing (never a client-supplied field); Cred is the caller's own
// credential headers, opaque to agent, replayed by the injected Completer /
// ToolPlane so an in-process call carries exactly the caller's identity.
type Principal struct {
	Org     string
	Project string
	User    string
	Cred    map[string]string
}

// Deps is agent's OWN small dependency surface — deliberately not cloud.Deps. It
// carries only what this package needs: a logger, the per-org SQLite data root,
// a brand tag, the default served model, and an optional principal resolver
// (defaults to gateway-header identity when nil).
type Deps struct {
	// Logger is the canonical Hanzo logger (luxfi/log). Required.
	Logger luxlog.Logger
	// DataDir is the per-deployment data root. Per-org SQLite files land at
	// {DataDir}/orgs/{orgSlug}/agent.db. Required.
	DataDir string
	// Brand is the white-label brand identifier (logged only).
	Brand string
	// Model is the served model used when a request supplies none.
	Model string
	// Principal resolves the validated caller from a request. Defaults to
	// header-based resolution (X-Org-Id / X-User-Id) when nil.
	Principal func(*zip.Ctx) (Principal, bool)
}

// Service is the mounted orchestrator handle. Its handlers are plain zip
// handlers; Close releases the per-org SQLite stores at host shutdown.
type Service struct {
	log       luxlog.Logger
	model     string
	store     *store
	completer Completer
	plane     ToolPlane
	principal func(*zip.Ctx) (Principal, bool)
}

// DefaultPrefix is where the standalone daemon answers. A host whose own router
// already spends that address folds this surface somewhere else with MountAt;
// nothing in the round depends on which address it was given.
const DefaultPrefix = "/v1/agent"

// Mount registers the routes at DefaultPrefix. See MountAt.
func Mount(app *zip.App, deps Deps, completer Completer, plane ToolPlane) (*Service, error) {
	return MountAt(app, DefaultPrefix, deps, completer, plane)
}

// MountAt registers the routes under prefix using the injected Completer and
// ToolPlane, and returns the Service so the host can Close it on shutdown. The
// per-org SQLite models are auto-migrated (schema created) on first per-org open.
// prefix is an absolute path chosen by whoever composes the router:
//
//	POST {prefix}                     — run one tool-calling round
//	GET  {prefix}/presets             — list the preset library
//	GET  {prefix}/conversations       — list the caller-org's conversations
//	GET  {prefix}/conversations/:id   — one conversation's messages
func MountAt(app *zip.App, prefix string, deps Deps, completer Completer, plane ToolPlane) (*Service, error) {
	if app == nil {
		return nil, fmt.Errorf("agent.Mount: nil zip.App")
	}
	if luxlog.Default() == nil {
		return nil, fmt.Errorf("agent.Mount: nil luxlog.Default()")
	}
	if completer == nil {
		return nil, fmt.Errorf("agent.Mount: nil Completer")
	}
	if plane == nil {
		return nil, fmt.Errorf("agent.Mount: nil ToolPlane")
	}
	if strings.TrimSpace(deps.DataDir) == "" {
		return nil, fmt.Errorf("agent.Mount: empty DataDir")
	}
	prefix = strings.TrimRight(strings.TrimSpace(prefix), "/")
	if !strings.HasPrefix(prefix, "/") {
		return nil, fmt.Errorf("agent.Mount: prefix %q is not an absolute path", prefix)
	}
	resolve := deps.Principal
	if resolve == nil {
		resolve = headerPrincipal
	}
	s := &Service{
		log:       luxlog.Default().New("subsystem", "agent"),
		model:     strings.TrimSpace(deps.Model),
		store:     newStore(deps.DataDir),
		completer: completer,
		plane:     plane,
		principal: resolve,
	}
	app.Post(prefix, s.handleRun)
	app.Get(prefix+"/presets", s.handlePresets)
	app.Get(prefix+"/conversations", s.handleListConversations)
	app.Get(prefix+"/conversations/:id", s.handleConversation)
	s.log.Info("agent mounted", "route", prefix, "presets", len(presets), "brand", deps.Brand)
	return s, nil
}

// Close releases every open per-org store. Idempotent.
func (s *Service) Close() error {
	if s == nil || s.store == nil {
		return nil
	}
	return s.store.closeAll()
}

// caller resolves the validated principal or returns a 403 error. Org is the sole
// authority for persistence + tool scope; a request without a non-empty org never
// runs a round.
func (s *Service) caller(c *zip.Ctx) (Principal, error) {
	p, ok := s.principal(c)
	if !ok || strings.TrimSpace(p.Org) == "" {
		return Principal{}, zip.ErrForbidden("a validated principal is required")
	}
	return p, nil
}

// credentialHeaders are the caller's own credential headers captured for replay
// by the injected Completer / ToolPlane, so an in-process call runs under the SAME
// identity check as a direct HTTP call. NO minted authority header (X-Org-Id/…) is
// captured — those are gateway-minted from a re-validated credential.
var credentialHeaders = []string{"Authorization", "X-Authorization", "Cookie", "Accept-Language", "X-Forwarded-For"}

// headerPrincipal is the default resolver: it reads the gateway-minted,
// ingress-validated identity headers (X-Org-Id / X-User-Id) that zip.Ctx exposes
// and captures the caller's credential headers for replay. A host with a richer
// identity middleware injects its own Deps.Principal (e.g. wrapping the host's
// PrincipalFrom so Project and the validated credential flow through).
func headerPrincipal(c *zip.Ctx) (Principal, bool) {
	org := strings.TrimSpace(c.Org())
	if org == "" {
		return Principal{}, false
	}
	cred := map[string]string{}
	for _, h := range credentialHeaders {
		if v := c.Header(h); v != "" {
			cred[h] = v
		}
	}
	return Principal{Org: org, User: c.User(), Cred: cred}, true
}
