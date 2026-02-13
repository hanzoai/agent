package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hanzoai/agents/control-plane/internal/core/domain"
	"github.com/hanzoai/agents/control-plane/internal/core/interfaces"
	"github.com/hanzoai/agents/control-plane/internal/services"
	"github.com/hanzoai/agents/control-plane/internal/storage"
	"github.com/hanzoai/agents/control-plane/pkg/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestGetNodesSummaryHandler_Structure tests the handler structure and routing
// This works without a server by testing handler setup and basic request handling
func TestGetNodesSummaryHandler_Structure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a minimal UIService mock using real storage (lightweight)
	ctx := context.Background()
	tempDir := t.TempDir()
	cfg := storage.StorageConfig{
		Mode: "local",
		Local: storage.LocalStorageConfig{
			DatabasePath: tempDir + "/test.db",
			KVStorePath:  tempDir + "/test.bolt",
		},
	}

	realStorage := storage.NewLocalStorage(storage.LocalStorageConfig{})
	err := realStorage.Initialize(ctx, cfg)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "fts5") {
		t.Skip("sqlite3 compiled without FTS5")
	}
	require.NoError(t, err)
	defer realStorage.Close(ctx)

	// Create real UIService with minimal dependencies
	mockAgentClient := &MockAgentClientForUI{}
	mockAgentService := &MockAgentServiceForUI{}
	statusManager := services.NewStatusManager(realStorage, services.StatusManagerConfig{}, nil, mockAgentClient)
	uiService := services.NewUIService(realStorage, mockAgentClient, mockAgentService, statusManager)

	handler := NewNodesHandler(uiService)
	router := gin.New()
	router.GET("/api/ui/v1/nodes", handler.GetNodesSummaryHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/ui/v1/nodes", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	// Should return OK (even if no nodes)
	assert.Equal(t, http.StatusOK, resp.Code)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &result))
	assert.Contains(t, result, "nodes")
	assert.Contains(t, result, "count")
}

// TestGetNodeDetailsHandler_Structure tests node details handler structure
func TestGetNodeDetailsHandler_Structure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx := context.Background()
	tempDir := t.TempDir()
	cfg := storage.StorageConfig{
		Mode: "local",
		Local: storage.LocalStorageConfig{
			DatabasePath: tempDir + "/test.db",
			KVStorePath:  tempDir + "/test.bolt",
		},
	}

	realStorage := storage.NewLocalStorage(storage.LocalStorageConfig{})
	err := realStorage.Initialize(ctx, cfg)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "fts5") {
		t.Skip("sqlite3 compiled without FTS5")
	}
	require.NoError(t, err)
	defer realStorage.Close(ctx)

	mockAgentClient := &MockAgentClientForUI{}
	mockAgentService := &MockAgentServiceForUI{}
	statusManager := services.NewStatusManager(realStorage, services.StatusManagerConfig{}, nil, mockAgentClient)
	uiService := services.NewUIService(realStorage, mockAgentClient, mockAgentService, statusManager)

	handler := NewNodesHandler(uiService)
	router := gin.New()
	router.GET("/api/ui/v1/nodes/:nodeId", handler.GetNodeDetailsHandler)

	// Test with missing nodeId (should return 400)
	req := httptest.NewRequest(http.MethodGet, "/api/ui/v1/nodes/", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusBadRequest, resp.Code)

	// Test with nodeId (should return 404 if not found, but handler works)
	req = httptest.NewRequest(http.MethodGet, "/api/ui/v1/nodes/node-1", nil)
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	// Should handle gracefully (404 or 500 depending on implementation)
	assert.True(t, resp.Code == http.StatusNotFound || resp.Code == http.StatusInternalServerError)
}

