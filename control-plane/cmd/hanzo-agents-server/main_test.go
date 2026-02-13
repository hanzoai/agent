package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/hanzoai/agents/control-plane/internal/config"
	"github.com/hanzoai/agents/control-plane/internal/server"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestLoadConfig_DefaultsApplied(t *testing.T) {
	t.Setenv("HANZO_AGENTS_PORT", "")
	t.Setenv("HANZO_AGENTS_CONFIG_FILE", "")
	viper.Reset()

	cfg, err := loadConfig("/dev/null")
	if err != nil {
		t.Fatalf("loadConfig returned error: %v", err)
	}

	if cfg.HanzoAgents.Port != 8080 {
		t.Errorf("expected default hanzo-agents port 8080, got %d", cfg.HanzoAgents.Port)
	}
	if cfg.Storage.Mode != "local" {
		t.Errorf("expected default storage mode local, got %s", cfg.Storage.Mode)
	}
	if !cfg.UI.Enabled {
		t.Error("expected UI enabled by default")
	}
	if cfg.UI.Mode != "embedded" {
		t.Errorf("expected default UI mode embedded, got %s", cfg.UI.Mode)
	}
	if !cfg.Features.DID.Enabled {
		t.Error("expected DID enabled by default")
	}
	if cfg.Features.DID.Keystore.Path == "" {
		t.Error("expected default DID keystore path to be set")
	}
}

