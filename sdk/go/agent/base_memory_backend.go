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

// BaseMemoryBackend stores agent memory in Hanzo Base, one record per
// (scope, scopeID, key) in one collection.
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
// The collection must exist: import agent_memory.collection.json once, as
// MEMORY.md shows. Nothing here creates schema.
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

// The record this backend writes. `mkey`, because `key` is reserved in some
// filter grammars.
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
		"scope=" + quote(apiScope(scope)),
		"scope_id=" + quote(scopeID),
	}
	if key != "" {
		parts = append(parts, "mkey="+quote(key))
	}
	return strings.Join(parts, " && ")
}

// A single-quoted literal in Base's filter grammar. Base's tokenizer reads a
// backslash as an escape, so backslashes double before quotes are escaped: the
// other order lets a value ending in one escape the closing quote.
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

	// Base answers a path it does not route with its admin SPA, at 200.
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
func (b *BaseMemoryBackend) write(ctx context.Context, scope MemoryScope, rec baseRecord) error {
	rec.Scope = apiScope(scope)
	existing, found, err := b.find(ctx, scope, rec.ScopeID, rec.MKey)
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
	if !found && status == http.StatusBadRequest {
		// Another writer created the key between the read and this create, and
		// the unique index refused a second record. A Set means the last write
		// wins, so update the record that won.
		if winner, ok, ferr := b.find(ctx, scope, rec.ScopeID, rec.MKey); ferr == nil && ok {
			raw, status, err = b.do(ctx, http.MethodPatch, b.records()+"/"+url.PathEscape(winner.ID), rec)
			if err != nil {
				return err
			}
		}
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
	return b.write(context.Background(), scope, baseRecord{
		ScopeID: scopeID, MKey: key, Value: string(encoded),
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

// List returns every key in a scope, across all of Base's pages.
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
	return b.write(context.Background(), scope, baseRecord{
		ScopeID: scopeID, MKey: key,
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

// SearchVector ranks the scope's vectors by cosine similarity, in the client:
// cost grows with the scope, which suits an agent's working set, not a corpus.
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
			// A different width is a different embedding model; skip it.
			if len(item.Embedding) != len(embedding) || !matches(item.Metadata, opts.Filters) {
				continue
			}
			score := cosine(embedding, item.Embedding)
			if score < opts.Threshold {
				continue
			}
			found = append(found, VectorSearchResult{
				Key: item.MKey, Score: score, Metadata: item.Metadata,
				Scope: scope, ScopeID: item.ScopeID,
			})
		}
		if len(out.Items) >= out.TotalItems {
			break
		}
	}

	return rank(found, opts.Limit), nil
}

// DeleteVector clears the embedding and keeps any value stored at the key.
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