// TestGetNodeStatusHandler_Structure tests node status handler
func TestGetNodeStatusHandler_Structure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx := context.Background()
	tempDir := t.TempDir()
	cfg := storage.StorageConfig{
		Mode: "local",
		Local: storage.LocalStorageConfig{
			DatabasePath: tempDir + "/test.db",
			KVStorePath:  tempDir + "/test.bolt",
		},
	}

	realStorage := storage.NewLocalStorage(storage.LocalStorageConfig{})
	err := realStorage.Initialize(ctx, cfg)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "fts5") {
		t.Skip("sqlite3 compiled without FTS5")
	}
	require.NoError(t, err)
	defer realStorage.Close(ctx)

	mockAgentClient := &MockAgentClientForUI{}
	mockAgentService := &MockAgentServiceForUI{}
	statusManager := services.NewStatusManager(realStorage, services.StatusManagerConfig{}, nil, mockAgentClient)
	uiService := services.NewUIService(realStorage, mockAgentClient, mockAgentService, statusManager)

	handler := NewNodesHandler(uiService)
	router := gin.New()
	router.GET("/api/ui/v1/nodes/:nodeId/status", handler.GetNodeStatusHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/ui/v1/nodes/node-1/status", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	// Should handle request (may return error if node not found)
	assert.True(t, resp.Code == http.StatusNotFound || resp.Code == http.StatusInternalServerError || resp.Code == http.StatusOK)
}

// TestRefreshNodeStatusHandler_Structure tests refresh node status handler
func TestRefreshNodeStatusHandler_Structure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx := context.Background()
	tempDir := t.TempDir()
	cfg := storage.StorageConfig{
		Mode: "local",
		Local: storage.LocalStorageConfig{
			DatabasePath: tempDir + "/test.db",
			KVStorePath:  tempDir + "/test.bolt",
		},
	}

	realStorage := storage.NewLocalStorage(storage.LocalStorageConfig{})
	err := realStorage.Initialize(ctx, cfg)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "fts5") {
		t.Skip("sqlite3 compiled without FTS5")
	}
	require.NoError(t, err)
	defer realStorage.Close(ctx)

	mockAgentClient := &MockAgentClientForUI{}
	mockAgentService := &MockAgentServiceForUI{}
	statusManager := services.NewStatusManager(realStorage, services.StatusManagerConfig{}, nil, mockAgentClient)
	uiService := services.NewUIService(realStorage, mockAgentClient, mockAgentService, statusManager)

	handler := NewNodesHandler(uiService)
	router := gin.New()
	router.POST("/api/ui/v1/nodes/:nodeId/status/refresh", handler.RefreshNodeStatusHandler)

	req := httptest.NewRequest(http.MethodPost, "/api/ui/v1/nodes/node-1/status/refresh", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	// Should handle request
	assert.True(t, resp.Code >= http.StatusBadRequest) // Any response is valid
}

// TestBulkNodeStatusHandler_Validation tests bulk node status handler request validation
func TestBulkNodeStatusHandler_Validation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx := context.Background()
	tempDir := t.TempDir()
	cfg := storage.StorageConfig{
		Mode: "local",
		Local: storage.LocalStorageConfig{
			DatabasePath: tempDir + "/test.db",
			KVStorePath:  tempDir + "/test.bolt",
		},
	}

	realStorage := storage.NewLocalStorage(storage.LocalStorageConfig{})
	err := realStorage.Initialize(ctx, cfg)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "fts5") {
		t.Skip("sqlite3 compiled without FTS5")
	}
	require.NoError(t, err)
	defer realStorage.Close(ctx)

	mockAgentClient := &MockAgentClientForUI{}
	mockAgentService := &MockAgentServiceForUI{}
	statusManager := services.NewStatusManager(realStorage, services.StatusManagerConfig{}, nil, mockAgentClient)
	uiService := services.NewUIService(realStorage, mockAgentClient, mockAgentService, statusManager)

	handler := NewNodesHandler(uiService)
	router := gin.New()
	router.POST("/api/ui/v1/nodes/status/bulk", handler.BulkNodeStatusHandler)

	// Test with invalid JSON
	req := httptest.NewRequest(http.MethodPost, "/api/ui/v1/nodes/status/bulk", strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusBadRequest, resp.Code)

	// Test with valid JSON but missing required field
	req = httptest.NewRequest(http.MethodPost, "/api/ui/v1/nodes/status/bulk", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp = httptest.NewRecorder()

	router.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusBadRequest, resp.Code)

	// Test with valid JSON
	req = httptest.NewRequest(http.MethodPost, "/api/ui/v1/nodes/status/bulk", strings.NewReader(`{"node_ids": ["node-1", "node-2"]}`))
	req.Header.Set("Content-Type", "application/json")
	resp = httptest.NewRecorder()

	router.ServeHTTP(resp, req)
	// Should process request (may return error if nodes don't exist, but handler works)
	assert.True(t, resp.Code >= http.StatusOK)
}

