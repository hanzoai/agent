package agent

// Share links: one conversation made readable by a link, to the people who open
// it signed in.
//
// A share is a row in the owning org's store: the conversation it names, the
// member who made it, the last message it carries, and the SHA-256 of its
// secret. The secret exists only in the link; the store never holds it, so a
// copy of the database cannot be turned back into working links.
//
// A second store, {DataDir}/shares.db, maps a secret's hash to the org whose
// store holds the share, and keeps the viewers: each signed-in person who opened
// the link. Reading a link is one primary-key lookup there and one in that org's
// store: it never opens a store named by the caller and never looks through
// another org's rows.
//
// Who sees a shared chat: its owner, and the viewers. A caller who is not a
// signed-in person gets the title and nothing of the transcript. A signed-in
// person who opens the link is recorded as a viewer and reads the snapshot, and
// from then on finds it among the chats shared with them, without the link.
// Nobody else can list or read it. Revoking the link ends it for every viewer; the owner can also
// remove one viewer, who then cannot open it again with the link. The link names
// a resource and nothing more: it authenticates nobody, and a reader is whoever
// their own bearer says they are.
//
// A share is a SNAPSHOT: it carries the user and assistant turns up to the last
// message at the moment it was made. System turns, tool calls and tool results
// are never part of it, and turns added later are not shared until the owner
// makes a new link. An assistant turn carries the model that produced it only
// when the round stored it; a turn written through the record endpoint carries
// none, and a reader is told it was recorded by the person who shared it.

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
	"time"

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

