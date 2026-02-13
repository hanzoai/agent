package agent

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestAgent(t *testing.T) *Agent {
	t.Helper()
	a, err := New(Config{
		NodeID:  "node-1",
		Version: "1.0.0",
		Logger:  log.New(io.Discard, "", 0),
	})
	require.NoError(t, err)
	return a
}

func captureOutput(t *testing.T, fn func() error) (string, string, error) {
	t.Helper()
	stdoutR, stdoutW, _ := os.Pipe()
	stderrR, stderrW, _ := os.Pipe()

	origStdout := os.Stdout
	origStderr := os.Stderr
	os.Stdout = stdoutW
	os.Stderr = stderrW

	err := fn()

	stdoutW.Close()
	stderrW.Close()
	os.Stdout = origStdout
	os.Stderr = origStderr

	outBytes, _ := io.ReadAll(stdoutR)
	errBytes, _ := io.ReadAll(stderrR)

	return string(outBytes), string(errBytes), err
}

func TestParseCLIArgs_MergePriority(t *testing.T) {
	a := newTestAgent(t)

	tmpFile, err := os.CreateTemp(t.TempDir(), "input-*.json")
	require.NoError(t, err)
	_, err = tmpFile.WriteString(`{"source":"file","shared":2}`)
	require.NoError(t, err)
	tmpFile.Close()

	origStdin := os.Stdin
	stdinR, stdinW, _ := os.Pipe()
	os.Stdin = stdinR
	_, _ = stdinW.WriteString(`{"source":"stdin","shared":1}`)
	stdinW.Close()
	t.Cleanup(func() { os.Stdin = origStdin })

	inv, err := a.parseCLIArgs([]string{
		"--input-file", tmpFile.Name(),
		"--input", `{"source":"flag","shared":3}`,
		"--set", "shared=4",
	})
	require.NoError(t, err)

	require.NotNil(t, inv.input)
	assert.Equal(t, "flag", inv.input["source"])
	assert.Equal(t, float64(4), inv.input["shared"])
}

func TestRunCLI_ExecutesDefaultReasoner(t *testing.T) {
	a := newTestAgent(t)

	a.RegisterReasoner("greet", func(ctx context.Context, input map[string]any) (any, error) {
		assert.True(t, IsCLIMode(ctx))
		args := GetCLIArgs(ctx)
		assert.Equal(t, "Bob", args["name"])
		return fmt.Sprintf("Hello, %s", input["name"]), nil
	}, WithCLI(), WithDefaultCLI(), WithDescription("Greets a user"))

	stdout, stderr, err := captureOutput(t, func() error {
		return a.runCLI(context.Background(), []string{"--set", "name=Bob", "--output", "json"})
	})

	require.NoError(t, err)
	assert.Contains(t, stdout, "Hello, Bob")
	assert.Equal(t, "", strings.TrimSpace(stderr))
}
