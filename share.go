package agent

// Share links: one conversation made readable by a link.
//
// A share is a row in the owning org's store: the conversation it names, the
// member who made it, the last message it carries, and the SHA-256 of its
// secret. The secret exists only in the link; the store never holds it, so a
// copy of the database cannot be turned back into working links.
//
// A second store, {DataDir}/shares.db, maps a secret's hash to the org whose
// store holds the share. Reading a link is one primary-key lookup there and one
// in that org's store: it never opens a store named by the caller and never
// looks through another org's rows.
//
// A share is a SNAPSHOT: it carries the user and assistant turns up to the last
// message at the moment it was made. System turns, tool calls and tool results
// are never part of it, and turns added later are not shared until the owner
// makes a new link.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/hanzoai/orm"
	ormdb "github.com/hanzoai/orm/db"
	"github.com/zap-proto/zip"
)

// AccessRead is the one access a share grants: read the snapshot. A recipient
// cannot append, because a conversation belongs to the member who opened it.
const AccessRead = "read"

// secretBytes is the entropy of a share secret: 256 bits.
const secretBytes = 32

// secretLen is the secret's length as unpadded base64url.
var secretLen = base64.RawURLEncoding.EncodedLen(secretBytes)

// Share is one link to a conversation, kept in the owning org's store.
type Share struct {
	orm.Model[Share]
	ConversationId string `json:"conversationId"`
	Org            string `json:"org"`
	User           string `json:"user"`
	Hash           string `json:"hash"`
	Access         string `json:"access"`
	Through        string `json:"through"`
	Revoked        bool   `json:"revoked"`
}

// ShareKey maps a secret's hash (the row id) to the org and share it opens.
type ShareKey struct {
	orm.Model[ShareKey]
	Org   string `json:"org"`
	Share string `json:"share"`
}

func init() {
	orm.Register[Share]("agent-share")
	orm.Register[ShareKey]("agent-share-key")
}

// errShareGone answers every link that does not open: never made, revoked, or
// malformed. One answer for all three, so a link says nothing about its history.
var errShareGone = errors.New("agent: share not found")

