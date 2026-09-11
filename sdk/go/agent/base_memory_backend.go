package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// BaseMemoryBackend stores agent memory in Hanzo Base.
//
// Base is the single-binary Go backend, so this is the backend to reach for
// when an agent should keep its state in the same process tree as everything
// else it runs beside — no second database, no separate migration, and the
// records are visible in Base's own admin view while an agent is running.
//
// One record per (scope, scopeID, key), in one collection. The alternative —
// a collection per scope — makes the number of collections a function of how
// many workflows have ever run.
//
// Paths are /v1/, with no /api/ prefix. Base served /api for a while and does
// not now, and a client written against the old shape fails in a way that
// reads like an auth problem: the router answers the SPA catch-all, so a
// wrong path is 200 text/html rather than 404.
type BaseMemoryBackend struct {
	baseURL    string
	token      string
	collection string
	httpClient *http.Client
}

// NewBaseMemoryBackend returns a backend backed by one Base collection.
//
// baseURL is where Base answers — http://127.0.0.1:8090 for a local one.
// token is an IAM bearer; Base guards the records API and an empty token gets
// a refusal rather than anonymous access.
//
// The collection must exist, with the fields this backend writes — import
// agent_memory.collection.json once, as MEMORY.md shows. It is not created
// here on purpose: a client that creates schema on first write is a client
// that can create the WRONG schema on a typo, and then read nothing from the
// collection anybody was looking at.
func NewBaseMemoryBackend(baseURL, token, collection string) *BaseMemoryBackend {
	if collection == "" {
		collection = "agent_memory"
	}
	return &BaseMemoryBackend{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		collection: collection,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// The record this backend writes. `mkey` rather than `key`: `key` is a
// reserved word in enough query grammars that a filter on it is a portability
// bet nobody needs to take.
type baseRecord struct {
	ID        string         `json:"id,omitempty"`
	Scope     string         `json:"scope"`
	ScopeID   string         `json:"scope_id"`
	MKey      string         `json:"mkey"`
	Value     string         `json:"value,omitempty"`
	Embedding []float64      `json:"embedding,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type baseList struct {
	Items      []baseRecord `json:"items"`
	TotalItems int          `json:"totalItems"`
}

func (b *BaseMemoryBackend) records() string {
	return b.baseURL + "/v1/collections/" + url.PathEscape(b.collection) + "/records"
}

// A filter in Base's own grammar. Quoted values, because a scopeID is caller
// data and an unquoted one is a filter injection.
func filterFor(scope MemoryScope, scopeID, key string) string {
	parts := []string{
		"scope=" + quote(string(scope)),
		"scope_id=" + quote(scopeID),
	}
	if key != "" {
		parts = append(parts, "mkey="+quote(key))
	}
	return strings.Join(parts, " && ")
}

// A single-quoted literal in Base's filter grammar, with the value escaped.
//
// Base's tokenizer treats a backslash as the escape rune, so a quote preceded
// by one does not close the literal. That makes the ORDER here load-bearing:
// backslashes are doubled first, then quotes escaped. Escaping quotes first
// would leave a value ending in a backslash escaping the closing quote we
// added, which ends the literal somewhere the caller chose.
func quote(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	return "'" + s + "'"
}

func (b *BaseMemoryBackend) do(ctx context.Context, method, endpoint string, body any) ([]byte, int, error) {
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if b.token != "" {
		req.Header.Set("Authorization", "Bearer "+b.token)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}

	// An HTML body on a JSON endpoint means the path was not served and the
	// SPA catch-all answered. Saying so beats a JSON parse error, which reads
	// as a broken server rather than a wrong URL.
	if ct := resp.Header.Get("Content-Type"); strings.HasPrefix(ct, "text/html") {
		return nil, resp.StatusCode, fmt.Errorf(
			"base answered %s with HTML for %s — that path is not served by this Base", ct, endpoint)
	}
	return raw, resp.StatusCode, nil
}

// find returns the record at one key, or ok=false where there is none.
func (b *BaseMemoryBackend) find(ctx context.Context, scope MemoryScope, scopeID, key string) (baseRecord, bool, error) {
	endpoint := b.records() + "?perPage=1&filter=" + url.QueryEscape(filterFor(scope, scopeID, key))
	raw, status, err := b.do(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return baseRecord{}, false, err
	}
	if status != http.StatusOK {
		return baseRecord{}, false, fmt.Errorf("base list returned %d: %s", status, snippet(raw))
	}
	var out baseList
	if err := json.Unmarshal(raw, &out); err != nil {
		return baseRecord{}, false, fmt.Errorf("decode list: %w", err)
	}
	if len(out.Items) == 0 {
		return baseRecord{}, false, nil
	}
	return out.Items[0], true, nil
}

// write creates or updates the one record at a key.
func (b *BaseMemoryBackend) write(ctx context.Context, rec baseRecord) error {
	existing, found, err := b.find(ctx, MemoryScope(rec.Scope), rec.ScopeID, rec.MKey)
	if err != nil {
		return err
	}

	method, endpoint := http.MethodPost, b.records()
	if found {
		// PATCH, not PUT: a Set of a value must not erase an embedding written
		// beside it by SetVector, and the two share a record.
		method, endpoint = http.MethodPatch, b.records()+"/"+url.PathEscape(existing.ID)
	}

	raw, status, err := b.do(ctx, method, endpoint, rec)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return fmt.Errorf("base write returned %d: %s", status, snippet(raw))
	}
	return nil
}

// Set stores a value at the given scope and key.
func (b *BaseMemoryBackend) Set(scope MemoryScope, scopeID, key string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode value: %w", err)
	}
	return b.write(context.Background(), baseRecord{
		Scope: string(scope), ScopeID: scopeID, MKey: key, Value: string(encoded),
	})
}

// Get retrieves a value; returns (value, found, error).
func (b *BaseMemoryBackend) Get(scope MemoryScope, scopeID, key string) (any, bool, error) {
	rec, found, err := b.find(context.Background(), scope, scopeID, key)
	if err != nil || !found || rec.Value == "" {
		return nil, found && rec.Value != "", err
	}
	var value any
	if err := json.Unmarshal([]byte(rec.Value), &value); err != nil {
		return nil, false, fmt.Errorf("decode value at %s/%s: %w", scopeID, key, err)
	}
	return value, true, nil
}

// Delete removes a key from storage. Absent is not an error: the caller asked
// for the key to be gone and it is.
func (b *BaseMemoryBackend) Delete(scope MemoryScope, scopeID, key string) error {
	ctx := context.Background()
	rec, found, err := b.find(ctx, scope, scopeID, key)
	if err != nil || !found {
		return err
	}
	raw, status, err := b.do(ctx, http.MethodDelete, b.records()+"/"+url.PathEscape(rec.ID), nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusNoContent {
		return fmt.Errorf("base delete returned %d: %s", status, snippet(raw))
	}
	return nil
}

// List returns all keys in a scope, following Base's pages to the end. A
// caller asking for "all keys" and receiving the first thirty would read that
// as the whole answer.
func (b *BaseMemoryBackend) List(scope MemoryScope, scopeID string) ([]string, error) {
	ctx := context.Background()
	var keys []string
	for page := 1; ; page++ {
		endpoint := b.records() +
			"?perPage=200&page=" + strconv.Itoa(page) +
			"&filter=" + url.QueryEscape(filterFor(scope, scopeID, ""))
		raw, status, err := b.do(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		if status != http.StatusOK {
			return nil, fmt.Errorf("base list returned %d: %s", status, snippet(raw))
		}
		var out baseList
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, fmt.Errorf("decode list: %w", err)
		}
		for _, item := range out.Items {
			keys = append(keys, item.MKey)
		}
		if len(out.Items) == 0 || len(keys) >= out.TotalItems {
			return keys, nil
		}
	}
}

// SetVector stores a vector embedding with optional metadata.
func (b *BaseMemoryBackend) SetVector(scope MemoryScope, scopeID, key string, embedding []float64, metadata map[string]any) error {
	return b.write(context.Background(), baseRecord{
		Scope: string(scope), ScopeID: scopeID, MKey: key,
		Embedding: embedding, Metadata: metadata,
	})
}

// GetVector retrieves a vector and its metadata.
func (b *BaseMemoryBackend) GetVector(scope MemoryScope, scopeID, key string) ([]float64, map[string]any, bool, error) {
	rec, found, err := b.find(context.Background(), scope, scopeID, key)
	if err != nil || !found || len(rec.Embedding) == 0 {
		return nil, nil, found && len(rec.Embedding) > 0, err
	}
	return rec.Embedding, rec.Metadata, true, nil
}

// SearchVector ranks the scope's vectors by cosine similarity.
//
// Scored here rather than in the database, and that is a limit worth stating:
// this reads the scope's vectors and ranks them in the client, so cost grows
// with the size of the scope. It is right for an agent's own working set and
// wrong for a corpus. Base can score server-side through a hook, and a scope
// large enough to need that should use one.
//
// Threshold and Filters are applied while scoring, before Limit, so a limit
// of five returns the five best MATCHING records rather than whatever survives
// filtering out of the first five.
func (b *BaseMemoryBackend) SearchVector(scope MemoryScope, scopeID string, embedding []float64, opts SearchOptions) ([]VectorSearchResult, error) {
	if len(embedding) == 0 {
		return nil, fmt.Errorf("search needs a query vector")
	}
	ctx := context.Background()

	found := []VectorSearchResult{}
	for page := 1; ; page++ {
		endpoint := b.records() +
			"?perPage=200&page=" + strconv.Itoa(page) +
			"&filter=" + url.QueryEscape(filterFor(scope, scopeID, ""))
		raw, status, err := b.do(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		if status != http.StatusOK {
			return nil, fmt.Errorf("base list returned %d: %s", status, snippet(raw))
		}
		var out baseList
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, fmt.Errorf("decode list: %w", err)
		}
		if len(out.Items) == 0 {
			break
		}
		for _, item := range out.Items {
			// A vector of a different width is not less similar, it is a
			// different embedding model. Skipped rather than scored, because
			// cosine over mismatched widths returns a number that means
			// nothing.
			if len(item.Embedding) != len(embedding) || !matches(item.Metadata, opts.Filters) {
				continue
			}
			score := cosine(embedding, item.Embedding)
			if score < opts.Threshold {
				continue
			}
			found = append(found, VectorSearchResult{
				Key: item.MKey, Score: score, Metadata: item.Metadata,
				Scope: MemoryScope(item.Scope), ScopeID: item.ScopeID,
			})
		}
		if len(out.Items) >= out.TotalItems {
			break
		}
	}

	return rank(found, opts.Limit), nil
}

// DeleteVector removes a vector from storage.
//
// The embedding, not the record: a value may be stored at the same key and is
// not the caller's to lose here.
func (b *BaseMemoryBackend) DeleteVector(scope MemoryScope, scopeID, key string) error {
	ctx := context.Background()
	rec, found, err := b.find(ctx, scope, scopeID, key)
	if err != nil || !found {
		return err
	}
	raw, status, err := b.do(ctx, http.MethodPatch, b.records()+"/"+url.PathEscape(rec.ID),
		map[string]any{"embedding": nil, "metadata": nil})
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("base patch returned %d: %s", status, snippet(raw))
	}
	return nil
}

// Enough of a body to identify a refusal, and not enough to put a token or a
// stored value into a log.
func snippet(raw []byte) string {
	const most = 200
	if len(raw) > most {
		return string(raw[:most]) + "…"
	}
	return string(raw)
}
