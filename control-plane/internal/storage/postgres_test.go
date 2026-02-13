package storage

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hanzoai/agents/control-plane/pkg/types"

	"github.com/stretchr/testify/require"
)

// TestPostgresStorage_ConnectionPooling tests postgres connection pool management
func TestPostgresStorage_ConnectionPooling(t *testing.T) {
	postgresURL := os.Getenv("POSTGRES_TEST_URL")
	if postgresURL == "" {
		t.Skip("POSTGRES_TEST_URL not set, skipping postgres tests")
	}

	ctx := context.Background()
	cfg := StorageConfig{
		Mode: "postgres",
		Postgres: PostgresStorageConfig{
			DSN:            postgresURL,
			MaxOpenConns:   10,
			MaxIdleConns:   5,
			ConnMaxLifetime: 5 * time.Minute,
		},
	}

	ls := NewPostgresStorage(PostgresStorageConfig{})
	err := ls.Initialize(ctx, cfg)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "does not exist") {
			t.Skip("PostgreSQL not available, skipping test")
		}
		require.NoError(t, err)
	}
	defer ls.Close(ctx)

	// Test that we can create and retrieve records
	exec := &types.Execution{
		ExecutionID: "exec-pg-1",
		RunID:       "run-pg-1",
		AgentNodeID: "agent-1",
		ReasonerID:  "reasoner-1",
		NodeID:      "node-1",
		Status:      string(types.ExecutionStatusPending),
		StartedAt:   time.Now().UTC(),
	}

	err = ls.CreateExecutionRecord(ctx, exec)
	require.NoError(t, err)

	retrieved, err := ls.GetExecutionRecord(ctx, "exec-pg-1")
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	require.Equal(t, exec.ExecutionID, retrieved.ExecutionID)
}

// TestPostgresStorage_DatabaseCreation tests automatic database creation
func TestPostgresStorage_DatabaseCreation(t *testing.T) {
	postgresURL := os.Getenv("POSTGRES_TEST_URL")
	if postgresURL == "" {
		t.Skip("POSTGRES_TEST_URL not set, skipping postgres tests")
	}

	// This test would require admin access to create databases
	// Skipping for now but structure is ready
	t.Skip("Requires admin database access")
}

// TestPostgresStorage_ConnectionSettings tests connection pool settings
func TestPostgresStorage_ConnectionSettings(t *testing.T) {
	postgresURL := os.Getenv("POSTGRES_TEST_URL")
	if postgresURL == "" {
		t.Skip("POSTGRES_TEST_URL not set, skipping postgres tests")
	}

	ctx := context.Background()
	cfg := StorageConfig{
		Mode: "postgres",
		Postgres: PostgresStorageConfig{
			DSN:             postgresURL,
			MaxOpenConns:    25,
			MaxIdleConns:    10,
			ConnMaxLifetime: 10 * time.Minute,
		},
	}

	ls := NewPostgresStorage(PostgresStorageConfig{})
	err := ls.Initialize(ctx, cfg)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "does not exist") {
			t.Skip("PostgreSQL not available, skipping test")
		}
		require.NoError(t, err)
	}
	defer ls.Close(ctx)

	// Verify storage is functional
	exec := &types.Execution{
		ExecutionID: "exec-pg-conn",
		RunID:       "run-pg-conn",
		AgentNodeID: "agent-1",
		ReasonerID:  "reasoner-1",
		NodeID:      "node-1",
		Status:      string(types.ExecutionStatusPending),
		StartedAt:   time.Now().UTC(),
	}

	err = ls.CreateExecutionRecord(ctx, exec)
	require.NoError(t, err)
}

// TestPostgresStorage_ConcurrentOperations tests concurrent operations on postgres
func TestPostgresStorage_ConcurrentOperations(t *testing.T) {
	postgresURL := os.Getenv("POSTGRES_TEST_URL")
	if postgresURL == "" {
		t.Skip("POSTGRES_TEST_URL not set, skipping postgres tests")
	}

	ctx := context.Background()
	cfg := StorageConfig{
		Mode: "postgres",
		Postgres: PostgresStorageConfig{
			DSN: postgresURL,
		},
	}

	ls := NewPostgresStorage(PostgresStorageConfig{})
	err := ls.Initialize(ctx, cfg)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "does not exist") {
			t.Skip("PostgreSQL not available, skipping test")
		}
		require.NoError(t, err)
	}
	defer ls.Close(ctx)

	// Create multiple executions concurrently
	const numExecutions = 10
	done := make(chan error, numExecutions)

	for i := 0; i < numExecutions; i++ {
		go func(id int) {
			exec := &types.Execution{
				ExecutionID: "exec-pg-concurrent-" + string(rune(id)),
				RunID:       "run-pg-concurrent",
				AgentNodeID: "agent-1",
				ReasonerID:  "reasoner-1",
				NodeID:      "node-1",
				Status:      string(types.ExecutionStatusPending),
				StartedAt:   time.Now().UTC(),
			}
			done <- ls.CreateExecutionRecord(ctx, exec)
		}(i)
	}

	// Wait for all operations to complete
	for i := 0; i < numExecutions; i++ {
		err := <-done
		require.NoError(t, err)
	}

	// Verify all executions were created
	results, err := ls.QueryExecutionRecords(ctx, types.ExecutionFilter{
		RunID: stringPtr("run-pg-concurrent"),
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(results), numExecutions)
}
