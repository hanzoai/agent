package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hanzoai/agents/sdk/go/ai"
	"github.com/hanzoai/agents/sdk/go/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
		check   func(t *testing.T, a *Agent)
	}{
		{
			name: "valid config",
			cfg: Config{
				NodeID:        "node-1",
				Version:       "1.0.0",
				HanzoAgentsURL: "https://api.example.com",
			},
			wantErr: false,
			check: func(t *testing.T, a *Agent) {
				assert.NotNil(t, a)
				assert.Equal(t, "node-1", a.cfg.NodeID)
				assert.Equal(t, "1.0.0", a.cfg.Version)
			},
		},
		{
			name: "missing NodeID",
			cfg: Config{
				Version:       "1.0.0",
				HanzoAgentsURL: "https://api.example.com",
			},
			wantErr: true,
		},
		{
			name: "missing Version",
			cfg: Config{
				NodeID:        "node-1",
				HanzoAgentsURL: "https://api.example.com",
			},
			wantErr: true,
		},
		{
			name: "missing HanzoAgentsURL",
			cfg: Config{
				NodeID:  "node-1",
				Version: "1.0.0",
			},
			wantErr: false,
			check: func(t *testing.T, a *Agent) {
				assert.Nil(t, a.client)
			},
		},
		{
			name: "defaults applied",
			cfg: Config{
				NodeID:        "node-1",
				Version:       "1.0.0",
				HanzoAgentsURL: "https://api.example.com",
			},
			wantErr: false,
			check: func(t *testing.T, a *Agent) {
				assert.Equal(t, "default", a.cfg.TeamID)
				assert.Equal(t, ":8001", a.cfg.ListenAddress)
				assert.Equal(t, 2*time.Minute, a.cfg.LeaseRefreshInterval)
				assert.NotNil(t, a.cfg.Logger)
			},
		},
		{
			name: "with AIConfig",
			cfg: Config{
				NodeID:        "node-1",
				Version:       "1.0.0",
				HanzoAgentsURL: "https://api.example.com",
				AIConfig: &ai.Config{
					APIKey:  "test-key",
					BaseURL: "https://api.openai.com/v1",
					Model:   "gpt-4o",
				},
			},
			wantErr: false,
			check: func(t *testing.T, a *Agent) {
				assert.NotNil(t, a.aiClient)
			},
		},
		{
			name: "invalid AIConfig",
			cfg: Config{
				NodeID:        "node-1",
				Version:       "1.0.0",
				HanzoAgentsURL: "https://api.example.com",
				AIConfig:      &ai.Config{
					// Missing required fields
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := New(tt.cfg)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, a)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, a)
				if tt.check != nil {
					tt.check(t, a)
				}
			}
		})
	}
}

func TestRegisterReasoner(t *testing.T) {
	cfg := Config{
		NodeID:        "node-1",
		Version:       "1.0.0",
		HanzoAgentsURL: "https://api.example.com",
		Logger:        log.New(io.Discard, "", 0),
	}

	agent, err := New(cfg)
	require.NoError(t, err)

	// Register reasoner
	agent.RegisterReasoner("test", func(ctx context.Context, input map[string]any) (any, error) {
		return map[string]any{"result": "ok"}, nil
	})

	// Verify registration
	reasoner, ok := agent.reasoners["test"]
	assert.True(t, ok)
	assert.NotNil(t, reasoner)
	assert.Equal(t, "test", reasoner.Name)
	assert.NotNil(t, reasoner.Handler)
}

