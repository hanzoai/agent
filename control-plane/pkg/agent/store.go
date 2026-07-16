package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hanzoai/orm"
	ormdb "github.com/hanzoai/orm/db"
)

// Conversation is one persisted chat thread. Org is the owning org — physical
// isolation (one SQLite file per org) already scopes it; Org is stored for clarity
// and defense-in-depth.
type Conversation struct {
	orm.Model[Conversation]
	Org   string `json:"org"`
	Title string `json:"title"`
}

// Message is one persisted turn. ConversationId is deliberately spelled with a
// lowercase-d so orm's PascalCase→camelCase filter (ToJSONFieldName lowercases
// only the first rune) maps Filter("ConversationId=") onto the stored
// "conversationId" JSON key. ToolCalls is the marshaled model tool_calls (nil for
// a plain user/assistant turn).
type Message struct {
	orm.Model[Message]
	ConversationId string          `json:"conversationId"`
	Org            string          `json:"org"`
	Role           string          `json:"role"`
	Content        string          `json:"content"`
	ToolCalls      json.RawMessage `json:"toolCalls,omitempty"`
}

func init() {
	orm.Register[Conversation]("agent-conversation")
	orm.Register[Message]("agent-message")
}

// idSeq disambiguates ids minted within the same nanosecond so the zero-padded
// id string sorts in creation order.
var idSeq atomic.Uint64

// newID mints a lexically-sortable unique id: zero-padded UnixNano + a rolling
// counter. Ordering messages by id is therefore chronological without a separate
// sequence column.
func newID() string {
	return fmt.Sprintf("%019d-%06d", time.Now().UnixNano(), idSeq.Add(1)%1000000)
}

// store is the lazily-opened, cached set of per-org orm.DBs. Each org's SQLite
// file is opened (and its schema auto-migrated) exactly once, at
// {dataDir}/orgs/{slug}/agent.db. Isolation is PHYSICAL: a distinct org resolves
// to a distinct file, so a query in one can never reach another's rows.
type store struct {
	dataDir string
	mu      sync.Mutex
	byOrg   map[string]orm.DB
}

func newStore(dataDir string) *store {
	return &store{dataDir: dataDir, byOrg: map[string]orm.DB{}}
}

// dbFor returns the org's DB, opening + migrating it on first use.
func (s *store) dbFor(org string) (orm.DB, error) {
	slug, err := orgSlug(org)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if db, ok := s.byOrg[slug]; ok {
		return db, nil
	}
	path := filepath.Join(s.dataDir, "orgs", slug, "agent.db")
	db, err := orm.OpenSQLite(&ormdb.SQLiteDBConfig{
		Path:   path,
		Config: ormdb.SQLiteConfig{BusyTimeout: 5000, JournalMode: "WAL"},
	})
	if err != nil {
		return nil, fmt.Errorf("agent: open org db: %w", err)
	}
	s.byOrg[slug] = db
	return db, nil
}

// closeAll closes every open per-org DB. Idempotent; returns the first error.
func (s *store) closeAll() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var first error
	for k, db := range s.byOrg {
		if err := db.Close(); err != nil && first == nil {
			first = err
		}
		delete(s.byOrg, k)
	}
	return first
}

// orgSlug reduces an org id to a filesystem-safe, lowercase slug and refuses any
// value that could traverse out of the data dir. Org is the VALIDATED principal
// value (never a client-supplied field), but this fails closed on anything unsafe.
func orgSlug(org string) (string, error) {
	org = strings.TrimSpace(org)
	if org == "" {
		return "", fmt.Errorf("agent: empty org")
	}
	slug := strings.ToLower(org)
	for _, r := range slug {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
		default:
			return "", fmt.Errorf("agent: unsafe org %q", org)
		}
	}
	if slug == "." || slug == ".." || strings.Contains(slug, "..") {
		return "", fmt.Errorf("agent: unsafe org %q", org)
	}
	return slug, nil
}

// loadOrCreateConversation returns the conversation for id (in the org's DB),
// creating a fresh one when id is empty or not found. Physical per-org isolation
// means a found row always belongs to org; the Org check is belt-and-suspenders.
func (s *store) loadOrCreateConversation(ctx context.Context, org, id, title string) (*Conversation, error) {
	db, err := s.dbFor(org)
	if err != nil {
		return nil, err
	}
	if id = strings.TrimSpace(id); id != "" {
		conv, gerr := orm.Get[Conversation](db, id)
		if gerr == nil && conv.Org == org {
			return conv, nil
		}
		if gerr != nil && gerr != orm.ErrNotFound {
			return nil, gerr
		}
		// Not found (or a cross-org id, impossible under physical isolation) →
		// fall through and open a fresh conversation rather than touch a foreign row.
	}
	conv := orm.New[Conversation](db)
	conv.SetId(newID())
	conv.Org = org
	conv.Title = clampTitle(title)
	if err := conv.CreateCtx(ctx); err != nil {
		return nil, err
	}
	return conv, nil
}

// appendMessage persists one turn in a conversation.
func (s *store) appendMessage(ctx context.Context, org, convID, role, content string, toolCalls json.RawMessage) (*Message, error) {
	db, err := s.dbFor(org)
	if err != nil {
		return nil, err
	}
	m := orm.New[Message](db)
	m.SetId(newID())
	m.ConversationId = convID
	m.Org = org
	m.Role = role
	m.Content = content
	m.ToolCalls = toolCalls
	if err := m.CreateCtx(ctx); err != nil {
		return nil, err
	}
	return m, nil
}

// listConversations returns the org's conversations, most-recently-updated first.
func (s *store) listConversations(ctx context.Context, org string) ([]*Conversation, error) {
	db, err := s.dbFor(org)
	if err != nil {
		return nil, err
	}
	items, err := orm.TypedQuery[Conversation](db).GetAll(ctx)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
	return items, nil
}

// conversationMessages returns a conversation's messages in chronological order.
func (s *store) conversationMessages(ctx context.Context, org, convID string) ([]*Message, error) {
	db, err := s.dbFor(org)
	if err != nil {
		return nil, err
	}
	items, err := orm.TypedQuery[Message](db).Filter("ConversationId=", convID).GetAll(ctx)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Id() < items[j].Id() })
	return items, nil
}

// clampTitle derives a short, single-line conversation title.
func clampTitle(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if s == "" {
		return "New conversation"
	}
	const max = 80
	if len(s) > max {
		return strings.TrimSpace(s[:max])
	}
	return s
}
