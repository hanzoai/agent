// Copyright © 2026 Hanzo AI. MIT License.

package main

import (
	"testing"

	"github.com/hanzoai/agents/control-plane/pkg/agents"
)

func TestPortFromAddr(t *testing.T) {
	tests := []struct {
		addr string
		want int
	}{
		{":8080", 8080},
		{"127.0.0.1:7243", 7243},
		{"0.0.0.0:9999", 9999},
	}
	for _, tt := range tests {
		got, err := portFromAddr(tt.addr)
		if err != nil {
			t.Errorf("portFromAddr(%q): %v", tt.addr, err)
			continue
		}
		if got != tt.want {
			t.Errorf("portFromAddr(%q): want %d, got %d", tt.addr, tt.want, got)
		}
	}
}

func TestPortFromAddr_Invalid(t *testing.T) {
	if _, err := portFromAddr("nocolon"); err == nil {
		t.Error("portFromAddr(\"nocolon\"): want error, got nil")
	}
}

func TestEnvBool(t *testing.T) {
	t.Setenv("AGENTD_TEST_BOOL", "true")
	if got := envBool("AGENTD_TEST_BOOL", false); !got {
		t.Error("envBool(true): want true")
	}
	t.Setenv("AGENTD_TEST_BOOL", "")
	if got := envBool("AGENTD_TEST_BOOL", true); !got {
		t.Error("envBool(empty): want default true")
	}
	t.Setenv("AGENTD_TEST_BOOL", "garbage")
	if got := envBool("AGENTD_TEST_BOOL", false); got {
		t.Error("envBool(garbage): want default false")
	}
}

func TestEnvStr(t *testing.T) {
	t.Setenv("AGENTD_TEST_STR", "hello")
	if got := envStr("AGENTD_TEST_STR", "fallback"); got != "hello" {
		t.Errorf("envStr: want hello, got %q", got)
	}
	t.Setenv("AGENTD_TEST_STR", "")
	if got := envStr("AGENTD_TEST_STR", "fallback"); got != "fallback" {
		t.Errorf("envStr(empty): want fallback, got %q", got)
	}
}

func TestBuildServerConfig_LocalDefaults(t *testing.T) {
	cfg, err := buildServerConfig(agents.EmbedConfig{
		HTTPAddr: ":8080",
		DataDir:  "/tmp/agentd-test",
	})
	if err != nil {
		t.Fatalf("buildServerConfig: %v", err)
	}
	if cfg.HanzoAgents.Port != 8080 {
		t.Errorf("port: want 8080, got %d", cfg.HanzoAgents.Port)
	}
	if cfg.Storage.Mode != "local" {
		t.Errorf("mode: want local, got %s", cfg.Storage.Mode)
	}
	if cfg.Storage.Local.DatabasePath != "/tmp/agentd-test/hanzo-agents.db" {
		t.Errorf("db path: want /tmp/agentd-test/hanzo-agents.db, got %s", cfg.Storage.Local.DatabasePath)
	}
}

func TestBuildServerConfig_PostgresMode(t *testing.T) {
	cfg, err := buildServerConfig(agents.EmbedConfig{
		HTTPAddr:    ":8080",
		PostgresDSN: "postgres://x@y/db",
	})
	if err != nil {
		t.Fatalf("buildServerConfig: %v", err)
	}
	if cfg.Storage.Mode != "postgres" {
		t.Errorf("mode: want postgres, got %s", cfg.Storage.Mode)
	}
	if cfg.Storage.Postgres.DSN != "postgres://x@y/db" {
		t.Errorf("DSN: want postgres://x@y/db, got %s", cfg.Storage.Postgres.DSN)
	}
}
