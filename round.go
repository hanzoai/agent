package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"github.com/zap-proto/zip"
)

// ── request / response ──────────────────────────────────────────────────────────

type inMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// runRequest is the POST /v1/agent body. Preset is the primary selector;
// Capability is a legacy alias for the same field. Org is NEVER read from the
// body — it comes only from the validated principal.
type runRequest struct {
	Messages       []inMessage   `json:"messages"`
	Preset         string        `json:"preset"`
	Capability     string        `json:"capability"`
	Tools          []openai.Tool `json:"tools"`
	System         string        `json:"system"`
	ConversationID string        `json:"conversationId"`
}

// action is one tool call the server executed against the registry.
type action struct {
	Name   string         `json:"name"`
	Args   map[string]any `json:"args,omitempty"`
	Result any            `json:"result,omitempty"`
	Error  string         `json:"error,omitempty"`
}

// op is one tool call the client must apply itself (a graph/UI mutation).
type op struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

type runResponse struct {
	Reply          string   `json:"reply"`
	Actions        []action `json:"actions"`
	Ops            []op     `json:"ops"`
	ConversationID string   `json:"conversationId"`
}

// ── run handler ─────────────────────────────────────────────────────────────────

// handleRun runs one tool-calling round: resolve the caller, load-or-create the
// conversation (scoped to the caller's org), persist the incoming user turn, call
// the injected completion, split the model's tool calls into server-executed
// actions vs client-applied ops, persist the assistant reply, and return the
// four-part result plus the conversation id.
func (s *Service) handleRun(c *zip.Ctx) error {
	p, err := s.caller(c)
	if err != nil {
		return err
	}
	var body runRequest
	if err := c.Bind(&body); err != nil {
		return err
	}
	if len(body.Messages) == 0 {
		return zip.ErrBadRequest("messages required")
	}
	presetID := strings.TrimSpace(body.Preset)
	if presetID == "" {
		presetID = strings.TrimSpace(body.Capability)
	}
	mode, ok := presetFor(presetID)
	if !ok {
		return zip.ErrBadRequest("unknown preset: " + presetID)
	}

	ctx := c.Context()

	conv, err := s.store.loadOrCreateConversation(ctx, p.Org, body.ConversationID, firstUserContent(body.Messages))
	if err != nil {
		return zip.Errorf(http.StatusInternalServerError, "agent: conversation: %v", err)
	}

	if last := lastUserMessage(body.Messages); last != "" {
		if _, err := s.store.appendMessage(ctx, p.Org, conv.Id(), openai.ChatMessageRoleUser, last, nil); err != nil {
			return zip.Errorf(http.StatusInternalServerError, "agent: persist user: %v", err)
		}
	}

	scope := Scope{Org: p.Org, Project: p.Project}
	req := openai.ChatCompletionRequest{
		Model:    s.model,
		Messages: assembleMessages(mode, body),
		Tools:    s.assembleTools(ctx, mode, body.Tools, scope),
	}

	resp, err := s.completer.Complete(ctx, p.Cred, req)
	if err != nil {
		// A completion refused for the caller's OWN reason (402 insufficient_balance,
		// 429 rate-limit, 403) is the caller's error, not a gateway fault — pass its
		// status + body through verbatim so the client shows the real billing message,
		// never an opaque 502. Any other failure (5xx, transport) IS a gateway fault.
		var up *UpstreamError
		if errors.As(err, &up) && up.Status >= 400 && up.Status < 500 {
			c.SetHeader("Content-Type", "application/json")
			return c.Bytes(up.Status, up.Body)
		}
		return zip.Errorf(http.StatusBadGateway, "agent: completion: %v", err)
	}
	if len(resp.Choices) == 0 {
		return zip.Errorf(http.StatusBadGateway, "agent: completion returned no choices")
	}
	msg := resp.Choices[0].Message

	out := runResponse{Reply: msg.Content, Actions: []action{}, Ops: []op{}, ConversationID: conv.Id()}
	for _, tc := range msg.ToolCalls {
		name := tc.Function.Name
		args := decodeArgs(tc.Function.Arguments)
		if mode.ServerExecuted && s.plane.Exists(ctx, scope, name) {
			a := action{Name: name, Args: args}
			if result, derr := s.plane.Dispatch(c, name, args); derr != nil {
				a.Error = derr.Error()
			} else {
				a.Result = result
			}
			out.Actions = append(out.Actions, a)
			continue
		}
		out.Ops = append(out.Ops, op{Name: name, Args: args})
	}

	if err := s.persistAssistant(ctx, p.Org, conv, msg); err != nil {
		return zip.Errorf(http.StatusInternalServerError, "agent: persist assistant: %v", err)
	}
	return c.JSON(http.StatusOK, out)
}

// persistAssistant appends the assistant reply (with its tool calls, if any) and
// bumps the conversation's UpdatedAt so a listing surfaces recent threads first.
func (s *Service) persistAssistant(ctx context.Context, org string, conv *Conversation, msg openai.ChatCompletionMessage) error {
	var toolCalls json.RawMessage
	if len(msg.ToolCalls) > 0 {
		b, err := json.Marshal(msg.ToolCalls)
		if err != nil {
			return err
		}
		toolCalls = b
	}
	if _, err := s.store.appendMessage(ctx, org, conv.Id(), openai.ChatMessageRoleAssistant, msg.Content, toolCalls); err != nil {
		return err
	}
	return conv.UpdateCtx(ctx)
}