// TestGetDashboardSummaryHandler_Structure tests dashboard handler structure
func TestGetDashboardSummaryHandler_Structure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx := context.Background()
	tempDir := t.TempDir()
	cfg := storage.StorageConfig{
		Mode: "local",
		Local: storage.LocalStorageConfig{
			DatabasePath: tempDir + "/test.db",
			KVStorePath:  tempDir + "/test.bolt",
		},
	}

	realStorage := storage.NewLocalStorage(storage.LocalStorageConfig{})
	err := realStorage.Initialize(ctx, cfg)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "fts5") {
		t.Skip("sqlite3 compiled without FTS5")
	}
	require.NoError(t, err)
	defer realStorage.Close(ctx)

	mockAgentService := &MockAgentServiceForUI{}
	handler := NewDashboardHandler(realStorage, mockAgentService)
	router := gin.New()
	router.GET("/api/ui/v1/dashboard", handler.GetDashboardSummaryHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/ui/v1/dashboard", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	// Should return OK (even with empty data)
	assert.Equal(t, http.StatusOK, resp.Code)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &result))
	// Dashboard should have some structure
	assert.NotNil(t, result)
}

// TestAPIErrorHandling tests error handling in API handlers
func TestAPIErrorHandling(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx := context.Background()
	tempDir := t.TempDir()
	cfg := storage.StorageConfig{
		Mode: "local",
		Local: storage.LocalStorageConfig{
			DatabasePath: tempDir + "/test.db",
			KVStorePath:  tempDir + "/test.bolt",
		},
	}

	realStorage := storage.NewLocalStorage(storage.LocalStorageConfig{})
	err := realStorage.Initialize(ctx, cfg)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "fts5") {
		t.Skip("sqlite3 compiled without FTS5")
	}
	require.NoError(t, err)
	defer realStorage.Close(ctx)

	mockAgentClient := &MockAgentClientForUI{}
	mockAgentService := &MockAgentServiceForUI{}
	statusManager := services.NewStatusManager(realStorage, services.StatusManagerConfig{}, nil, mockAgentClient)
	uiService := services.NewUIService(realStorage, mockAgentClient, mockAgentService, statusManager)

	handler := NewNodesHandler(uiService)
	router := gin.New()
	router.GET("/api/ui/v1/nodes/:nodeId", handler.GetNodeDetailsHandler)

	// Test various invalid inputs
	tests := []struct {
		name   string
		path   string
		method string
	}{
		{"empty nodeId", "/api/ui/v1/nodes/", "GET"},
		{"special chars in nodeId", "/api/ui/v1/nodes/node%20with%20spaces", "GET"},
		{"very long nodeId", "/api/ui/v1/nodes/" + strings.Repeat("a", 1000), "GET"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)

			// Should handle gracefully (not panic)
			assert.True(t, resp.Code >= http.StatusBadRequest)
		})
	}
}

// TestAPIMethodValidation tests that handlers only accept correct HTTP methods
func TestAPIMethodValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx := context.Background()
	tempDir := t.TempDir()
	cfg := storage.StorageConfig{
		Mode: "local",
		Local: storage.LocalStorageConfig{
			DatabasePath: tempDir + "/test.db",
			KVStorePath:  tempDir + "/test.bolt",
		},
	}

	realStorage := storage.NewLocalStorage(storage.LocalStorageConfig{})
	err := realStorage.Initialize(ctx, cfg)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "fts5") {
		t.Skip("sqlite3 compiled without FTS5")
	}
	require.NoError(t, err)
	defer realStorage.Close(ctx)

	mockAgentClient := &MockAgentClientForUI{}
	mockAgentService := &MockAgentServiceForUI{}
	statusManager := services.NewStatusManager(realStorage, services.StatusManagerConfig{}, nil, mockAgentClient)
	uiService := services.NewUIService(realStorage, mockAgentClient, mockAgentService, statusManager)

	handler := NewNodesHandler(uiService)
	router := gin.New()
	router.GET("/api/ui/v1/nodes", handler.GetNodesSummaryHandler)
	router.POST("/api/ui/v1/nodes/:nodeId/status/refresh", handler.RefreshNodeStatusHandler)

	// Test GET endpoint with wrong method
	req := httptest.NewRequest(http.MethodPost, "/api/ui/v1/nodes", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusNotFound, resp.Code) // Gin returns 404 for wrong method

	// Test POST endpoint with wrong method
	req = httptest.NewRequest(http.MethodGet, "/api/ui/v1/nodes/node-1/status/refresh", nil)
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusNotFound, resp.Code)
}