func TestLoadConfig_ConfigFileValues(t *testing.T) {
	viper.Reset()

	dir := t.TempDir()
	file := filepath.Join(dir, "hanzo-agents.yaml")
	content := []byte(`hanzo-agents:
  port: 9231
storage:
  mode: local
  local:
    database_path: "/tmp/custom.db"
ui:
  enabled: false
  mode: dev
features:
  did:
    enabled: false
`)
	if err := os.WriteFile(file, content, 0o644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	cfg, err := loadConfig(file)
	if err != nil {
		t.Fatalf("loadConfig returned error: %v", err)
	}

	if cfg.HanzoAgents.Port != 9231 {
		t.Errorf("expected port 9231, got %d", cfg.HanzoAgents.Port)
	}
	if cfg.UI.Enabled {
		t.Error("expected UI disabled from config")
	}
	if cfg.Features.DID.Enabled {
		t.Error("expected DID disabled from config")
	}
	if cfg.Storage.Local.DatabasePath != "/tmp/custom.db" {
		t.Errorf("unexpected database path %s", cfg.Storage.Local.DatabasePath)
	}
}

func TestLoadConfig_VCRequirementsFromConfigFile(t *testing.T) {
	viper.Reset()

	dir := t.TempDir()
	file := filepath.Join(dir, "hanzo-agents.yaml")
	content := []byte(`hanzo-agents:
  port: 8080
features:
  did:
    enabled: true
    vc_requirements:
      require_vc_registration: true
      require_vc_execution: true
      require_vc_cross_agent: true
      persist_execution_vc: true
`)
	if err := os.WriteFile(file, content, 0o644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	cfg, err := loadConfig(file)
	if err != nil {
		t.Fatalf("loadConfig returned error: %v", err)
	}

	if !cfg.Features.DID.Enabled {
		t.Error("expected DID enabled from config")
	}
	if !cfg.Features.DID.VCRequirements.RequireVCForExecution {
		t.Error("expected require_vc_execution=true from config")
	}
	if !cfg.Features.DID.VCRequirements.PersistExecutionVC {
		t.Error("expected persist_execution_vc=true from config")
	}
}

func TestBuildUI_SkipsWhenPackageJSONMissing(t *testing.T) {
	cfg := &config.Config{UI: config.UIConfig{SourcePath: t.TempDir()}}

	original := commandRunner
	defer func() { commandRunner = original }()

	commandRunner = func(dir string, env []string, name string, args ...string) error {
		t.Fatalf("commandRunner should not be invoked when package.json is missing")
		return nil
	}

	if err := buildUI(cfg); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestBuildUI_RunsInstallAndBuild(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("failed to create package.json: %v", err)
	}

	cfg := &config.Config{
		UI: config.UIConfig{
			SourcePath: dir,
			DistPath:   filepath.Join(dir, "dist"),
		},
		HanzoAgents: config.HanzoAgentsConfig{Port: 8081},
	}

	var mu sync.Mutex
	var calls []string

	original := commandRunner
	defer func() { commandRunner = original }()

	commandRunner = func(dir string, env []string, name string, args ...string) error {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, fmt.Sprintf("%s %v", name, args))
		if dir != cfg.UI.SourcePath {
			t.Errorf("unexpected command dir %s", dir)
		}
		// Ensure environment includes proxy
		expectedPrefix := fmt.Sprintf("VITE_API_PROXY_TARGET=http://localhost:%d", cfg.HanzoAgents.Port)
		found := false
		for _, envVar := range env {
			if envVar == expectedPrefix {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected environment to include %s", expectedPrefix)
		}
		return nil
	}

	if err := buildUI(cfg); err != nil {
		t.Fatalf("buildUI returned error: %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(calls))
	}
	if calls[0] != "npm [install --force]" {
		t.Errorf("unexpected first command %s", calls[0])
	}
	if calls[1] != "npm [run build]" {
		t.Errorf("unexpected second command %s", calls[1])
	}
}

func TestBuildUI_CommandError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("failed to create package.json: %v", err)
	}

	cfg := &config.Config{UI: config.UIConfig{SourcePath: dir}}

	original := commandRunner
	defer func() { commandRunner = original }()

	wantErr := errors.New("boom")
	commandRunner = func(dir string, env []string, name string, args ...string) error {
		return wantErr
	}

	if err := buildUI(cfg); err == nil || !errors.Is(err, wantErr) {
		t.Fatalf("expected error %v, got %v", wantErr, err)
	}
}

func TestRunServer_AppliesFlagOverrides(t *testing.T) {
	cfg := &config.Config{
		HanzoAgents: config.HanzoAgentsConfig{Port: 4000},
		UI:         config.UIConfig{Enabled: true, Mode: "embedded"},
		Features: config.FeatureConfig{DID: config.DIDConfig{
			VCRequirements: config.VCRequirements{
				RequireVCForExecution: true,
			},
		}},
	}

	loadOrig := loadConfigFunc
	newOrig := newHanzoAgentsServerFunc
	buildOrig := buildUIFunc
	openOrig := openBrowserFunc
	sleepOrig := sleepFunc
	waitOrig := waitForShutdownFunc
	startOrig := startHanzoAgentsServerFunc

	defer func() {
		loadConfigFunc = loadOrig
		newHanzoAgentsServerFunc = newOrig
		buildUIFunc = buildOrig
		openBrowserFunc = openOrig
		sleepFunc = sleepOrig
		waitForShutdownFunc = waitOrig
		startHanzoAgentsServerFunc = startOrig
	}()

	loadConfigFunc = func(path string) (*config.Config, error) {
		if path != "" {
			t.Logf("loadConfig called with %s", path)
		}
		return cfg, nil
	}

	var gotCfg *config.Config
	newHanzoAgentsServerFunc = func(c *config.Config) (*server.HanzoAgentsServer, error) {
		gotCfg = c
		return &server.HanzoAgentsServer{}, nil
	}

	buildUIFunc = func(*config.Config) error { return nil }
	openBrowserFunc = func(string) {}
	sleepFunc = func(time.Duration) {}
	waitForShutdownFunc = func() {}

	started := make(chan struct{})
	startHanzoAgentsServerFunc = func(*server.HanzoAgentsServer) error {
		close(started)
		return nil
	}

	cmd := &cobra.Command{}
	cmd.Flags().String("config", "", "")
	cmd.Flags().Bool("backend-only", false, "")
	cmd.Flags().Bool("ui-dev", false, "")
	cmd.Flags().Bool("open", true, "")
	cmd.Flags().Int("port", 0, "")
	cmd.Flags().Bool("no-vc-execution", false, "")

	if err := cmd.Flags().Set("backend-only", "true"); err != nil {
		t.Fatalf("failed to set backend-only: %v", err)
	}
	if err := cmd.Flags().Set("port", "9090"); err != nil {
		t.Fatalf("failed to set port flag: %v", err)
	}
	if err := cmd.Flags().Set("no-vc-execution", "true"); err != nil {
		t.Fatalf("failed to set no-vc-execution: %v", err)
	}

	t.Setenv("HANZO_AGENTS_PORT", "12345")

	runServer(cmd, nil)

	<-started

	if gotCfg == nil {
		t.Fatal("expected af server creation to be invoked")
	}
	if gotCfg.HanzoAgents.Port != 12345 {
		t.Fatalf("expected env override port 12345, got %d", gotCfg.HanzoAgents.Port)
	}
	if gotCfg.UI.Enabled {
		t.Fatal("backend-only flag should disable UI")
	}
	if gotCfg.Features.DID.VCRequirements.RequireVCForExecution {
		t.Fatal("no-vc-execution flag should disable VC requirement for execution")
	}
}

func TestRunServer_OpensBrowserForDevUI(t *testing.T) {
	cfg := &config.Config{
		HanzoAgents: config.HanzoAgentsConfig{Port: 8800},
		UI: config.UIConfig{
			Enabled: true,
			Mode:    "dev",
			DevPort: 4200,
		},
		Features: config.FeatureConfig{DID: config.DIDConfig{}},
	}

	loadOrig := loadConfigFunc
	newOrig := newHanzoAgentsServerFunc
	openOrig := openBrowserFunc
	sleepOrig := sleepFunc
	waitOrig := waitForShutdownFunc
	startOrig := startHanzoAgentsServerFunc

	defer func() {
		loadConfigFunc = loadOrig
		newHanzoAgentsServerFunc = newOrig
		openBrowserFunc = openOrig
		sleepFunc = sleepOrig
		waitForShutdownFunc = waitOrig
		startHanzoAgentsServerFunc = startOrig
	}()

	loadConfigFunc = func(string) (*config.Config, error) { return cfg, nil }
	newHanzoAgentsServerFunc = func(*config.Config) (*server.HanzoAgentsServer, error) { return &server.HanzoAgentsServer{}, nil }
	sleepFunc = func(time.Duration) {}
	waitForShutdownFunc = func() {}
	started := make(chan struct{})
	startHanzoAgentsServerFunc = func(*server.HanzoAgentsServer) error {
		close(started)
		return nil
	}

	var opened string
	openBrowserFunc = func(url string) {
		opened = url
	}

	cmd := &cobra.Command{}
	cmd.Flags().String("config", "", "")
	cmd.Flags().Bool("backend-only", false, "")
	cmd.Flags().Bool("ui-dev", false, "")
	cmd.Flags().Bool("open", true, "")
	cmd.Flags().Int("port", 0, "")
	cmd.Flags().Bool("no-vc-execution", false, "")

	runServer(cmd, nil)

	<-started

	if opened != "http://localhost:4200" {
		t.Fatalf("expected browser to open dev port, got %s", opened)
	}
}

func TestOpenBrowserUsesLauncher(t *testing.T) {
	orig := browserLauncher
	defer func() { browserLauncher = orig }()

	var called bool
	browserLauncher = func(name string, args ...string) error {
		called = true
		if runtime.GOOS == "darwin" && name != "open" {
			t.Fatalf("expected open command on darwin, got %s", name)
		}
		return nil
	}

	openBrowser("http://example.com")

	if !called {
		t.Fatal("expected browserLauncher to be invoked")
	}
}