// ── read handlers ────────────────────────────────────────────────────────────────

func (s *Service) handlePresets(c *zip.Ctx) error {
	return c.JSON(http.StatusOK, map[string]any{"presets": Presets()})
}

type convSummary struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (s *Service) handleListConversations(c *zip.Ctx) error {
	p, err := s.caller(c)
	if err != nil {
		return err
	}
	items, err := s.store.listConversations(c.Context(), p.Org)
	if err != nil {
		return zip.Errorf(http.StatusInternalServerError, "agent: list: %v", err)
	}
	out := make([]convSummary, 0, len(items))
	for _, cv := range items {
		out = append(out, convSummary{ID: cv.Id(), Title: cv.Title, UpdatedAt: cv.UpdatedAt})
	}
	return c.JSON(http.StatusOK, map[string]any{"conversations": out})
}

type msgOut struct {
	ID        string          `json:"id"`
	Role      string          `json:"role"`
	Content   string          `json:"content"`
	ToolCalls json.RawMessage `json:"toolCalls,omitempty"`
	CreatedAt time.Time       `json:"createdAt"`
}

func (s *Service) handleConversation(c *zip.Ctx) error {
	p, err := s.caller(c)
	if err != nil {
		return err
	}
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return zip.ErrBadRequest("conversation id required")
	}
	msgs, err := s.store.conversationMessages(c.Context(), p.Org, id)
	if err != nil {
		return zip.Errorf(http.StatusInternalServerError, "agent: messages: %v", err)
	}
	out := make([]msgOut, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, msgOut{ID: m.Id(), Role: m.Role, Content: m.Content, ToolCalls: m.ToolCalls, CreatedAt: m.CreatedAt})
	}
	return c.JSON(http.StatusOK, map[string]any{"conversationId": id, "messages": out})
}

// ── assembly ────────────────────────────────────────────────────────────────────

// assembleMessages prepends the preset's system prompt (plus any caller-supplied
// system text) to the caller's messages.
func assembleMessages(mode Preset, body runRequest) []openai.ChatCompletionMessage {
	system := mode.SystemPrompt
	if extra := strings.TrimSpace(body.System); extra != "" {
		system = strings.TrimSpace(system + "\n\n" + extra)
	}
	msgs := make([]openai.ChatCompletionMessage, 0, len(body.Messages)+1)
	if system != "" {
		msgs = append(msgs, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleSystem, Content: system})
	}
	for _, m := range body.Messages {
		role := strings.TrimSpace(m.Role)
		if role == "" {
			role = openai.ChatMessageRoleUser
		}
		msgs = append(msgs, openai.ChatCompletionMessage{Role: role, Content: m.Content})
	}
	return msgs
}

// assembleTools builds the tool array offered to the model: the preset's builtin
// tools, then the caller-passed defs, then the org's ACTIVATED + dispatchable
// registry tools — deduped by function name (an earlier, more-specific def wins).
func (s *Service) assembleTools(ctx context.Context, mode Preset, caller []openai.Tool, scope Scope) []openai.Tool {
	out := make([]openai.Tool, 0, len(mode.BuiltinTools)+len(caller))
	seen := map[string]bool{}
	add := func(t openai.Tool) {
		if t.Function == nil || t.Function.Name == "" || seen[t.Function.Name] {
			return
		}
		seen[t.Function.Name] = true
		out = append(out, t)
	}
	for _, t := range mode.BuiltinTools {
		add(t)
	}
	for _, t := range caller {
		add(t)
	}
	for _, t := range s.plane.List(ctx, scope) {
		if !t.Activated || !t.Dispatchable || seen[t.Name] {
			continue
		}
		seen[t.Name] = true
		out = append(out, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  schemaOrEmpty(t.Schema),
			},
		})
	}
	return out
}

// schemaOrEmpty returns the tool's JSON-Schema, or the empty-object schema when a
// source published none (the model requires a parameters object).
func schemaOrEmpty(schema json.RawMessage) json.RawMessage {
	if len(schema) == 0 {
		return json.RawMessage(`{"type":"object","properties":{}}`)
	}
	return schema
}

// decodeArgs parses a tool call's JSON argument string into a map. Empty or
// malformed arguments yield an empty map rather than an error, so a no-arg call
// still dispatches / returns as an op.
func decodeArgs(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil || args == nil {
		return map[string]any{}
	}
	return args
}

// lastUserMessage returns the last user turn's content — the request this round
// answers, persisted as the incoming user message.
func lastUserMessage(msgs []inMessage) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		role := strings.TrimSpace(msgs[i].Role)
		if role == "" || role == openai.ChatMessageRoleUser {
			if strings.TrimSpace(msgs[i].Content) != "" {
				return msgs[i].Content
			}
		}
	}
	return ""
}

// firstUserContent returns the first user turn's content — used to title a new
// conversation.
func firstUserContent(msgs []inMessage) string {
	for _, m := range msgs {
		role := strings.TrimSpace(m.Role)
		if role == "" || role == openai.ChatMessageRoleUser {
			if strings.TrimSpace(m.Content) != "" {
				return m.Content
			}
		}
	}
	return ""
}