// Share is one link to a conversation, kept in the owning org's store. Name is
// how the member who made it is shown to the people who open it.
type Share struct {
	orm.Model[Share]
	ConversationId string `json:"conversationId"`
	Org            string `json:"org"`
	User           string `json:"user"`
	Name           string `json:"name"`
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

// Viewer is one signed-in person who opened a share, kept in the share index.
// Org is the org that owns the share; User is the viewer, as their principal
// names them, and Name how the owner sees them. A removed viewer keeps the row,
// so the link does not let them back in.
type Viewer struct {
	orm.Model[Viewer]
	Org            string `json:"org"`
	Share          string `json:"share"`
	ConversationId string `json:"conversationId"`
	User           string `json:"user"`
	Name           string `json:"name"`
	Removed        bool   `json:"removed"`
}

func init() {
	orm.Register[Share]("agent-share")
	orm.Register[ShareKey]("agent-share-key")
	orm.Register[Viewer]("agent-share-viewer")
}

// errShareGone answers every link that does not open: never made, revoked,
// malformed, or closed to this viewer. One answer for all, so a link says
// nothing about its history.
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

// viewerID is the viewer row's id: one row per (share, person), so opening a
// link twice records one viewer.
func viewerID(org, share, user string) string {
	sum := sha256.Sum256([]byte(org + "\x00" + share + "\x00" + user))
	return hex.EncodeToString(sum[:])
}

// index returns the share index — hash → org, and the viewers — opening it on
// first use.
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
// org's store and it was opened by this member. A thread recorded with no member
// — before users were kept, or by a principal that names none — is nobody's to
// share, and neither is anything asked for by a caller with no user.
func (s *store) ownedConversation(org, user, convID string) (orm.DB, *Conversation, error) {
	user = strings.TrimSpace(user)
	if user == "" {
		return nil, nil, orm.ErrNotFound
	}
	db, err := s.dbFor(org)
	if err != nil {
		return nil, nil, err
	}
	conv, err := orm.Get[Conversation](db, strings.TrimSpace(convID))
	if err != nil {
		return nil, nil, err
	}
	if conv.Org != org || conv.User == "" || conv.User != user {
		return nil, nil, orm.ErrNotFound
	}
	return db, conv, nil
}

// createShare makes a read-only link to a conversation the member owns and
// returns its secret. The secret is returned once and kept nowhere.
func (s *store) createShare(ctx context.Context, org, user, name, convID string) (string, *Share, error) {
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
	sh.Name = strings.TrimSpace(name)
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

// liveShares returns the org's unrevoked shares, oldest first; conv narrows them
// to one conversation and user to one member's, when not empty.
func liveShares(ctx context.Context, db orm.DB, org, conv, user string) ([]*Share, error) {
	q := orm.TypedQuery[Share](db)
	if conv != "" {
		q = q.Filter("ConversationId=", conv)
	}
	if user != "" {
		q = q.Filter("User=", user)
	}
	all, err := q.GetAll(ctx)
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

// listShares returns the live links to a conversation the member owns, oldest first.
func (s *store) listShares(ctx context.Context, org, user, convID string) ([]*Share, error) {
	db, conv, err := s.ownedConversation(org, user, convID)
	if err != nil {
		return nil, err
	}
	return liveShares(ctx, db, org, conv.Id(), "")
}

// viewersOf returns the people a share is open to, earliest first.
func (s *store) viewersOf(ctx context.Context, org, share string) ([]*Viewer, error) {
	keys, err := s.index()
	if err != nil {
		return nil, err
	}
	rows, err := orm.TypedQuery[Viewer](keys).Filter("Share=", share).GetAll(ctx)
	if err != nil {
		return nil, err
	}
	out := rows[:0]
	for _, v := range rows {
		if v.Org == org && !v.Removed {
			out = append(out, v)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// endShare revokes a share: the row is marked revoked, its index row removed so
// the secret resolves to nothing, and every viewer row dropped so it leaves the
// lists of the people it was open to.
func (s *store) endShare(ctx context.Context, sh *Share) error {
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
	viewers, err := orm.TypedQuery[Viewer](keys).Filter("Share=", sh.Id()).GetAll(ctx)
	if err != nil {
		return err
	}
	for _, v := range viewers {
		if v.Org == sh.Org {
			if err := v.DeleteCtx(ctx); err != nil {
				return err
			}
		}
	}
	if sh.Revoked {
		return nil
	}
	sh.Revoked = true
	return sh.UpdateCtx(ctx)
}

// revokeShare ends a link to a conversation the member owns.
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
	return s.endShare(ctx, sh)
}

// revokeOrgShare ends any link in the org: an org admin's revoke.
func (s *store) revokeOrgShare(ctx context.Context, org, shareID string) (*Share, error) {
	db, err := s.dbFor(org)
	if err != nil {
		return nil, err
	}
	sh, err := orm.Get[Share](db, strings.TrimSpace(shareID))
	if err != nil {
		return nil, err
	}
	if sh.Org != org {
		return nil, orm.ErrNotFound
	}
	return sh, s.endShare(ctx, sh)
}

// removeViewer closes a share to one viewer. The row stays, marked removed, so
// the link does not let them back in; a new link does.
func (s *store) removeViewer(ctx context.Context, org, user, convID, shareID, viewerID string) error {
	db, conv, err := s.ownedConversation(org, user, convID)
	if err != nil {
		return err
	}
	sh, err := orm.Get[Share](db, strings.TrimSpace(shareID))
	if err != nil {
		return err
	}
	if sh.Org != org || sh.ConversationId != conv.Id() || sh.Revoked {
		return orm.ErrNotFound
	}
	keys, err := s.index()
	if err != nil {
		return err
	}
	v, err := orm.Get[Viewer](keys, strings.TrimSpace(viewerID))
	if err != nil {
		return err
	}
	if v.Org != org || v.Share != sh.Id() {
		return orm.ErrNotFound
	}
	if v.Removed {
		return nil
	}
	v.Removed = true
	return v.UpdateCtx(ctx)
}

// resolve returns the share a secret opens, and the owning org's store. Every
// failure — malformed, unknown, revoked — is errShareGone.
func (s *store) resolve(secret string) (orm.DB, *Share, error) {
	if !wellFormed(secret) {
		return nil, nil, errShareGone
	}
	hash := seal(secret)
	keys, err := s.index()
	if err != nil {
		return nil, nil, err
	}
	key, err := orm.Get[ShareKey](keys, hash)
	if errors.Is(err, orm.ErrNotFound) {
		return nil, nil, errShareGone
	}
	if err != nil {
		return nil, nil, err
	}
	db, err := s.dbFor(key.Org)
	if err != nil {
		return nil, nil, errShareGone
	}
	sh, err := orm.Get[Share](db, key.Share)
	if errors.Is(err, orm.ErrNotFound) {
		return nil, nil, errShareGone
	}
	if err != nil {
		return nil, nil, err
	}
	if sh.Revoked || sh.Org != key.Org || subtle.ConstantTimeCompare([]byte(sh.Hash), []byte(hash)) != 1 {
		return nil, nil, errShareGone
	}
	return db, sh, nil
}

// transcript returns a live share's conversation and the turns the share
// carries, oldest first. A conversation since removed is errShareGone.
func transcript(ctx context.Context, db orm.DB, sh *Share) (*Conversation, []*Message, error) {
	conv, err := orm.Get[Conversation](db, sh.ConversationId)
	if errors.Is(err, orm.ErrNotFound) {
		return nil, nil, errShareGone
	}
	if err != nil {
		return nil, nil, err
	}
	all, err := orm.TypedQuery[Message](db).Filter("ConversationId=", conv.Id()).GetAll(ctx)
	if err != nil {
		return nil, nil, err
	}
	turns := all[:0]
	for _, m := range all {
		if m.Id() > sh.Through || !spoken(m) {
			continue
		}
		turns = append(turns, m)
	}
	sort.SliceStable(turns, func(i, j int) bool { return turns[i].Id() < turns[j].Id() })
	return conv, turns, nil
}

// owner reports whether p made the share.
func owner(sh *Share, p Principal) bool {
	return p.Org == sh.Org && strings.TrimSpace(p.User) != "" && p.User == sh.User
}

// admit records p as a viewer of sh, once. A viewer the owner removed is
// refused with errShareGone; the owner is never recorded.
func (s *store) admit(ctx context.Context, sh *Share, p Principal) error {
	if owner(sh, p) {
		return nil
	}
	keys, err := s.index()
	if err != nil {
		return err
	}
	id := viewerID(sh.Org, sh.Id(), p.User)
	v, err := orm.Get[Viewer](keys, id)
	switch {
	case err == nil && v.Removed:
		return errShareGone
	case err == nil:
		return nil
	case !errors.Is(err, orm.ErrNotFound):
		return err
	}
	v = orm.New[Viewer](keys)
	v.SetId(id)
	v.Org = sh.Org
	v.Share = sh.Id()
	v.ConversationId = sh.ConversationId
	v.User = p.User
	v.Name = strings.TrimSpace(p.Name)
	v.CreatedAt = time.Now()
	v.UpdatedAt = v.CreatedAt
	// First writer wins and a row that is there is never written over, so an open
	// racing another open, or the owner's removal, cannot rewrite the row.
	created, err := keys.CreateIfAbsent(ctx, v.Key(), v)
	if err != nil {
		return err
	}
	if !created {
		had, err := orm.Get[Viewer](keys, id)
		if err != nil {
			return err
		}
		if had.Removed {
			return errShareGone
		}
		return nil
	}
	// A revoke that ran between the read of the share and this write deleted
	// every viewer row it found; this one came after, so it goes too.
	db, err := s.dbFor(sh.Org)
	if err != nil {
		return err
	}
	now, err := orm.Get[Share](db, sh.Id())
	if err != nil || now.Revoked {
		_ = v.DeleteCtx(ctx)
		return errShareGone
	}
	return nil
}

// viewer reports whether p is a live viewer of sh: recorded, and not removed.
// A removed viewer is errShareGone.
func (s *store) viewer(sh *Share, p Principal) (bool, error) {
	keys, err := s.index()
	if err != nil {
		return false, err
	}
	v, err := orm.Get[Viewer](keys, viewerID(sh.Org, sh.Id(), p.User))
	switch {
	case errors.Is(err, orm.ErrNotFound):
		return false, nil
	case err != nil:
		return false, err
	case v.Removed:
		return false, errShareGone
	}
	return true, nil
}

// viewing returns the share id names when user is one of its live viewers, and
// the owning org's store.
func (s *store) viewing(user, shareID string) (orm.DB, *Share, error) {
	shareID = strings.TrimSpace(shareID)
	if strings.TrimSpace(user) == "" || shareID == "" {
		return nil, nil, errShareGone
	}
	keys, err := s.index()
	if err != nil {
		return nil, nil, err
	}
	v, err := orm.TypedQuery[Viewer](keys).Filter("Share=", shareID).Filter("User=", user).First()
	if errors.Is(err, orm.ErrNotFound) {
		return nil, nil, errShareGone
	}
	if err != nil {
		return nil, nil, err
	}
	if v.Removed {
		return nil, nil, errShareGone
	}
	db, err := s.dbFor(v.Org)
	if err != nil {
		return nil, nil, errShareGone
	}
	sh, err := orm.Get[Share](db, shareID)
	if errors.Is(err, orm.ErrNotFound) {
		return nil, nil, errShareGone
	}
	if err != nil {
		return nil, nil, err
	}
	if sh.Revoked {
		_ = v.DeleteCtx(context.Background())
		return nil, nil, errShareGone
	}
	if sh.Org != v.Org {
		return nil, nil, errShareGone
	}
	return db, sh, nil
}

// own returns a live share p made, from p's own org store.
func (s *store) own(p Principal, shareID string) (orm.DB, *Share, error) {
	db, err := s.dbFor(p.Org)
	if err != nil {
		return nil, nil, errShareGone
	}
	sh, err := orm.Get[Share](db, strings.TrimSpace(shareID))
	if errors.Is(err, orm.ErrNotFound) {
		return nil, nil, errShareGone
	}
	if err != nil {
		return nil, nil, err
	}
	if sh.Revoked || !owner(sh, p) {
		return nil, nil, errShareGone
	}
	return db, sh, nil
}

// sharedWith returns the live shares user is a viewer of, most recently opened
// first, each with its conversation.
func (s *store) sharedWith(ctx context.Context, user string) ([]sharedItem, error) {
	if strings.TrimSpace(user) == "" {
		return nil, nil
	}
	keys, err := s.index()
	if err != nil {
		return nil, err
	}
	rows, err := orm.TypedQuery[Viewer](keys).Filter("User=", user).GetAll(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]sharedItem, 0, len(rows))
	for _, v := range rows {
		if v.Removed {
			continue
		}
		db, sh, err := s.viewing(user, v.Share)
		if errors.Is(err, errShareGone) {
			continue
		}
		if err != nil {
			return nil, err
		}
		conv, err := orm.Get[Conversation](db, sh.ConversationId)
		if errors.Is(err, orm.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, sharedItem{Share: sh.Id(), Title: conv.Title, By: sh.Name, OpenedAt: stamp(v.CreatedAt)})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].OpenedAt > out[j].OpenedAt })
	return out, nil
}

// spoken reports whether a turn is part of a shared transcript: a user or
// assistant turn with words in it. System turns, tool results and a call-only
// assistant turn are not.
func spoken(m *Message) bool {
	role := strings.TrimSpace(m.Role)
	return (role == "user" || role == "assistant") && strings.TrimSpace(m.Content) != ""
}

// ── routes ──────────────────────────────────────────────────────────────────────

// shareOut is a share as its owner sees it: the secret is not in it — it was
// returned once, when the share was made — and the viewers are.
type shareOut struct {
	ID        string      `json:"id"`
	Access    string      `json:"access"`
	CreatedAt string      `json:"createdAt"`
	Viewers   []viewerOut `json:"viewers"`
}

// viewerOut is one viewer as the owner sees them.
type viewerOut struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	OpenedAt string `json:"openedAt"`
}

// orgShareOut is a share as an org admin sees it.
type orgShareOut struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversationId"`
	Title          string `json:"title"`
	User           string `json:"user"`
	CreatedAt      string `json:"createdAt"`
	Viewers        int    `json:"viewers"`
}