// seal returns the hex SHA-256 of a secret: what the stores keep in its place.
func seal(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// mintSecret returns a fresh 256-bit secret as unpadded base64url.
func mintSecret() (string, error) {
	b := make([]byte, secretBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("agent: share secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// wellFormed reports whether s has the shape of a minted secret.
func wellFormed(s string) bool {
	if len(s) != secretLen {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

// index returns the hash → org store, opening it on first use.
func (s *store) index() (orm.DB, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.keys != nil {
		return s.keys, nil
	}
	db, err := orm.OpenSQLite(&ormdb.SQLiteDBConfig{
		Path:   filepath.Join(s.dataDir, "shares.db"),
		Config: ormdb.SQLiteConfig{BusyTimeout: 5000, JournalMode: "WAL"},
	})
	if err != nil {
		return nil, fmt.Errorf("agent: open share index: %w", err)
	}
	s.keys = db
	return db, nil
}

// ownedConversation returns the conversation when user may share it: it is in
// org's store and it is theirs (see owns).
func (s *store) ownedConversation(org, user, convID string) (orm.DB, *Conversation, error) {
	db, err := s.dbFor(org)
	if err != nil {
		return nil, nil, err
	}
	conv, err := orm.Get[Conversation](db, strings.TrimSpace(convID))
	if err != nil {
		return nil, nil, err
	}
	if conv.Org != org || !owns(conv, user) {
		return nil, nil, orm.ErrNotFound
	}
	return db, conv, nil
}

// createShare makes a read-only link to a conversation the member owns and
// returns its secret. The secret is returned once and kept nowhere.
func (s *store) createShare(ctx context.Context, org, user, convID string) (string, *Share, error) {
	db, conv, err := s.ownedConversation(org, user, convID)
	if err != nil {
		return "", nil, err
	}
	msgs, err := orm.TypedQuery[Message](db).Filter("ConversationId=", conv.Id()).GetAll(ctx)
	if err != nil {
		return "", nil, err
	}
	through := ""
	for _, m := range msgs {
		if m.Id() > through {
			through = m.Id()
		}
	}
	if through == "" {
		return "", nil, fmt.Errorf("agent: conversation has nothing to share")
	}
	secret, err := mintSecret()
	if err != nil {
		return "", nil, err
	}
	hash := seal(secret)
	keys, err := s.index()
	if err != nil {
		return "", nil, err
	}
	sh := orm.New[Share](db)
	sh.SetId(newID())
	sh.ConversationId = conv.Id()
	sh.Org = org
	sh.User = strings.TrimSpace(user)
	sh.Hash = hash
	sh.Access = AccessRead
	sh.Through = through
	if err := sh.CreateCtx(ctx); err != nil {
		return "", nil, err
	}
	key := orm.New[ShareKey](keys)
	key.SetId(hash)
	key.Org = org
	key.Share = sh.Id()
	if err := key.CreateCtx(ctx); err != nil {
		_ = sh.DeleteCtx(ctx)
		return "", nil, err
	}
	return secret, sh, nil
}

// listShares returns the live links to a conversation the member owns, oldest first.
func (s *store) listShares(ctx context.Context, org, user, convID string) ([]*Share, error) {
	db, conv, err := s.ownedConversation(org, user, convID)
	if err != nil {
		return nil, err
	}
	all, err := orm.TypedQuery[Share](db).Filter("ConversationId=", conv.Id()).GetAll(ctx)
	if err != nil {
		return nil, err
	}
	live := all[:0]
	for _, sh := range all {
		if !sh.Revoked && sh.Org == org {
			live = append(live, sh)
		}
	}
	sort.SliceStable(live, func(i, j int) bool { return live[i].Id() < live[j].Id() })
	return live, nil
}

// revokeShare ends a link: the share row is marked revoked and its index row
// removed, so the secret resolves to nothing from then on.
func (s *store) revokeShare(ctx context.Context, org, user, convID, shareID string) error {
	db, conv, err := s.ownedConversation(org, user, convID)
	if err != nil {
		return err
	}
	sh, err := orm.Get[Share](db, strings.TrimSpace(shareID))
	if err != nil {
		return err
	}
	if sh.Org != org || sh.ConversationId != conv.Id() {
		return orm.ErrNotFound
	}
	keys, err := s.index()
	if err != nil {
		return err
	}
	if key, kerr := orm.Get[ShareKey](keys, sh.Hash); kerr == nil {
		if err := key.DeleteCtx(ctx); err != nil {
			return err
		}
	} else if !errors.Is(kerr, orm.ErrNotFound) {
		return kerr
	}
	if sh.Revoked {
		return nil
	}
	sh.Revoked = true
	return sh.UpdateCtx(ctx)
}

// openShare resolves a secret to its conversation and the turns it carries.
// Every failure — malformed, unknown, revoked, or a conversation since removed
// — is errShareGone.
func (s *store) openShare(ctx context.Context, secret string) (*Share, *Conversation, []*Message, error) {
	if !wellFormed(secret) {
		return nil, nil, nil, errShareGone
	}
	hash := seal(secret)
	keys, err := s.index()
	if err != nil {
		return nil, nil, nil, err
	}
	key, err := orm.Get[ShareKey](keys, hash)
	if errors.Is(err, orm.ErrNotFound) {
		return nil, nil, nil, errShareGone
	}
	if err != nil {
		return nil, nil, nil, err
	}
	db, err := s.dbFor(key.Org)
	if err != nil {
		return nil, nil, nil, errShareGone
	}
	sh, err := orm.Get[Share](db, key.Share)
	if errors.Is(err, orm.ErrNotFound) {
		return nil, nil, nil, errShareGone
	}
	if err != nil {
		return nil, nil, nil, err
	}
	if sh.Revoked || sh.Org != key.Org || subtle.ConstantTimeCompare([]byte(sh.Hash), []byte(hash)) != 1 {
		return nil, nil, nil, errShareGone
	}
	conv, err := orm.Get[Conversation](db, sh.ConversationId)
	if errors.Is(err, orm.ErrNotFound) {
		return nil, nil, nil, errShareGone
	}
	if err != nil {
		return nil, nil, nil, err
	}
	all, err := orm.TypedQuery[Message](db).Filter("ConversationId=", conv.Id()).GetAll(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	turns := all[:0]
	for _, m := range all {
		if m.Id() > sh.Through || !spoken(m) {
			continue
		}
		turns = append(turns, m)
	}
	sort.SliceStable(turns, func(i, j int) bool { return turns[i].Id() < turns[j].Id() })
	return sh, conv, turns, nil
}

// spoken reports whether a turn is part of a shared transcript: a user or
// assistant turn with words in it. System turns, tool results and a call-only
// assistant turn are not.
func spoken(m *Message) bool {
	role := strings.TrimSpace(m.Role)
	return (role == "user" || role == "assistant") && strings.TrimSpace(m.Content) != ""
}

// clip shortens s to at most n runes, marking a cut with an ellipsis.
func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:n])) + "…"
}

// ── routes ──────────────────────────────────────────────────────────────────────

// shareOut is a share as its owner sees it. The secret is not in it: it was
// returned once, when the share was made.
type shareOut struct {
	ID        string `json:"id"`
	Access    string `json:"access"`
	CreatedAt string `json:"createdAt"`
}

func outShare(sh *Share) shareOut {
	return shareOut{ID: sh.Id(), Access: sh.Access, CreatedAt: sh.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00")}
}

// sharedTurn is one turn of a shared transcript: who spoke and what they said.
type sharedTurn struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	CreatedAt string `json:"createdAt,omitempty"`
}

// previewTurns is how many turns a reader who is not signed in is shown, and
// previewRunes how much of each.
const (
	previewTurns = 2
	previewRunes = 280
)

// handleShare makes a link to one of the caller's conversations and returns its
// secret, once. POST {prefix}/conversations/:id/shares.
func (s *Service) handleShare(c *zip.Ctx) error {
	p, err := s.caller(c)
	if err != nil {
		return err
	}
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return zip.ErrBadRequest("conversation id required")
	}
	secret, sh, err := s.store.createShare(c.Context(), p.Org, p.User, id)
	if err != nil {
		if errors.Is(err, orm.ErrNotFound) {
			return zip.ErrNotFound("conversation not found")
		}
		return zip.Errorf(http.StatusInternalServerError, "agent: share: %v", err)
	}
	s.record(c, p, "agent.share.create", id, sh.Id())
	return c.JSON(http.StatusCreated, map[string]any{"share": outShare(sh), "token": secret})
}

// handleShares lists the live links to one of the caller's conversations.
// GET {prefix}/conversations/:id/shares.
func (s *Service) handleShares(c *zip.Ctx) error {
	p, err := s.caller(c)
	if err != nil {
		return err
	}
	items, err := s.store.listShares(c.Context(), p.Org, p.User, c.Param("id"))
	if err != nil {
		return conversationRefusal(err)
	}
	out := make([]shareOut, 0, len(items))
	for _, sh := range items {
		out = append(out, outShare(sh))
	}
	return c.JSON(http.StatusOK, map[string]any{"shares": out})
}

// handleUnshare revokes one link. DELETE {prefix}/conversations/:id/shares/:share.
func (s *Service) handleUnshare(c *zip.Ctx) error {
	p, err := s.caller(c)
	if err != nil {
		return err
	}
	id, share := strings.TrimSpace(c.Param("id")), strings.TrimSpace(c.Param("share"))
	if err := s.store.revokeShare(c.Context(), p.Org, p.User, id, share); err != nil {
		if errors.Is(err, orm.ErrNotFound) {
			return zip.ErrNotFound("share not found")
		}
		return zip.Errorf(http.StatusInternalServerError, "agent: revoke: %v", err)
	}
	s.record(c, p, "agent.share.revoke", id, share)
	return c.JSON(http.StatusOK, map[string]any{"id": share, "revoked": true})
}

// openRequest carries a share secret in the body, so it never sits in a URL an
// access log keeps.
type openRequest struct {
	Token string `json:"token"`
}

// handleOpenShare reads a shared conversation. POST {prefix}/shares/read.
//
// Any validated principal, of any org, reads the whole snapshot; the reader's
// own org is neither read nor changed. A caller with no principal gets the
// title and the first turns, clipped, and `full: false`. A secret that does not
// open answers 404 whatever the reason.
func (s *Service) handleOpenShare(c *zip.Ctx) error {
	var body openRequest
	if err := c.Bind(&body); err != nil {
		return err
	}
	sh, conv, turns, err := s.store.openShare(c.Context(), strings.TrimSpace(body.Token))
	if errors.Is(err, errShareGone) {
		return zip.ErrNotFound("This chat is no longer shared.")
	}
	if err != nil {
		return zip.Errorf(http.StatusInternalServerError, "agent: open share: %v", err)
	}
	p, ok := s.principal(c)
	full := ok && strings.TrimSpace(p.Org) != ""
	out := make([]sharedTurn, 0, len(turns))
	for i, m := range turns {
		if !full && i >= previewTurns {
			break
		}
		t := sharedTurn{Role: m.Role, Content: m.Content}
		if full {
			t.CreatedAt = m.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
		} else {
			t.Content = clip(m.Content, previewRunes)
		}
		out = append(out, t)
	}
	return c.JSON(http.StatusOK, map[string]any{
		"title":    conv.Title,
		"access":   sh.Access,
		"full":     full,
		"turns":    len(turns),
		"messages": out,
	})
}

// record reports a change to a share: to the host's audit trail when it gave
// one (Deps.Audit), and to the log always.
func (s *Service) record(c *zip.Ctx, p Principal, action, conversation, share string) {
	s.log.Info("agent share", "action", action, "org", p.Org, "user", p.User, "conversation", conversation, "share", share)
	if s.audit != nil {
		s.audit(c, action, conversation, share)
	}
}
