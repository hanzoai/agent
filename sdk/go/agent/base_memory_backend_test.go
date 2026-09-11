package agent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// A Base standing in for the real one, over the same records API. Enough of it
// to hold this backend to its contract offline: the filter grammar it sends,
// the paging it walks, and the create-or-patch decision it makes.
type fakeBase struct {
	records map[string]*baseRecord
	next    int
	// Every path this backend asked for, so a test can say what it sent and
	// not only what it got back.
	asked []string
	// Runs before a create is decided, which is where a second writer lands.
	beforeCreate func()
}

var clause = regexp.MustCompile(`(\w+)='((?:[^'\\]|\\.)*)'`)

func newFakeBase() *fakeBase {
	return &fakeBase{records: map[string]*baseRecord{}}
}

func (f *fakeBase) serve() *httptest.Server {
	mux := http.NewServeMux()

	// The SPA catch-all a real Base serves for a path it does not route. It is
	// here because it is the failure this backend has to name: HTML with a 200,
	// not a 404.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte("<!doctype html><title>Base</title>"))
	})

	const prefix = "/v1/collections/agent_memory/records"
	mux.HandleFunc(prefix, func(w http.ResponseWriter, r *http.Request) {
		f.asked = append(f.asked, r.Method+" "+r.URL.String())
		switch r.Method {
		case http.MethodGet:
			f.list(w, r)
		case http.MethodPost:
			f.create(w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(prefix+"/", func(w http.ResponseWriter, r *http.Request) {
		f.asked = append(f.asked, r.Method+" "+r.URL.String())
		id := strings.TrimPrefix(r.URL.Path, prefix+"/")
		rec, ok := f.records[id]
		if !ok {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		switch r.Method {
		case http.MethodPatch:
			f.patch(w, r, rec)
		case http.MethodDelete:
			delete(f.records, id)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	return httptest.NewServer(mux)
}

func (f *fakeBase) list(w http.ResponseWriter, r *http.Request) {
	want := map[string]string{}
	for _, m := range clause.FindAllStringSubmatch(r.URL.Query().Get("filter"), -1) {
		want[m[1]] = unescape(m[2])
	}

	var hits []baseRecord
	for _, rec := range f.records {
		if want["scope"] != rec.Scope || want["scope_id"] != rec.ScopeID {
			continue
		}
		if k, ok := want["mkey"]; ok && k != rec.MKey {
			continue
		}
		hits = append(hits, *rec)
	}
	// Stable order, so a paging test is about paging and not about map order.
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0 && hits[j].MKey < hits[j-1].MKey; j-- {
			hits[j], hits[j-1] = hits[j-1], hits[j]
		}
	}

	total := len(hits)
	per, _ := strconv.Atoi(r.URL.Query().Get("perPage"))
	if per <= 0 {
		per = 30
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page <= 0 {
		page = 1
	}
	from := (page - 1) * per
	if from > total {
		from = total
	}
	to := from + per
	if to > total {
		to = total
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(baseList{Items: hits[from:to], TotalItems: total})
}

// A create under the unique index over scope, scope_id and mkey.
func (f *fakeBase) create(w http.ResponseWriter, r *http.Request) {
	var rec baseRecord
	json.NewDecoder(r.Body).Decode(&rec)
	if f.beforeCreate != nil {
		f.beforeCreate()
	}
	w.Header().Set("Content-Type", "application/json")
	for _, have := range f.records {
		if have.Scope == rec.Scope && have.ScopeID == rec.ScopeID && have.MKey == rec.MKey {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"message":"Failed to create record.","data":{"mkey":{"code":"validation_not_unique"}}}`))
			return
		}
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(f.insert(rec))
}

func (f *fakeBase) insert(rec baseRecord) *baseRecord {
	f.next++
	rec.ID = fmt.Sprintf("r%d", f.next)
	f.records[rec.ID] = &rec
	return &rec
}

// A merge, which is what PATCH means: a field the request does not carry keeps
// the value it had, and an explicit null clears it.
func (f *fakeBase) patch(w http.ResponseWriter, r *http.Request, rec *baseRecord) {
	var sent map[string]json.RawMessage
	json.NewDecoder(r.Body).Decode(&sent)

	if raw, ok := sent["value"]; ok {
		var v string
		json.Unmarshal(raw, &v)
		if v != "" {
			rec.Value = v
		}
	}
	if raw, ok := sent["embedding"]; ok {
		rec.Embedding = nil
		json.Unmarshal(raw, &rec.Embedding)
	}
	if raw, ok := sent["metadata"]; ok {
		rec.Metadata = nil
		json.Unmarshal(raw, &rec.Metadata)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rec)
}

// Undo the escaping a quoted literal carries, the way Base's tokenizer does:
// a backslash escapes whatever follows it, itself included.
func unescape(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		out.WriteByte(s[i])
	}
	return out.String()
}

func backedByFake(t *testing.T) (*BaseMemoryBackend, *fakeBase) {
	t.Helper()
	fake := newFakeBase()
	server := fake.serve()
	t.Cleanup(server.Close)
	return NewBaseMemoryBackend(server.URL, "test-token", "agent_memory"), fake
}

func TestBaseMemoryRoundTrip(t *testing.T) {
	backend, _ := backedByFake(t)

	if err := backend.Set(ScopeSession, "s1", "greeting", "hello"); err != nil {
		t.Fatalf("set: %v", err)
	}

	got, found, err := backend.Get(ScopeSession, "s1", "greeting")
	if err != nil || !found {
		t.Fatalf("get: %v found=%v", err, found)
	}
	if got != "hello" {
		t.Fatalf("got %#v, want hello", got)
	}
}

func TestBaseMemoryStructuredValue(t *testing.T) {
	backend, _ := backedByFake(t)

	want := map[string]any{"plan": []any{"read", "write"}, "depth": 2.0}
	if err := backend.Set(ScopeWorkflow, "w1", "state", want); err != nil {
		t.Fatalf("set: %v", err)
	}

	got, found, err := backend.Get(ScopeWorkflow, "w1", "state")
	if err != nil || !found {
		t.Fatalf("get: %v found=%v", err, found)
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

// A second Set replaces rather than appending, so one key is one record.
func TestBaseMemoryOverwrite(t *testing.T) {
	backend, fake := backedByFake(t)

	backend.Set(ScopeUser, "u1", "tone", "terse")
	backend.Set(ScopeUser, "u1", "tone", "plain")

	got, _, _ := backend.Get(ScopeUser, "u1", "tone")
	if got != "plain" {
		t.Fatalf("got %#v, want plain", got)
	}
	if len(fake.records) != 1 {
		t.Fatalf("%d records, want 1", len(fake.records))
	}
}

func TestBaseMemoryAbsentKey(t *testing.T) {
	backend, _ := backedByFake(t)

	got, found, err := backend.Get(ScopeGlobal, "", "never-written")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if found || got != nil {
		t.Fatalf("got %#v found=%v, want nil false", got, found)
	}
}

// Each scope is its own space: the same key in two scopes is two values, and a
// scope sees only its own.
func TestBaseMemoryScopesAreSeparate(t *testing.T) {
	backend, _ := backedByFake(t)

	backend.Set(ScopeSession, "s1", "who", "session")
	backend.Set(ScopeUser, "u1", "who", "user")

	if got, _, _ := backend.Get(ScopeSession, "s1", "who"); got != "session" {
		t.Fatalf("session scope got %#v", got)
	}
	if got, _, _ := backend.Get(ScopeUser, "u1", "who"); got != "user" {
		t.Fatalf("user scope got %#v", got)
	}
	keys, _ := backend.List(ScopeSession, "s1")
	if len(keys) != 1 {
		t.Fatalf("session scope lists %v, want one key", keys)
	}
}

func TestBaseMemoryDelete(t *testing.T) {
	backend, _ := backedByFake(t)

	backend.Set(ScopeSession, "s1", "temp", 1)
	if err := backend.Delete(ScopeSession, "s1", "temp"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, found, _ := backend.Get(ScopeSession, "s1", "temp"); found {
		t.Fatal("still there after delete")
	}
	// Deleting what is not there is what the caller asked for, not an error.
	if err := backend.Delete(ScopeSession, "s1", "temp"); err != nil {
		t.Fatalf("second delete: %v", err)
	}
}

// List walks every page. A caller asking for all the keys and getting the
// first page would read that as the whole answer.
func TestBaseMemoryListPagesToTheEnd(t *testing.T) {
	backend, _ := backedByFake(t)

	const many = 250 // more than one 200-record page
	for i := 0; i < many; i++ {
		if err := backend.Set(ScopeGlobal, "", fmt.Sprintf("k%03d", i), i); err != nil {
			t.Fatalf("set %d: %v", i, err)
		}
	}

	keys, err := backend.List(ScopeGlobal, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(keys) != many {
		t.Fatalf("listed %d keys, want %d", len(keys), many)
	}
}

func TestBaseMemoryVectorRoundTrip(t *testing.T) {
	backend, _ := backedByFake(t)

	meta := map[string]any{"kind": "note"}
	if err := backend.SetVector(ScopeSession, "s1", "a", []float64{1, 0, 0}, meta); err != nil {
		t.Fatalf("set vector: %v", err)
	}

	embedding, got, found, err := backend.GetVector(ScopeSession, "s1", "a")
	if err != nil || !found {
		t.Fatalf("get vector: %v found=%v", err, found)
	}
	if len(embedding) != 3 || embedding[0] != 1 {
		t.Fatalf("embedding %v", embedding)
	}
	if got["kind"] != "note" {
		t.Fatalf("metadata %#v", got)
	}
}

func TestBaseMemorySearchRanks(t *testing.T) {
	backend, _ := backedByFake(t)

	backend.SetVector(ScopeSession, "s1", "same", []float64{1, 0, 0}, nil)
	backend.SetVector(ScopeSession, "s1", "near", []float64{0.9, 0.4, 0}, nil)
	backend.SetVector(ScopeSession, "s1", "across", []float64{0, 1, 0}, nil)

	hits, err := backend.SearchVector(ScopeSession, "s1", []float64{1, 0, 0}, SearchOptions{})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 3 {
		t.Fatalf("%d hits, want 3", len(hits))
	}
	if hits[0].Key != "same" || hits[1].Key != "near" || hits[2].Key != "across" {
		t.Fatalf("order %s %s %s", hits[0].Key, hits[1].Key, hits[2].Key)
	}
	if hits[0].Score < 0.999 {
		t.Fatalf("an identical vector scored %v", hits[0].Score)
	}
	if hits[0].Scope != ScopeSession || hits[0].ScopeID != "s1" {
		t.Fatalf("hit does not carry its scope: %#v", hits[0])
	}
}

// The threshold cuts before the limit, so a limit of one returns the best
// match that passed rather than whatever survived out of the first one.
func TestBaseMemorySearchThresholdThenLimit(t *testing.T) {
	backend, _ := backedByFake(t)

	backend.SetVector(ScopeSession, "s1", "across", []float64{0, 1, 0}, nil)
	backend.SetVector(ScopeSession, "s1", "same", []float64{1, 0, 0}, nil)

	hits, err := backend.SearchVector(ScopeSession, "s1", []float64{1, 0, 0},
		SearchOptions{Threshold: 0.5, Limit: 1})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 || hits[0].Key != "same" {
		t.Fatalf("hits %#v", hits)
	}
}

func TestBaseMemorySearchFilters(t *testing.T) {
	backend, _ := backedByFake(t)

	backend.SetVector(ScopeSession, "s1", "note", []float64{1, 0}, map[string]any{"kind": "note"})
	backend.SetVector(ScopeSession, "s1", "task", []float64{1, 0}, map[string]any{"kind": "task"})

	hits, err := backend.SearchVector(ScopeSession, "s1", []float64{1, 0},
		SearchOptions{Filters: map[string]any{"kind": "task"}})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 || hits[0].Key != "task" {
		t.Fatalf("hits %#v", hits)
	}
}

// A vector of another width came from another embedding model. Cosine across
// the two would return a number that means nothing, so it is skipped.
func TestBaseMemorySearchSkipsOtherWidths(t *testing.T) {
	backend, _ := backedByFake(t)

	backend.SetVector(ScopeSession, "s1", "wide", []float64{1, 0, 0, 0}, nil)
	backend.SetVector(ScopeSession, "s1", "right", []float64{1, 0}, nil)

	hits, err := backend.SearchVector(ScopeSession, "s1", []float64{1, 0}, SearchOptions{})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 || hits[0].Key != "right" {
		t.Fatalf("hits %#v", hits)
	}
}

func TestBaseMemorySearchRefusesEmptyQuery(t *testing.T) {
	backend, _ := backedByFake(t)

	if _, err := backend.SearchVector(ScopeSession, "s1", nil, SearchOptions{}); err == nil {
		t.Fatal("searching with no query vector should refuse")
	}
}

// Two writers creating one key race between the read and the create, and the
// unique index refuses the second record. A Set means the last write wins, so
// the loser updates the record that won rather than failing.
func TestBaseMemoryLastWriteWinsARace(t *testing.T) {
	backend, fake := backedByFake(t)
	fake.beforeCreate = func() {
		fake.beforeCreate = nil
		fake.insert(baseRecord{Scope: "session", ScopeID: "s1", MKey: "k", Value: `"theirs"`})
	}

	if err := backend.Set(ScopeSession, "s1", "k", "ours"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got, _, _ := backend.Get(ScopeSession, "s1", "k"); got != "ours" {
		t.Fatalf("got %#v, want ours", got)
	}
	if len(fake.records) != 1 {
		t.Fatalf("%d records, want 1", len(fake.records))
	}
}

// Go names the per-user scope "user"; the control plane and the other SDKs
// name it "actor". Records are filed under the shared name so every SDK reads
// the others' records.
func TestBaseMemoryFilesUserScopeAsActor(t *testing.T) {
	backend, fake := backedByFake(t)
	if err := backend.Set(ScopeUser, "u1", "tone", "plain"); err != nil {
		t.Fatalf("set: %v", err)
	}
	for _, rec := range fake.records {
		if rec.Scope != "actor" {
			t.Fatalf("filed under %q, want actor", rec.Scope)
		}
	}
	if got, found, _ := backend.Get(ScopeUser, "u1", "tone"); !found || got != "plain" {
		t.Fatalf("got %#v found=%v", got, found)
	}
}

// A value and an embedding share one record, so neither write may erase the
// other and neither delete may take both.
func TestBaseMemoryValueAndVectorCoexist(t *testing.T) {
	backend, _ := backedByFake(t)

	backend.SetVector(ScopeSession, "s1", "doc", []float64{1, 0}, map[string]any{"kind": "note"})
	backend.Set(ScopeSession, "s1", "doc", "the text")

	if _, _, found, _ := backend.GetVector(ScopeSession, "s1", "doc"); !found {
		t.Fatal("setting the value erased the embedding")
	}

	if err := backend.DeleteVector(ScopeSession, "s1", "doc"); err != nil {
		t.Fatalf("delete vector: %v", err)
	}
	if _, _, found, _ := backend.GetVector(ScopeSession, "s1", "doc"); found {
		t.Fatal("embedding survived its delete")
	}
	got, found, _ := backend.Get(ScopeSession, "s1", "doc")
	if !found || got != "the text" {
		t.Fatalf("deleting the embedding took the value: %#v found=%v", got, found)
	}
}

// Base answers a path it does not route with its SPA, at 200. A backend
// pointed at the wrong collection has to say that, not report a parse error
// that reads like a broken server.
func TestBaseMemoryNamesAnUnroutedPath(t *testing.T) {
	fake := newFakeBase()
	server := fake.serve()
	t.Cleanup(server.Close)

	backend := NewBaseMemoryBackend(server.URL, "test-token", "wrong_collection")
	_, _, err := backend.Get(ScopeSession, "s1", "anything")
	if err == nil {
		t.Fatal("a path Base does not serve should be an error")
	}
	if !strings.Contains(err.Error(), "HTML") {
		t.Fatalf("error does not name the cause: %v", err)
	}
}

func TestBaseMemorySendsItsToken(t *testing.T) {
	var sent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[],"totalItems":0}`))
	}))
	t.Cleanup(server.Close)

	NewBaseMemoryBackend(server.URL, "shhh", "agent_memory").Get(ScopeSession, "s1", "k")
	if sent != "Bearer shhh" {
		t.Fatalf("sent %q", sent)
	}
}

// A scope id is caller data, and a quote in one must not end the clause it
// sits in.
func TestBaseMemoryQuotesFilterValues(t *testing.T) {
	backend, _ := backedByFake(t)

	odd := "tenant' && scope='global"
	if err := backend.Set(ScopeUser, odd, "k", "v"); err != nil {
		t.Fatalf("set: %v", err)
	}
	backend.Set(ScopeGlobal, "", "k", "leaked")

	got, found, err := backend.Get(ScopeUser, odd, "k")
	if err != nil || !found {
		t.Fatalf("get: %v found=%v", err, found)
	}
	if got != "v" {
		t.Fatalf("got %#v, want v", got)
	}
}

// The default collection is named rather than left to the caller, so an empty
// argument is one collection and not a path with a hole in it.
func TestBaseMemoryDefaultCollection(t *testing.T) {
	backend := NewBaseMemoryBackend("http://example.invalid/", "t", "")
	if !strings.HasSuffix(backend.records(), "/v1/collections/agent_memory/records") {
		t.Fatalf("records path %q", backend.records())
	}
	if strings.Contains(backend.records(), "//v1") {
		t.Fatalf("a trailing slash on the base url doubled: %q", backend.records())
	}
}

// It satisfies the interface the Memory type is built on. A compile-time
// check, because that is when it should fail.
var _ MemoryBackend = (*BaseMemoryBackend)(nil)

// A scope id is caller data. Base's tokenizer reads a backslash as an escape,
// so a value ending in one would escape the closing quote and end the literal
// where the caller chose rather than where this backend put it.
func TestBaseMemoryEscapesBackslashBeforeQuote(t *testing.T) {
	for _, odd := range []string{
		`ends-with-backslash\`,
		`has'quote`,
		`both\' together`,
		`\\`,
	} {
		backend, _ := backedByFake(t)
		if err := backend.Set(ScopeUser, odd, "k", "mine"); err != nil {
			t.Fatalf("set %q: %v", odd, err)
		}
		backend.Set(ScopeGlobal, "", "k", "not mine")

		got, found, err := backend.Get(ScopeUser, odd, "k")
		if err != nil || !found {
			t.Fatalf("get %q: %v found=%v", odd, err, found)
		}
		if got != "mine" {
			t.Fatalf("scope id %q read back %#v", odd, got)
		}
	}
}

// The clause this backend writes is exactly what Base's grammar parses: a
// single-quoted literal per field, joined by &&.
func TestBaseMemoryFilterShape(t *testing.T) {
	if got := filterFor(ScopeSession, "s1", "k"); got != "scope='session' && scope_id='s1' && mkey='k'" {
		t.Fatalf("filter %q", got)
	}
	// No key means the whole scope, so the clause must not pin one.
	if got := filterFor(ScopeGlobal, "", ""); got != "scope='global' && scope_id=''" {
		t.Fatalf("scope filter %q", got)
	}
	if got := quote(`a\`); got != `'a\\'` {
		t.Fatalf("quote(%q) = %s", `a\`, got)
	}
}
