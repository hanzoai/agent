package agent

import (
	"encoding/json"
	"sort"
	"strings"

	openai "github.com/hanzoai/go-openai"
)

// Preset is one named agent type in the preset LIBRARY — a first-class, extensible
// catalog the round is created from and the presets route lists.
// It frames a tool-calling round: the system prompt that instructs the model, the
// builtin tool defs offered alongside the caller's and the org's registered tools,
// and whether the model's tool calls are EXECUTED server-side (registry Dispatch →
// actions) or handed back to the client as ops (a graph/UI mutation the server
// cannot perform). Adding an agent type is one Register call — no other change.
type Preset struct {
	ID           string        `json:"id"`
	Title        string        `json:"title"`
	SystemPrompt string        `json:"systemPrompt"`
	BuiltinTools []openai.Tool `json:"-"`
	// ServerExecuted gates the tool-call split: when true, a call the tool
	// registry knows is dispatched server-side and reported in actions; when
	// false the round is advisory — every call is returned as an op for the
	// client to apply.
	ServerExecuted bool `json:"serverExecuted"`
}

// DefaultPreset is used when a request omits preset (and its capability alias).
const DefaultPreset = "graph"

// presets is the library. Seeded with graph + create below; a host extends it by
// calling Register before Mount.
var presets = map[string]Preset{}

// Register adds (or replaces) a preset in the library. Call from init() or before
// Mount. Replacing an existing id is allowed so a host can re-skin a builtin.
func Register(p Preset) {
	presets[strings.TrimSpace(p.ID)] = p
}

// Presets returns every registered preset, sorted by ID for a stable listing.
func Presets() []Preset {
	out := make([]Preset, 0, len(presets))
	for _, p := range presets {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// presetFor resolves a preset by id, defaulting an empty id to DefaultPreset. ok
// is false for an unknown id so the handler answers 400 rather than guessing.
func presetFor(id string) (Preset, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		id = DefaultPreset
	}
	p, ok := presets[id]
	return p, ok
}

// toolDef builds an OpenAI function tool from a JSON-Schema string. The schema is
// held as json.RawMessage so it reaches the model verbatim.
func toolDef(name, desc, schema string) openai.Tool {
	return openai.Tool{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        name,
			Description: desc,
			Parameters:  json.RawMessage(schema),
		},
	}
}

func init() {
	Register(Preset{
		ID:             "graph",
		Title:          "Studio Graph Copilot",
		ServerExecuted: false,
		SystemPrompt: strings.TrimSpace(`
You are Hanzo Studio Copilot. You read and mutate a node workflow graph
(LiteGraph / ComfyUI-compatible). The user describes an outcome; you respond with
tool calls that describe the graph operations to apply. You never execute the
graph yourself — the Studio client applies each operation and re-queues the graph.
Reference nodes you create by the id you give them in add_node. Prefer the fewest
operations that achieve the request; explain briefly in text when helpful.`),
		BuiltinTools: []openai.Tool{
			toolDef("add_node", "Add a node to the graph.", `{
				"type":"object",
				"properties":{
					"id":{"type":"string","description":"caller label to reference this node in later ops"},
					"type":{"type":"string","description":"node class, e.g. KSampler"},
					"pos":{"type":"array","items":{"type":"number"},"description":"[x,y] canvas position"}
				},
				"required":["type"]
			}`),
			toolDef("set_widget", "Set a widget value on a node.", `{
				"type":"object",
				"properties":{
					"node_id":{"type":"string"},
					"name":{"type":"string","description":"widget name"},
					"value":{}
				},
				"required":["node_id","name","value"]
			}`),
			toolDef("set_prompt", "Set the prompt text on a node.", `{
				"type":"object",
				"properties":{"node_id":{"type":"string"},"text":{"type":"string"}},
				"required":["node_id","text"]
			}`),
			toolDef("connect", "Connect an output slot to an input slot.", `{
				"type":"object",
				"properties":{
					"from":{"type":"string","description":"source node id"},
					"from_slot":{"description":"output slot index or name"},
					"to":{"type":"string","description":"target node id"},
					"to_slot":{"description":"input slot index or name"}
				},
				"required":["from","to"]
			}`),
			toolDef("move_node", "Move a node to a new position.", `{
				"type":"object",
				"properties":{"node_id":{"type":"string"},"pos":{"type":"array","items":{"type":"number"}}},
				"required":["node_id","pos"]
			}`),
			toolDef("delete_node", "Remove a node from the graph.", `{
				"type":"object",
				"properties":{"node_id":{"type":"string"}},
				"required":["node_id"]
			}`),
			toolDef("layout", "Auto-arrange the graph.", `{"type":"object","properties":{}}`),
			toolDef("queue", "Queue the graph for execution.", `{"type":"object","properties":{}}`),
		},
	})

	Register(Preset{
		ID:             "create",
		Title:          "Product & Fashion Create",
		ServerExecuted: true,
		SystemPrompt: strings.TrimSpace(`
You are Hanzo Create, a fashion and product render assistant. You help the user
generate, edit, and refine product and fashion imagery. Use the tools available to
this workspace — the org's connected render and MCP services — to run and iterate
on renders. Call a tool when it advances the request; describe the result briefly.`),
		// No builtin tools: the callable set is the org's registered MCP/registry
		// tools, resolved per request and dispatched server-side.
		BuiltinTools: nil,
	})
}
