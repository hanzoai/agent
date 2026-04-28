// Copyright © 2026 Hanzo AI. MIT License.
//
// agentd is the Hanzo Agents control-plane daemon: one Go binary,
// gateway-trust auth, embedded SPA. The shape mirrors tasksd —
// see ~/work/hanzo/HANZO_BINARY.md for the spec.

package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/hanzoai/agents/control-plane/internal/config"
	"github.com/hanzoai/agents/control-plane/internal/server"
	"github.com/hanzoai/agents/control-plane/pkg/agents"
)

// Build-time version stamps (-ldflags).
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	var (
		httpAddr      = flag.String("http", envStr("AGENTD_HTTP_ADDR", ":8080"), "HTTP listen address")
		dataDir       = flag.String("data", envStr("AGENTD_DATA_DIR", "./hanzo-agents-data"), "Local-mode persistence directory")
		postgresDSN   = flag.String("postgres-dsn", envStr("AGENTD_POSTGRES_DSN", ""), "PostgreSQL DSN; non-empty switches storage to postgres mode")
		iamEndpoint   = flag.String("iam-endpoint", envStr("AGENTD_IAM_ENDPOINT", ""), "Internal IAM endpoint (informational, gateway-trust mode)")
		jwtKeySource  = flag.String("jwt-key-source", envStr("AGENTD_JWT_KEY_SOURCE", ""), "Legacy JWKS URL (gateway handles JWT validation)")
		requireIDFlag = flag.Bool("require-identity", envBool("AGENTD_REQUIRE_IDENTITY", false), "Require gateway-supplied identity headers (cloud)")
		showVersion   = flag.Bool("version", false, "Print version and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("agentd %s (commit %s, built %s)\n", version, commit, date)
		return
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := agents.EmbedConfig{
		DataDir:         *dataDir,
		HTTPAddr:        *httpAddr,
		PostgresDSN:     *postgresDSN,
		IAMEndpoint:     *iamEndpoint,
		JWTKeySource:    *jwtKeySource,
		RequireIdentity: *requireIDFlag,
		Logger:          logger,
		ShutdownTimeout: 10 * time.Second,
		Bootstrap:       bootstrap,
	}

	embedded, err := agents.Embed(ctx, cfg)
	if err != nil {
		logger.Error("agents.Embed", "err", err)
		os.Exit(1)
	}

	logger.Info("agentd ready", "version", version, "addr", *httpAddr, "require_identity", *requireIDFlag)
	<-ctx.Done()

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := embedded.Stop(shutdownCtx); err != nil {
		logger.Error("shutdown", "err", err)
	}
}

// bootstrap is the agents.BootstrapFunc implementation: translate
// EmbedConfig into the existing internal/config + internal/server
// shape and start the gin router. When internal/server lifts into
// pkg/agents this collapses; today it bridges.
func bootstrap(ctx context.Context, cfg agents.EmbedConfig) (func(context.Context) error, error) {
	srvCfg, err := buildServerConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build server config: %w", err)
	}

	srv, err := server.NewHanzoAgentsServer(srvCfg)
	if err != nil {
		return nil, fmt.Errorf("build server: %w", err)
	}

	go func() {
		if err := srv.Start(); err != nil {
			cfg.Logger.Error("server exited", "err", err)
		}
	}()

	return func(_ context.Context) error {
		return srv.Stop()
	}, nil
}

// buildServerConfig translates the canonical agents.EmbedConfig
// into the legacy internal/config.Config. Single source of truth
// for the env-var → config mapping. When internal/config lifts
// into pkg/agents this disappears.
func buildServerConfig(cfg agents.EmbedConfig) (*config.Config, error) {
	port, err := portFromAddr(cfg.HTTPAddr)
	if err != nil {
		return nil, err
	}
	c := &config.Config{}
	c.HanzoAgents.Port = port

	if cfg.PostgresDSN != "" {
		c.Storage.Mode = "postgres"
		c.Storage.Postgres.DSN = cfg.PostgresDSN
		c.Storage.Postgres.URL = cfg.PostgresDSN
	} else {
		c.Storage.Mode = "local"
		c.Storage.Local.DatabasePath = cfg.DataDir + "/hanzo-agents.db"
		c.Storage.Local.KVStorePath = cfg.DataDir + "/cache"
	}

	c.UI.Enabled = true
	c.UI.Mode = "embedded"

	c.Cloud.Defaults()
	c.Cloud.ApplyEnvOverrides()

	return c, nil
}

func portFromAddr(addr string) (int, error) {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			var port int
			if _, err := fmt.Sscanf(addr[i+1:], "%d", &port); err != nil {
				return 0, fmt.Errorf("parse port from %q: %w", addr, err)
			}
			return port, nil
		}
	}
	return 0, fmt.Errorf("missing port in address %q", addr)
}

func envBool(k string, def bool) bool {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func envStr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
