package communication

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hanzoai/agents/control-plane/internal/core/interfaces"
	"github.com/hanzoai/agents/control-plane/internal/storage"
	"github.com/hanzoai/agents/control-plane/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestStorage(t *testing.T, ctx context.Context) storage.StorageProvider {
	t.Helper()

	tempDir := t.TempDir()
	cfg := storage.StorageConfig{
		Mode: "local",
		Local: storage.LocalStorageConfig{
			DatabasePath: filepath.Join(tempDir, "test.db"),
			KVStorePath:  filepath.Join(tempDir, "test.bolt"),
		},
	}

	provider := storage.NewLocalStorage(storage.LocalStorageConfig{})
	if err := provider.Initialize(ctx, cfg); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "fts5") {
			t.Skip("sqlite3 compiled without FTS5 support")
		}
		require.NoError(t, err)
	}

	t.Cleanup(func() {
		_ = provider.Close(ctx)
	})

	return provider
}

func registerAgent(t *testing.T, ctx context.Context, provider storage.StorageProvider, baseURL string) string {
	t.Helper()

	agent := &types.AgentNode{
		ID:              "agent-test",
		TeamID:          "team-1",
		BaseURL:         baseURL,
		Version:         "1.0.0",
		HealthStatus:    types.HealthStatusActive,
		LifecycleStatus: types.AgentStatusReady,
		LastHeartbeat:   time.Now(),
		RegisteredAt:    time.Now(),
	}

	require.NoError(t, provider.RegisterAgent(ctx, agent))
	return agent.ID
}

func TestHTTPAgentClient_GetMCPHealthCachesAndExpires(t *testing.T) {
	ctx := context.Background()

	var healthCalls int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&healthCalls, 1)
		assert.Equal(t, "/health/mcp", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(interfaces.MCPHealthResponse{
			Servers: []interfaces.MCPServerHealth{
				{
					Alias:       "default",
					Status:      "running",
					ToolCount:   2,
					SuccessRate: 0.95,
				},
			},
			Summary: interfaces.MCPSummary{
				TotalServers:   1,
				RunningServers: 1,
				TotalTools:     2,
				OverallHealth:  1.0,
			},
		}))
	}))
	defer server.Close()

	provider := setupTestStorage(t, ctx)
	agentID := registerAgent(t, ctx, provider, server.URL)
	client := NewHTTPAgentClient(provider, time.Second)

	resp1, err := client.GetMCPHealth(ctx, agentID)
	require.NoError(t, err)
	assert.Equal(t, 1, int(atomic.LoadInt64(&healthCalls)))
	assert.Equal(t, 1, resp1.Summary.TotalServers)

	resp2, err := client.GetMCPHealth(ctx, agentID)
	require.NoError(t, err)
	assert.Equal(t, 1, int(atomic.LoadInt64(&healthCalls)), "second call should hit cache")
	assert.Equal(t, resp1.Summary, resp2.Summary)

	stats := client.GetCacheStats()
	require.Equal(t, 1, stats["total_entries"])

	client.cacheMutex.Lock()
	cached := client.cache[agentID]
	require.NotNil(t, cached)
	cached.Timestamp = time.Now().Add(-31 * time.Second)
	client.cacheMutex.Unlock()

	client.CleanupExpiredCache()

	stats = client.GetCacheStats()
	require.Equal(t, 0, stats["total_entries"])

	_, err = client.GetMCPHealth(ctx, agentID)
	require.NoError(t, err)
	assert.Equal(t, 2, int(atomic.LoadInt64(&healthCalls)), "cache expiration should trigger a fresh call")
}

func TestHTTPAgentClient_GetMCPHealthHandlesNotFound(t *testing.T) {
	ctx := context.Background()

	var healthCalls int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&healthCalls, 1)
		http.NotFound(w, r)
	}))
	defer server.Close()

	provider := setupTestStorage(t, ctx)
	agentID := registerAgent(t, ctx, provider, server.URL)
	client := NewHTTPAgentClient(provider, time.Second)

	resp, err := client.GetMCPHealth(ctx, agentID)
	require.NoError(t, err)
	assert.Equal(t, 1, int(atomic.LoadInt64(&healthCalls)))
	assert.NotNil(t, resp)
	assert.Equal(t, 0, resp.Summary.TotalServers)
	assert.Equal(t, 1.0, resp.Summary.OverallHealth)
	assert.Empty(t, resp.Servers)
}