// sharedItem is one chat shared with the caller, and who shared it.
type sharedItem struct {
	Share    string `json:"share"`
	Title    string `json:"title"`
	By       string `json:"by"`
	OpenedAt string `json:"openedAt"`
}

// sharedTurn is one turn of a shared transcript: who spoke, what they said, and
// for an assistant turn the round stored, the model that answered.
type sharedTurn struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Model     string `json:"model,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
}

// stamp formats a time the way every answer here does: RFC 3339, in UTC.
func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func (s *Service) outShare(ctx context.Context, sh *Share) (shareOut, error) {
	vs, err := s.store.viewersOf(ctx, sh.Org, sh.Id())
	if err != nil {
		return shareOut{}, err
	}
	out := shareOut{ID: sh.Id(), Access: sh.Access, CreatedAt: stamp(sh.CreatedAt), Viewers: make([]viewerOut, 0, len(vs))}
	for _, v := range vs {
		out.Viewers = append(out.Viewers, viewerOut{ID: v.Id(), Name: v.Name, OpenedAt: stamp(v.CreatedAt)})
	}
	return out, nil
}

// readOut is a shared conversation as a signed-in reader gets it.
func readOut(sh *Share, conv *Conversation, turns []*Message) map[string]any {
	out := make([]sharedTurn, 0, len(turns))
	for _, m := range turns {
		t := sharedTurn{Role: m.Role, Content: m.Content, CreatedAt: stamp(m.CreatedAt)}
		if m.Role == "assistant" {
			t.Model = m.Producer
		}
		out = append(out, t)
	}
	return map[string]any{"share": sh.Id(), "title": conv.Title, "by": sh.Name, "access": sh.Access, "full": true, "messages": out}
}

// signedIn returns the caller's principal when it is a person signed in with an
// org and a user: the only caller a shared chat opens for.
func (s *Service) signedIn(c *zip.Ctx) (Principal, bool) {
	p, ok := s.principal(c)
	if !ok || !p.Person || strings.TrimSpace(p.Org) == "" || strings.TrimSpace(p.User) == "" {
		return Principal{}, false
	}
	return p, true
}

// shareRefusal turns a store error into the answer a caller gets.
func shareRefusal(err error, what string) error {
	switch {
	case errors.Is(err, errShareGone):
		return zip.ErrNotFound("This chat is no longer shared.")
	case errors.Is(err, orm.ErrNotFound):
		return zip.ErrNotFound(what + " not found")
	}
	return zip.Errorf(http.StatusInternalServerError, "agent: %s: %v", what, err)
}

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
	secret, sh, err := s.store.createShare(c.Context(), p.Org, p.User, p.Name, id)
	if err != nil {
		return shareRefusal(err, "conversation")
	}
	s.record(c, p, "agent.share.create", id, sh.Id())
	out, err := s.outShare(c.Context(), sh)
	if err != nil {
		return shareRefusal(err, "share")
	}
	return c.JSON(http.StatusCreated, map[string]any{"share": out, "token": secret})
}

// handleShares lists the live links to one of the caller's conversations, each
// with its viewers. GET {prefix}/conversations/:id/shares.
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
		o, err := s.outShare(c.Context(), sh)
		if err != nil {
			return shareRefusal(err, "share")
		}
		out = append(out, o)
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
		return shareRefusal(err, "share")
	}
	s.record(c, p, "agent.share.revoke", id, share)
	return c.JSON(http.StatusOK, map[string]any{"id": share, "revoked": true})
}

// handleUnview removes one viewer from a link.
// DELETE {prefix}/conversations/:id/shares/:share/viewers/:viewer.
func (s *Service) handleUnview(c *zip.Ctx) error {
	p, err := s.caller(c)
	if err != nil {
		return err
	}
	id, share, viewer := strings.TrimSpace(c.Param("id")), strings.TrimSpace(c.Param("share")), strings.TrimSpace(c.Param("viewer"))
	if err := s.store.removeViewer(c.Context(), p.Org, p.User, id, share, viewer); err != nil {
		return shareRefusal(err, "viewer")
	}
	s.record(c, p, "agent.share.unview", id, share)
	return c.JSON(http.StatusOK, map[string]any{"id": viewer, "removed": true})
}

// openRequest carries a share secret in the body, so it never sits in a URL an
// access log keeps. Open is the reader's go-ahead to be recorded as a viewer,
// given after they were told who shared the chat and that their name is shown
// to them.
type openRequest struct {
	Token string `json:"token"`
	Open  bool   `json:"open"`
}

// handleOpenShare opens a link. POST {prefix}/shares/read.
//
// A caller who is not a signed-in person gets the title and `full: false`, and
// no turn of the transcript. A signed-in person who has not opened this link
// before gets the title, who shared it (`by`) and `confirm: true`, and nothing
// is recorded; asked again with `open: true`, they are recorded as a viewer —
// the sharer sees their name — and read the whole snapshot. The owner, and a
// viewer already recorded, read it at once. A link that does not open — or that
// its owner closed to this viewer — answers 404 whatever the reason.
func (s *Service) handleOpenShare(c *zip.Ctx) error {
	var body openRequest
	if err := c.Bind(&body); err != nil {
		return err
	}
	ctx := c.Context()
	db, sh, err := s.store.resolve(strings.TrimSpace(body.Token))
	if err != nil {
		return shareRefusal(err, "share")
	}
	conv, turns, err := transcript(ctx, db, sh)
	if err != nil {
		return shareRefusal(err, "share")
	}
	p, ok := s.signedIn(c)
	if !ok {
		return c.JSON(http.StatusOK, map[string]any{"title": conv.Title, "access": sh.Access, "full": false, "messages": []sharedTurn{}})
	}
	if !owner(sh, p) {
		seen, err := s.store.viewer(sh, p)
		if err != nil {
			return shareRefusal(err, "share")
		}
		if !seen && !body.Open {
			return c.JSON(http.StatusOK, map[string]any{"title": conv.Title, "by": sh.Name, "access": sh.Access, "full": false, "confirm": true, "messages": []sharedTurn{}})
		}
		if !seen {
			if err := s.store.admit(ctx, sh, p); err != nil {
				return shareRefusal(err, "share")
			}
		}
	}
	return c.JSON(http.StatusOK, readOut(sh, conv, turns))
}

// handleSharedWithMe lists the chats shared with the caller: the live links
// they opened signed in. GET {prefix}/shared.
func (s *Service) handleSharedWithMe(c *zip.Ctx) error {
	p, ok := s.signedIn(c)
	if !ok {
		return zip.ErrForbidden("a validated principal is required")
	}
	items, err := s.store.sharedWith(c.Context(), p.User)
	if err != nil {
		return shareRefusal(err, "share")
	}
	return c.JSON(http.StatusOK, map[string]any{"shared": items})
}

// handleReadShared reads one chat shared with the caller, without the link:
// the caller is one of its viewers, or the owner who made it.
// GET {prefix}/shared/:share.
func (s *Service) handleReadShared(c *zip.Ctx) error {
	p, ok := s.signedIn(c)
	if !ok {
		return zip.ErrForbidden("a validated principal is required")
	}
	db, sh, err := s.store.viewing(p.User, c.Param("share"))
	if errors.Is(err, errShareGone) {
		db, sh, err = s.store.own(p, c.Param("share"))
	}
	if err != nil {
		return shareRefusal(err, "share")
	}
	conv, turns, err := transcript(c.Context(), db, sh)
	if err != nil {
		return shareRefusal(err, "share")
	}
	return c.JSON(http.StatusOK, readOut(sh, conv, turns))
}

// orgAdmin returns the caller when they administer their org.
func (s *Service) orgAdmin(c *zip.Ctx) (Principal, error) {
	p, err := s.caller(c)
	if err != nil {
		return Principal{}, err
	}
	if !p.Admin {
		return Principal{}, zip.ErrForbidden("only an admin of this organization manages its shared links")
	}
	return p, nil
}

// handleOrgShares lists every live link in the caller's org, for its admins;
// ?user= narrows it to one member's. GET {prefix}/shares.
func (s *Service) handleOrgShares(c *zip.Ctx) error {
	p, err := s.orgAdmin(c)
	if err != nil {
		return err
	}
	ctx := c.Context()
	db, err := s.store.dbFor(p.Org)
	if err != nil {
		return shareRefusal(err, "share")
	}
	items, err := liveShares(ctx, db, p.Org, "", strings.TrimSpace(c.Query("user")))
	if err != nil {
		return shareRefusal(err, "share")
	}
	out := make([]orgShareOut, 0, len(items))
	for _, sh := range items {
		title := ""
		if conv, err := orm.Get[Conversation](db, sh.ConversationId); err == nil {
			title = conv.Title
		}
		vs, err := s.store.viewersOf(ctx, sh.Org, sh.Id())
		if err != nil {
			return shareRefusal(err, "share")
		}
		out = append(out, orgShareOut{ID: sh.Id(), ConversationID: sh.ConversationId, Title: title, User: sh.User, CreatedAt: stamp(sh.CreatedAt), Viewers: len(vs)})
	}
	return c.JSON(http.StatusOK, map[string]any{"shares": out})
}

// handleOrgUnshare revokes any link in the caller's org, for its admins.
// DELETE {prefix}/shares/:share.
func (s *Service) handleOrgUnshare(c *zip.Ctx) error {
	p, err := s.orgAdmin(c)
	if err != nil {
		return err
	}
	share := strings.TrimSpace(c.Param("share"))
	sh, err := s.store.revokeOrgShare(c.Context(), p.Org, share)
	if err != nil {
		return shareRefusal(err, "share")
	}
	s.record(c, p, "agent.share.revoke", sh.ConversationId, share)
	return c.JSON(http.StatusOK, map[string]any{"id": share, "revoked": true})
}

// record reports a change to a share: to the host's audit trail when it gave
// one (Deps.Audit), and to the log always. The ids are copied first: a route
// parameter aliases the request buffer, which is reused once the request ends.
func (s *Service) record(c *zip.Ctx, p Principal, action, conversation, share string) {
	conversation, share = strings.Clone(conversation), strings.Clone(share)
	s.log.Info("agent share", "action", action, "org", p.Org, "user", p.User, "conversation", conversation, "share", share)
	if s.audit != nil {
		s.audit(c, action, conversation, share)
	}
}
