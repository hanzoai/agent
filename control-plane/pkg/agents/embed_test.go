// Copyright © 2026 Hanzo AI. MIT License.

package agents

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestEmbedConfig_WithDefaults(t *testing.T) {
	got := EmbedConfig{}.withDefaults()
	if got.DataDir != "./hanzo-agents-data" {
		t.Errorf("DataDir: want ./hanzo-agents-data, got %q", got.DataDir)
	}
	if got.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr: want :8080, got %q", got.HTTPAddr)
	}
	if got.Logger == nil {
		t.Error("Logger: want default, got nil")
	}
	if got.ShutdownTimeout == 0 {
		t.Error("ShutdownTimeout: want non-zero default, got 0")
	}
}

func TestEmbedConfig_WithDefaultsPreservesUserValues(t *testing.T) {
	got := EmbedConfig{
		DataDir:  "/tmp/agents",
		HTTPAddr: "127.0.0.1:9090",
	}.withDefaults()
	if got.DataDir != "/tmp/agents" {
		t.Errorf("DataDir: want /tmp/agents, got %q", got.DataDir)
	}
	if got.HTTPAddr != "127.0.0.1:9090" {
		t.Errorf("HTTPAddr: want 127.0.0.1:9090, got %q", got.HTTPAddr)
	}
}

func TestEmbed_RequiresBootstrap(t *testing.T) {
	_, err := Embed(context.Background(), EmbedConfig{})
	if err == nil {
		t.Fatal("Embed without Bootstrap: want error, got nil")
	}
}

func TestEmbed_BootstrapAndShutdown(t *testing.T) {
	bootstrapped := false
	shutdownCalled := false
	bootstrap := func(ctx context.Context, cfg EmbedConfig) (func(context.Context) error, error) {
		bootstrapped = true
		return func(ctx context.Context) error {
			shutdownCalled = true
			return nil
		}, nil
	}

	e, err := Embed(context.Background(), EmbedConfig{Bootstrap: bootstrap})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if !bootstrapped {
		t.Error("Bootstrap not invoked")
	}

	if err := e.Stop(context.Background()); err != nil {
		t.Errorf("Stop: %v", err)
	}
	if !shutdownCalled {
		t.Error("shutdown not invoked")
	}

	// After Stop the singleton releases — a second Embed must succeed.
	e2, err := Embed(context.Background(), EmbedConfig{Bootstrap: bootstrap})
	if err != nil {
		t.Fatalf("re-Embed after Stop: %v", err)
	}
	_ = e2.Stop(context.Background())
}

func TestEmbed_SingletonGuard(t *testing.T) {
	bootstrap := func(ctx context.Context, cfg EmbedConfig) (func(context.Context) error, error) {
		return func(ctx context.Context) error { return nil }, nil
	}

	e, err := Embed(context.Background(), EmbedConfig{Bootstrap: bootstrap})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	defer e.Stop(context.Background())

	if _, err := Embed(context.Background(), EmbedConfig{Bootstrap: bootstrap}); !errors.Is(err, ErrAlreadyEmbedded) {
		t.Errorf("second Embed: want ErrAlreadyEmbedded, got %v", err)
	}
}

func TestEmbed_BootstrapError(t *testing.T) {
	wantErr := errors.New("boot failed")
	bootstrap := func(ctx context.Context, cfg EmbedConfig) (func(context.Context) error, error) {
		return nil, wantErr
	}
	if _, err := Embed(context.Background(), EmbedConfig{Bootstrap: bootstrap}); !errors.Is(err, wantErr) {
		t.Errorf("Embed bootstrap err: want wrap of %v, got %v", wantErr, err)
	}
	// Singleton guard must release after a bootstrap failure.
	e, err := Embed(context.Background(), EmbedConfig{Bootstrap: func(ctx context.Context, cfg EmbedConfig) (func(context.Context) error, error) {
		return func(ctx context.Context) error { return nil }, nil
	}})
	if err != nil {
		t.Errorf("post-failure Embed: %v", err)
		return
	}
	_ = e.Stop(context.Background())
}

func TestEmbed_StopRespectsTimeout(t *testing.T) {
	bootstrap := func(ctx context.Context, cfg EmbedConfig) (func(context.Context) error, error) {
		return func(ctx context.Context) error { return nil }, nil
	}
	e, err := Embed(context.Background(), EmbedConfig{
		Bootstrap:       bootstrap,
		ShutdownTimeout: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if err := e.Stop(context.Background()); err != nil {
		t.Errorf("Stop: %v", err)
	}
}