func TestHTTPAgentClient_RestartMCPServerInvalidatesCache(t *testing.T) {
	ctx := context.Background()

	var healthCalls, restartCalls int64
	mux := http.NewServeMux()
	mux.HandleFunc("/health/mcp", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&healthCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(interfaces.MCPHealthResponse{
			Servers: []interfaces.MCPServerHealth{},
			Summary: interfaces.MCPSummary{TotalServers: 0, OverallHealth: 1.0},
		}))
	})
	mux.HandleFunc("/mcp/servers/search/restart", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&restartCalls, 1)
		assert.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(interfaces.MCPRestartResponse{
			Success: true,
			Message: "restarted",
		}))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	provider := setupTestStorage(t, ctx)
	agentID := registerAgent(t, ctx, provider, server.URL)
	client := NewHTTPAgentClient(provider, time.Second)

	_, err := client.GetMCPHealth(ctx, agentID)
	require.NoError(t, err)

	client.cacheMutex.RLock()
	_, exists := client.cache[agentID]
	client.cacheMutex.RUnlock()
	require.True(t, exists)

	err = client.RestartMCPServer(ctx, agentID, "search")
	require.NoError(t, err)
	assert.Equal(t, 1, int(atomic.LoadInt64(&restartCalls)))

	client.cacheMutex.RLock()
	_, exists = client.cache[agentID]
	client.cacheMutex.RUnlock()
	assert.False(t, exists, "cache should be invalidated after restart")

	_, err = client.GetMCPHealth(ctx, agentID)
	require.NoError(t, err)
	assert.Equal(t, 2, int(atomic.LoadInt64(&healthCalls)), "health endpoint should be called again after restart")
}

func TestHTTPAgentClient_RestartMCPServerPropagatesFailure(t *testing.T) {
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(interfaces.MCPRestartResponse{
			Success: false,
			Message: "unable to restart",
		}))
	}))
	defer server.Close()

	provider := setupTestStorage(t, ctx)
	agentID := registerAgent(t, ctx, provider, server.URL)
	client := NewHTTPAgentClient(provider, time.Second)

	err := client.RestartMCPServer(ctx, agentID, "search")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unable to restart")
}

type flakyTransport struct {
	attempts int32
}

func (ft *flakyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	attempt := atomic.AddInt32(&ft.attempts, 1)
	if attempt == 1 {
		return nil, errors.New("connection reset by peer")
	}

	recorder := httptest.NewRecorder()
	recorder.Code = http.StatusOK
	recorder.Header().Set("Content-Type", "application/json")
	recorder.Body.WriteString(`{
		"status":"running",
		"uptime":"1s",
		"uptime_seconds":1,
		"pid":123,
		"version":"1.0.0",
		"node_id":"agent-test",
		"last_activity":"2024-01-01T00:00:00Z",
		"resources":{}
	}`)
	return recorder.Result(), nil
}

func TestHTTPAgentClient_GetAgentStatusRetriesNetworkErrors(t *testing.T) {
	ctx := context.Background()

	provider := setupTestStorage(t, ctx)
	agentID := registerAgent(t, ctx, provider, "http://agent.local")
	client := NewHTTPAgentClient(provider, 0)

	flaky := &flakyTransport{}
	client.httpClient.Transport = flaky
	client.httpClient.Timeout = 0

	resp, err := client.GetAgentStatus(ctx, agentID)
	require.NoError(t, err)
	assert.Equal(t, "running", resp.Status)
	assert.Equal(t, int32(2), atomic.LoadInt32(&flaky.attempts), "client should retry once after transient network failure")
}

type storageOverride struct {
	storage.StorageProvider
	override func(ctx context.Context, id string) (*types.AgentNode, error)
}

func (s *storageOverride) GetAgent(ctx context.Context, id string) (*types.AgentNode, error) {
	if s.override != nil {
		return s.override(ctx, id)
	}
	return s.StorageProvider.GetAgent(ctx, id)
}

func TestHTTPAgentClient_GetAgentStatusHandlesMissingAgents(t *testing.T) {
	ctx := context.Background()

	provider := setupTestStorage(t, ctx)
	agentID := registerAgent(t, ctx, provider, "http://agent.local")

	override := &storageOverride{
		StorageProvider: provider,
		override: func(ctx context.Context, id string) (*types.AgentNode, error) {
			return nil, nil
		},
	}

	client := NewHTTPAgentClient(override, time.Second)

	_, err := client.GetAgentStatus(ctx, agentID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestHTTPAgentClient_GetAgentStatusRejectsMismatchedNodeID(t *testing.T) {
	ctx := context.Background()

	// Fake agent that returns a different node_id than requested
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"status":"running",
			"uptime":"1s",
			"uptime_seconds":1,
			"pid":123,
			"version":"1.0.0",
			"node_id":"other-agent",
			"last_activity":"2024-01-01T00:00:00Z",
			"resources":{}
		}`))
	}))
	defer server.Close()

	provider := setupTestStorage(t, ctx)
	agentID := registerAgent(t, ctx, provider, server.URL)
	client := NewHTTPAgentClient(provider, time.Second)

	_, err := client.GetAgentStatus(ctx, agentID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent ID mismatch")
}