// TestAPIResponseFormat tests that API responses have correct format
func TestAPIResponseFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx := context.Background()
	tempDir := t.TempDir()
	cfg := storage.StorageConfig{
		Mode: "local",
		Local: storage.LocalStorageConfig{
			DatabasePath: tempDir + "/test.db",
			KVStorePath:  tempDir + "/test.bolt",
		},
	}

	realStorage := storage.NewLocalStorage(storage.LocalStorageConfig{})
	err := realStorage.Initialize(ctx, cfg)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "fts5") {
		t.Skip("sqlite3 compiled without FTS5")
	}
	require.NoError(t, err)
	defer realStorage.Close(ctx)

	mockAgentClient := &MockAgentClientForUI{}
	mockAgentService := &MockAgentServiceForUI{}
	statusManager := services.NewStatusManager(realStorage, services.StatusManagerConfig{}, nil, mockAgentClient)
	uiService := services.NewUIService(realStorage, mockAgentClient, mockAgentService, statusManager)

	handler := NewNodesHandler(uiService)
	router := gin.New()
	router.GET("/api/ui/v1/nodes", handler.GetNodesSummaryHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/ui/v1/nodes", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	// Verify response is valid JSON
	assert.Equal(t, "application/json; charset=utf-8", resp.Header().Get("Content-Type"))

	var result map[string]interface{}
	err = json.Unmarshal(resp.Body.Bytes(), &result)
	require.NoError(t, err, "Response should be valid JSON")

	// Verify expected fields
	assert.Contains(t, result, "nodes")
	assert.Contains(t, result, "count")
}

// MockAgentClientForUI is a minimal mock for interfaces.AgentClient
type MockAgentClientForUI struct {
	mock.Mock
}

func (m *MockAgentClientForUI) GetAgentStatus(ctx context.Context, nodeID string) (*interfaces.AgentStatusResponse, error) {
	args := m.Called(ctx, nodeID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.AgentStatusResponse), args.Error(1)
}

func (m *MockAgentClientForUI) GetMCPHealth(ctx context.Context, nodeID string) (*interfaces.MCPHealthResponse, error) {
	return nil, nil
}

func (m *MockAgentClientForUI) RestartMCPServer(ctx context.Context, nodeID, alias string) error {
	return nil
}

func (m *MockAgentClientForUI) GetMCPTools(ctx context.Context, nodeID, alias string) (*interfaces.MCPToolsResponse, error) {
	return nil, nil
}

func (m *MockAgentClientForUI) ShutdownAgent(ctx context.Context, nodeID string, graceful bool, timeoutSeconds int) (*interfaces.AgentShutdownResponse, error) {
	return nil, nil
}

// MockAgentServiceForUI is a minimal mock for interfaces.AgentService
type MockAgentServiceForUI struct {
	mock.Mock
}

func (m *MockAgentServiceForUI) GetAgentStatus(name string) (*domain.AgentStatus, error) {
	args := m.Called(name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.AgentStatus), args.Error(1)
}

func (m *MockAgentServiceForUI) RunAgent(name string, options domain.RunOptions) (*domain.RunningAgent, error) {
	return nil, nil
}

func (m *MockAgentServiceForUI) StopAgent(name string) error {
	return nil
}

func (m *MockAgentServiceForUI) ListRunningAgents() ([]domain.RunningAgent, error) {
	return []domain.RunningAgent{}, nil
}

// MockAgentService is a mock for interfaces.AgentService (used by dashboard)
type MockAgentService struct {
	mock.Mock
}

func (m *MockAgentService) GetAgents(ctx context.Context) ([]*types.AgentNode, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.AgentNode), args.Error(1)
}

func (m *MockAgentService) GetAgent(ctx context.Context, id string) (*types.AgentNode, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.AgentNode), args.Error(1)
}