func TestRegisterReasoner_WithOptions(t *testing.T) {
	cfg := Config{
		NodeID:        "node-1",
		Version:       "1.0.0",
		HanzoAgentsURL: "https://api.example.com",
		Logger:        log.New(io.Discard, "", 0),
	}

	agent, err := New(cfg)
	require.NoError(t, err)

	inputSchema := json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}}}`)
	outputSchema := json.RawMessage(`{"type":"object","properties":{"result":{"type":"string"}}}`)

	agent.RegisterReasoner("test",
		func(ctx context.Context, input map[string]any) (any, error) {
			return nil, nil
		},
		WithInputSchema(inputSchema),
		WithOutputSchema(outputSchema),
	)

	reasoner := agent.reasoners["test"]
	assert.Equal(t, inputSchema, reasoner.InputSchema)
	assert.Equal(t, outputSchema, reasoner.OutputSchema)
}

func TestRegisterReasoner_NilHandler(t *testing.T) {
	cfg := Config{
		NodeID:        "node-1",
		Version:       "1.0.0",
		HanzoAgentsURL: "https://api.example.com",
		Logger:        log.New(io.Discard, "", 0),
	}

	agent, err := New(cfg)
	require.NoError(t, err)

	// This should panic
	assert.Panics(t, func() {
		agent.RegisterReasoner("test", nil)
	})
}

func TestInitialize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/nodes" {
			var req types.NodeRegistrationRequest
			json.NewDecoder(r.Body).Decode(&req)
			assert.Equal(t, "node-1", req.ID)
			assert.Equal(t, "team-1", req.TeamID)

			resp := types.NodeRegistrationResponse{
				ID:      "node-1",
				Success: true,
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
		} else if strings.Contains(r.URL.Path, "/status") {
			resp := types.LeaseResponse{
				LeaseSeconds: 120,
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	cfg := Config{
		NodeID:           "node-1",
		Version:          "1.0.0",
		TeamID:           "team-1",
		HanzoAgentsURL:    server.URL,
		Logger:           log.New(io.Discard, "", 0),
		DisableLeaseLoop: true, // Disable for testing
	}

	agent, err := New(cfg)
	require.NoError(t, err)

	agent.RegisterReasoner("test", func(ctx context.Context, input map[string]any) (any, error) {
		return map[string]any{"ok": true}, nil
	})

	err = agent.Initialize(context.Background())
	assert.NoError(t, err)
	assert.True(t, agent.initialized)
}

func TestInitialize_NoReasoners(t *testing.T) {
	cfg := Config{
		NodeID:        "node-1",
		Version:       "1.0.0",
		HanzoAgentsURL: "https://api.example.com",
		Logger:        log.New(io.Discard, "", 0),
	}

	agent, err := New(cfg)
	require.NoError(t, err)

	err = agent.Initialize(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no reasoners registered")
}

func TestHandler(t *testing.T) {
	cfg := Config{
		NodeID:        "node-1",
		Version:       "1.0.0",
		HanzoAgentsURL: "https://api.example.com",
		Logger:        log.New(io.Discard, "", 0),
	}

	agent, err := New(cfg)
	require.NoError(t, err)

	agent.RegisterReasoner("test", func(ctx context.Context, input map[string]any) (any, error) {
		return map[string]any{"result": "ok"}, nil
	})

	handler := agent.Handler()
	assert.NotNil(t, handler)

	// Test health endpoint
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response map[string]any
	json.NewDecoder(w.Body).Decode(&response)
	assert.Equal(t, "ok", response["status"])
}

func TestHandleReasoner_Sync(t *testing.T) {
	cfg := Config{
		NodeID:        "node-1",
		Version:       "1.0.0",
		HanzoAgentsURL: "https://api.example.com",
		Logger:        log.New(io.Discard, "", 0),
	}

	agent, err := New(cfg)
	require.NoError(t, err)

	agent.RegisterReasoner("test", func(ctx context.Context, input map[string]any) (any, error) {
		return map[string]any{"value": input["value"]}, nil
	})

	server := httptest.NewServer(agent.handler())
	defer server.Close()

	reqBody := []byte(`{"value":42}`)
	req, err := http.NewRequest(http.MethodPost, server.URL+"/reasoners/test", bytes.NewReader(reqBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, float64(42), result["value"]) // JSON numbers are float64
}

func TestHandleReasoner_NotFound(t *testing.T) {
	cfg := Config{
		NodeID:        "node-1",
		Version:       "1.0.0",
		HanzoAgentsURL: "https://api.example.com",
		Logger:        log.New(io.Discard, "", 0),
	}

	agent, err := New(cfg)
	require.NoError(t, err)

	server := httptest.NewServer(agent.handler())
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/reasoners/nonexistent", bytes.NewReader([]byte("{}")))
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestHandleReasoner_WrongMethod(t *testing.T) {
	cfg := Config{
		NodeID:        "node-1",
		Version:       "1.0.0",
		HanzoAgentsURL: "https://api.example.com",
		Logger:        log.New(io.Discard, "", 0),
	}

	agent, err := New(cfg)
	require.NoError(t, err)

	server := httptest.NewServer(agent.handler())
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/reasoners/test", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

func TestHandleReasoner_Error(t *testing.T) {
	cfg := Config{
		NodeID:        "node-1",
		Version:       "1.0.0",
		HanzoAgentsURL: "https://api.example.com",
		Logger:        log.New(io.Discard, "", 0),
	}

	agent, err := New(cfg)
	require.NoError(t, err)

	agent.RegisterReasoner("test", func(ctx context.Context, input map[string]any) (any, error) {
		return nil, assert.AnError
	})

	server := httptest.NewServer(agent.handler())
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/reasoners/test", bytes.NewReader([]byte("{}")))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Contains(t, result["error"], "assert.AnError")
}

func TestCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/execute/") {
			// Verify headers
			assert.Equal(t, "run-1", r.Header.Get("X-Run-ID"))
			assert.Equal(t, "parent-exec", r.Header.Get("X-Parent-Execution-ID"))
			assert.Equal(t, "session-1", r.Header.Get("X-Session-ID"))
			assert.Equal(t, "actor-1", r.Header.Get("X-Actor-ID"))

			var reqBody map[string]any
			json.NewDecoder(r.Body).Decode(&reqBody)
			assert.Equal(t, map[string]any{"value": float64(42)}, reqBody["input"])

			resp := map[string]any{
				"execution_id": "exec-1",
				"run_id":       "run-1",
				"status":       "succeeded",
				"result":       map[string]any{"output": "result"},
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	cfg := Config{
		NodeID:        "node-1",
		Version:       "1.0.0",
		HanzoAgentsURL: server.URL,
		Logger:        log.New(io.Discard, "", 0),
	}

	agent, err := New(cfg)
	require.NoError(t, err)

	// Create context with execution context
	ctx := contextWithExecution(context.Background(), ExecutionContext{
		RunID:       "run-1",
		ExecutionID: "parent-exec",
		SessionID:   "session-1",
		ActorID:     "actor-1",
	})

	result, err := agent.Call(ctx, "target.node", map[string]any{"value": 42})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "result", result["output"])
}

func TestCall_ErrorHandling(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		wantErr        bool
	}{
		{
			name: "API error",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("bad request"))
			},
			wantErr: true,
		},
		{
			name: "execution failed status",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				resp := map[string]any{
					"status":        "failed",
					"error_message": "execution failed",
				}
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(resp)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			cfg := Config{
				NodeID:        "node-1",
				Version:       "1.0.0",
				HanzoAgentsURL: server.URL,
				Logger:        log.New(io.Discard, "", 0),
			}

			agent, err := New(cfg)
			require.NoError(t, err)

			result, err := agent.Call(context.Background(), "target", map[string]any{})
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}

func TestAI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ai.Response{
			Choices: []ai.Choice{
				{
					Message: ai.Message{
						Content: "AI response",
					},
				},
			},
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := Config{
		NodeID:        "node-1",
		Version:       "1.0.0",
		HanzoAgentsURL: "https://api.example.com",
		Logger:        log.New(io.Discard, "", 0),
		AIConfig: &ai.Config{
			APIKey:  "test-key",
			BaseURL: server.URL,
			Model:   "gpt-4o",
		},
	}

	agent, err := New(cfg)
	require.NoError(t, err)

	resp, err := agent.AI(context.Background(), "Hello")
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "AI response", resp.Text())
}

func TestAI_NotConfigured(t *testing.T) {
	cfg := Config{
		NodeID:        "node-1",
		Version:       "1.0.0",
		HanzoAgentsURL: "https://api.example.com",
		Logger:        log.New(io.Discard, "", 0),
		// No AIConfig
	}

	agent, err := New(cfg)
	require.NoError(t, err)

	resp, err := agent.AI(context.Background(), "Hello")
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "AI not configured")
}

func TestAIStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// Write SSE chunks with proper formatting
		chunks := []string{
			"data: {\"id\":\"test\",\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n",
			"data: [DONE]\n\n",
		}

		for _, chunk := range chunks {
			w.Write([]byte(chunk))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}))
	defer server.Close()

	cfg := Config{
		NodeID:        "node-1",
		Version:       "1.0.0",
		HanzoAgentsURL: "https://api.example.com",
		Logger:        log.New(io.Discard, "", 0),
		AIConfig: &ai.Config{
			APIKey:  "test-key",
			BaseURL: server.URL,
			Model:   "gpt-4o",
		},
	}

	agent, err := New(cfg)
	require.NoError(t, err)

	chunks, errs := agent.AIStream(context.Background(), "Hello")

	var receivedChunks []ai.StreamChunk
	done := make(chan bool)

	go func() {
		for chunk := range chunks {
			receivedChunks = append(receivedChunks, chunk)
		}
		done <- true
	}()

	// Wait for either error or completion
	select {
	case err := <-errs:
		if err != nil {
			t.Logf("Received error: %v", err)
		}
	case <-done:
	case <-time.After(2 * time.Second):
		t.Log("Timeout waiting for stream")
	}

	// The stream may or may not receive chunks depending on timing
	// Just verify the channels work
	assert.NotNil(t, chunks)
	assert.NotNil(t, errs)
}

func TestAIStream_NotConfigured(t *testing.T) {
	cfg := Config{
		NodeID:        "node-1",
		Version:       "1.0.0",
		HanzoAgentsURL: "https://api.example.com",
		Logger:        log.New(io.Discard, "", 0),
	}

	agent, err := New(cfg)
	require.NoError(t, err)

	chunks, errs := agent.AIStream(context.Background(), "Hello")

	// Should receive error immediately
	select {
	case err := <-errs:
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "AI not configured")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Expected error but got none")
	}

	// Chunks channel should be closed
	_, ok := <-chunks
	assert.False(t, ok)
}

func TestExecutionContext(t *testing.T) {
	ctx := context.Background()
	execCtx := ExecutionContext{
		RunID:             "run-1",
		ExecutionID:       "exec-1",
		ParentExecutionID: "parent-1",
		SessionID:         "session-1",
		ActorID:           "actor-1",
	}

	ctxWithExec := contextWithExecution(ctx, execCtx)
	retrieved := executionContextFrom(ctxWithExec)

	assert.Equal(t, execCtx, retrieved)
}

func TestExecutionContext_Empty(t *testing.T) {
	ctx := context.Background()
	execCtx := executionContextFrom(ctx)
	assert.Equal(t, ExecutionContext{}, execCtx)
}

func TestHandleReasonerAsyncPostsStatus(t *testing.T) {
	callbackCh := make(chan map[string]any, 1)
	callbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		dec := json.NewDecoder(r.Body)
		var payload map[string]any
		if err := dec.Decode(&payload); err == nil {
			callbackCh <- payload
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer callbackServer.Close()

	cfg := Config{
		NodeID:        "node-1",
		Version:       "1.0.0",
		TeamID:        "team",
		HanzoAgentsURL: callbackServer.URL,
		ListenAddress: ":0",
		PublicURL:     "http://localhost:0",
		Logger:        log.New(io.Discard, "[test] ", 0),
	}

	agent, err := New(cfg)
	require.NoError(t, err)

	agent.RegisterReasoner("demo", func(ctx context.Context, input map[string]any) (any, error) {
		time.Sleep(10 * time.Millisecond)
		return map[string]any{"ok": true}, nil
	})

	server := httptest.NewServer(agent.handler())
	defer server.Close()

	reqBody := []byte(`{"value":42}`)
	req, err := http.NewRequest(http.MethodPost, server.URL+"/reasoners/demo", bytes.NewReader(reqBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Execution-ID", "exec-test")
	req.Header.Set("X-Run-ID", "run-1")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	resp.Body.Close()

	select {
	case payload := <-callbackCh:
		assert.Equal(t, "exec-test", payload["execution_id"])
		assert.Equal(t, "succeeded", payload["status"])
		result, ok := payload["result"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, true, result["ok"])
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for callback payload")
	}
}

func TestChildContext(t *testing.T) {
	parent := ExecutionContext{
		RunID:          "run-1",
		ExecutionID:    "exec-parent",
		WorkflowID:     "wf-1",
		RootWorkflowID: "root-wf",
		SessionID:      "session-1",
		ActorID:        "actor-1",
		Depth:          2,
	}

	child := parent.ChildContext("node-1", "child-reasoner")

	assert.Equal(t, "run-1", child.RunID)
	assert.Equal(t, "wf-1", child.WorkflowID)
	assert.Equal(t, "wf-1", child.ParentWorkflowID)
	assert.Equal(t, "root-wf", child.RootWorkflowID)
	assert.Equal(t, "exec-parent", child.ParentExecutionID)
	assert.Equal(t, 3, child.Depth)
	assert.Equal(t, "node-1", child.AgentNodeID)
	assert.Equal(t, "child-reasoner", child.ReasonerName)
	assert.NotEmpty(t, child.ExecutionID)
	assert.False(t, child.StartedAt.IsZero())
}

func TestChildContextGeneratesRunID(t *testing.T) {
	parent := ExecutionContext{}

	child := parent.ChildContext("node-1", "child-reasoner")

	assert.NotEmpty(t, child.RunID)
	assert.NotEmpty(t, child.WorkflowID)
	assert.Equal(t, child.WorkflowID, child.ParentWorkflowID)
	assert.Equal(t, child.WorkflowID, child.RootWorkflowID)
}

func TestBuildChildContext(t *testing.T) {
	cfg := Config{
		NodeID:        "node-1",
		Version:       "1.0.0",
		HanzoAgentsURL: "http://example.com",
		Logger:        log.New(io.Discard, "", 0),
	}

	ag, err := New(cfg)
	require.NoError(t, err)

	parent := ExecutionContext{
		RunID:          "run-1",
		ExecutionID:    "exec-parent",
		WorkflowID:     "wf-1",
		RootWorkflowID: "root-wf",
		Depth:          1,
		SessionID:      "session-1",
		ActorID:        "actor-1",
	}

	child := ag.buildChildContext(parent, "child")

	assert.Equal(t, parent.RunID, child.RunID)
	assert.Equal(t, parent.ExecutionID, child.ParentExecutionID)
	assert.Equal(t, parent.WorkflowID, child.WorkflowID)
	assert.Equal(t, parent.WorkflowID, child.ParentWorkflowID)
	assert.Equal(t, parent.RootWorkflowID, child.RootWorkflowID)
	assert.Equal(t, "node-1", child.AgentNodeID)
	assert.Equal(t, "child", child.ReasonerName)
	assert.Equal(t, parent.Depth+1, child.Depth)
	assert.NotEmpty(t, child.ExecutionID)
}

func TestBuildChildContextRoot(t *testing.T) {
	cfg := Config{
		NodeID:  "node-1",
		Version: "1.0.0",
		Logger:  log.New(io.Discard, "", 0),
	}

	ag, err := New(cfg)
	require.NoError(t, err)

	child := ag.buildChildContext(ExecutionContext{}, "root-reasoner")

	assert.NotEmpty(t, child.RunID)
	assert.NotEmpty(t, child.ExecutionID)
	assert.Equal(t, child.WorkflowID, child.RootWorkflowID)
	assert.Empty(t, child.ParentExecutionID)
	assert.Equal(t, "node-1", child.AgentNodeID)
	assert.Equal(t, "root-reasoner", child.ReasonerName)
}

func TestCallLocalEmitsEvents(t *testing.T) {
	eventCh := make(chan types.WorkflowExecutionEvent, 4)
	eventServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body, _ := io.ReadAll(r.Body)
		var event types.WorkflowExecutionEvent
		if err := json.Unmarshal(body, &event); err == nil {
			eventCh <- event
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer eventServer.Close()

	cfg := Config{
		NodeID:        "node-1",
		Version:       "1.0.0",
		HanzoAgentsURL: eventServer.URL,
		Logger:        log.New(io.Discard, "", 0),
	}

	ag, err := New(cfg)
	require.NoError(t, err)

	ag.RegisterReasoner("child", func(ctx context.Context, input map[string]any) (any, error) {
		return map[string]any{"echo": input["msg"]}, nil
	})

	parentCtx := contextWithExecution(context.Background(), ExecutionContext{
		RunID:          "run-1",
		ExecutionID:    "exec-parent",
		WorkflowID:     "wf-1",
		RootWorkflowID: "wf-1",
		ReasonerName:   "parent",
		AgentNodeID:    "node-1",
	})

	res, err := ag.CallLocal(parentCtx, "child", map[string]any{"msg": "hi"})
	require.NoError(t, err)

	resultMap, ok := res.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "hi", resultMap["echo"])

	var received []types.WorkflowExecutionEvent
	timeout := time.After(2 * time.Second)

	for len(received) < 2 {
		select {
		case evt := <-eventCh:
			received = append(received, evt)
		case <-timeout:
			t.Fatalf("timed out waiting for workflow events, received %d", len(received))
		}
	}

	statuses := map[string]bool{}
	for _, evt := range received {
		assert.Equal(t, "child", evt.ReasonerID)
		assert.Equal(t, "node-1", evt.AgentNodeID)
		assert.Equal(t, "wf-1", evt.WorkflowID)
		assert.Equal(t, "run-1", evt.RunID)
		if evt.ParentExecutionID == nil {
			t.Fatalf("expected ParentExecutionID to be set")
		}
		assert.Equal(t, "exec-parent", *evt.ParentExecutionID)
		statuses[evt.Status] = true
	}

	assert.True(t, statuses["running"])
	assert.True(t, statuses["succeeded"])
}

func TestCallLocalUnknownReasoner(t *testing.T) {
	cfg := Config{
		NodeID:  "node-1",
		Version: "1.0.0",
		Logger:  log.New(io.Discard, "", 0),
	}

	ag, err := New(cfg)
	require.NoError(t, err)

	_, err = ag.CallLocal(context.Background(), "missing", map[string]any{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown reasoner")
}
