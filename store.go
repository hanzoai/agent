package agent

import (
	"context"
	"encoding/json"
	"errors"
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
// and defense-in-depth. User is the member who opened it: a thread is one
// person's inside a shared org, so it lists and opens for them alone. A thread
// recorded before users were kept has none, and stays the org's. Pinned lists it
// first; Archived takes it out of the list, into the archived one. UpdatedAt is
// when it was last spoken in: a rename, a pin or an archive does not move it.
type Conversation struct {
	orm.Model[Conversation]
	Org      string `json:"org"`
	User     string `json:"user"`
	Title    string `json:"title"`
	Pinned   bool   `json:"pinned"`
	Archived bool   `json:"archived"`
}

// Message is one persisted turn. ConversationId is deliberately spelled with a
// lowercase-d so orm's PascalCase→camelCase filter (ToJSONFieldName lowercases
// only the first rune) maps Filter("ConversationId=") onto the stored
// "conversationId" JSON key. ToolCalls is the marshaled model tool_calls (nil for
// a plain user/assistant turn). Producer names the model whose completion this
// server received and stored as the turn — set only by the round; a turn written
// through the record endpoint carries none, whatever role it claims.
type Message struct {
	orm.Model[Message]
	ConversationId string          `json:"conversationId"`
	Org            string          `json:"org"`
	Role           string          `json:"role"`
	Content        string          `json:"content"`
	Producer       string          `json:"model,omitempty"`
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
	// keys is the share index, {dataDir}/shares.db (share.go), opened on first use.
	keys orm.DB
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
	if s.keys != nil {
		if err := s.keys.Close(); err != nil && first == nil {
			first = err
		}
		s.keys = nil
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
func (s *store) loadOrCreateConversation(ctx context.Context, org, user, id, title string) (*Conversation, error) {
	db, err := s.dbFor(org)
	if err != nil {
		return nil, err
	}
	if id = strings.TrimSpace(id); id != "" {
		conv, gerr := orm.Get[Conversation](db, id)
		if gerr == nil && conv.Org == org {
			if !owns(conv, user) {
				return nil, orm.ErrNotFound
			}
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
	conv.User = strings.TrimSpace(user)
	conv.Title = clampTitle(title)
	if err := conv.CreateCtx(ctx); err != nil {
		return nil, err
	}
	return conv, nil
}

// appendMessage persists one turn in a conversation. model is the model whose
// completion the turn is, or "" for a turn a caller wrote.
func (s *store) appendMessage(ctx context.Context, org, convID, role, content, model string, toolCalls json.RawMessage) (*Message, error) {
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
	m.Producer = model
	m.ToolCalls = toolCalls
	if err := m.CreateCtx(ctx); err != nil {
		return nil, err
	}
	return m, nil
}

// listConversations returns the member's conversations that are archived, or
// that are not: pinned first, then most recently spoken in.
func (s *store) listConversations(ctx context.Context, org, user string, archived bool) ([]*Conversation, error) {
	db, err := s.dbFor(org)
	if err != nil {
		return nil, err
	}
	all, err := orm.TypedQuery[Conversation](db).GetAll(ctx)
	if err != nil {
		return nil, err
	}
	items := all[:0]
	for _, cv := range all {
		if owns(cv, user) && cv.Archived == archived {
			items = append(items, cv)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Pinned != items[j].Pinned {
			return items[i].Pinned
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	return items, nil
}

// change is what a member may set on their own conversation; nil leaves it.
type change struct {
	Title    *string `json:"title"`
	Pinned   *bool   `json:"pinned"`
	Archived *bool   `json:"archived"`
}

// errEmptyTitle refuses a rename to nothing.
var errEmptyTitle = errors.New("agent: title must not be empty")

// updateConversation applies a change to a conversation the member owns. The
// row is written as it stands, so UpdatedAt keeps the last time it was spoken in.
func (s *store) updateConversation(ctx context.Context, org, user, convID string, ch change) (*Conversation, error) {
	if ch.Title != nil && strings.TrimSpace(*ch.Title) == "" {
		return nil, errEmptyTitle
	}
	db, conv, err := s.ownedConversation(org, user, convID)
	if err != nil {
		return nil, err
	}
	if ch.Title != nil {
		conv.Title = clampTitle(*ch.Title)
	}
	if ch.Pinned != nil {
		conv.Pinned = *ch.Pinned
	}
	if ch.Archived != nil {
		conv.Archived = *ch.Archived
	}
	if _, err := db.Put(ctx, conv.Key(), conv); err != nil {
		return nil, err
	}
	return conv, nil
}

// deleteConversation removes a conversation the member owns, for good: its links
// first, so none opens once the call returns, then its turns, then the thread.
// A failure part way leaves the thread listed, and deleting it again finishes.
func (s *store) deleteConversation(ctx context.Context, org, user, convID string) error {
	db, conv, err := s.ownedConversation(org, user, convID)
	if err != nil {
		return err
	}
	shares, err := orm.TypedQuery[Share](db).Filter("ConversationId=", conv.Id()).GetAll(ctx)
	if err != nil {
		return err
	}
	for _, sh := range shares {
		if err := s.endShare(ctx, sh); err != nil {
			return err
		}
		if err := sh.DeleteCtx(ctx); err != nil {
			return err
		}
	}
	turns, err := orm.TypedQuery[Message](db).Filter("ConversationId=", conv.Id()).GetAll(ctx)
	if err != nil {
		return err
	}
	for _, m := range turns {
		if err := m.DeleteCtx(ctx); err != nil {
			return err
		}
	}
	return conv.DeleteCtx(ctx)
}

// touch marks a conversation as spoken in now, and reports whether it is still
// there. It reads the row again rather than writing back a copy taken when the
// round began, so a rename, pin or archive made while the model answered
// stands, and a thread deleted meanwhile stays deleted.
func (s *store) touch(ctx context.Context, org, convID string) (bool, error) {
	db, err := s.dbFor(org)
	if err != nil {
		return false, err
	}
	conv, err := orm.Get[Conversation](db, convID)
	if errors.Is(err, orm.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, conv.UpdateCtx(ctx)
}

// conversationMessages returns a conversation's messages in chronological order.
func (s *store) conversationMessages(ctx context.Context, org, user, convID string) ([]*Message, error) {
	db, err := s.dbFor(org)
	if err != nil {
		return nil, err
	}
	conv, err := orm.Get[Conversation](db, convID)
	if err != nil {
		return nil, err
	}
	if conv.Org != org || !owns(conv, user) {
		return nil, orm.ErrNotFound
	}
	items, err := orm.TypedQuery[Message](db).Filter("ConversationId=", convID).GetAll(ctx)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Id() < items[j].Id() })
	return items, nil
}

// whole reports whether the stored transcript is everything a round's caller
// put in front of the model: no system text of the caller's, no tools of the
// caller's, and the messages sent equal, turn for turn, the user and assistant
// turns the conversation holds.
func (s *store) whole(ctx context.Context, org, user, convID string, body runRequest) (bool, error) {
	if strings.TrimSpace(body.System) != "" || len(body.Tools) > 0 {
		return false, nil
	}
	held, err := s.conversationMessages(ctx, org, user, convID)
	if errors.Is(err, orm.ErrNotFound) {
		// Deleted while the model answered: there is no transcript to be whole.
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if len(held) != len(body.Messages) {
		return false, nil
	}
	for i, m := range held {
		sent := body.Messages[i]
		role := strings.TrimSpace(sent.Role)
		if role == "" {
			role = "user"
		}
		if (m.Role != "user" && m.Role != "assistant") || m.Role != role || m.Content != sent.Content {
			return false, nil
		}
	}
	return true, nil
}

// owns reports whether a conversation is this member's to list and read: theirs,
// or one recorded before users were kept.
func owns(cv *Conversation, user string) bool {
	return cv.User == "" || cv.User == strings.TrimSpace(user)
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
