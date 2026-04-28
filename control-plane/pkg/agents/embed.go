// Copyright © 2026 Hanzo AI. MIT License.

// Package agents is the in-process Hanzo Agents control plane.
//
// One backend, two transports: HTTP/JSON for browsers and the
// embedded UI, and the existing internal/server (gin) shape for
// the gRPC + REST API. Embed runs both behind a single goroutine
// you can stop with Embedded.Stop. This is the canonical Hanzo
// binary shape — see ~/work/hanzo/HANZO_BINARY.md.
//
// Decoupling note (2026-04-27): this package owns the canonical
// EmbedConfig contract that the future fused `hanzo` binary will
// import. The legacy server bootstrap lives in internal/server
// and is wired by cmd/agentd today; when that path is stable the
// engine lifts into pkg/agents directly. Same shape `tasks` used
// during its rebrand: contract first, engine follows.
package agents

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// EmbedConfig configures an in-process agentd. Exactly one shape.
//
// The fields here are the cross-binary contract: any future fused
// `hanzo` binary that imports pkg/agents.Embed must populate the
// same struct, no more. Service-specific config still flows via
// internal/config; this struct only carries what the shared shape
// requires.
type EmbedConfig struct {
	// DataDir is where local-mode persistence lives (SQLite, BoltDB,
	// keystore). "" → "./hanzo-agents-data".
	DataDir string

	// HTTPAddr is the HTTP listen address served to browsers. The
	// gateway proxies authenticated traffic to it. "" → ":8080".
	HTTPAddr string

	// PostgresDSN, when set, switches storage to postgres mode and
	// pins the data source. Wins over DataDir.
	PostgresDSN string

	// IAMEndpoint is the internal hanzo.id endpoint, retained for
	// gateway-bypass smoke tests. Production traffic goes through
	// hanzoai/gateway and the binary trusts identity headers; this
	// field is informational only.
	IAMEndpoint string

	// JWTKeySource is preserved for parity with the upstream
	// envelope auth template. agentd does not validate JWTs in
	// the gateway-trust path; this remains for the legacy embed.
	JWTKeySource string

	// RequireIdentity flips the binary into multitenant mode: every
	// authenticated route must carry X-Org-Id/X-User-Id from the
	// gateway. Default false — solo mode for `agentd` running
	// standalone with no gateway in front.
	RequireIdentity bool

	// Logger receives structured logs. nil → slog.Default.
	Logger *slog.Logger

	// ShutdownTimeout caps how long Stop will wait for graceful
	// drain. 0 → 5s.
	ShutdownTimeout time.Duration

	// Bootstrap is the optional engine-start callback supplied by
	// the binary entrypoint. It owns the actual server.Start() call
	// — the cmd/agentd main.go provides one that invokes
	// internal/server.NewHanzoAgentsServer. Tests can supply a stub.
	Bootstrap BootstrapFunc
}

// BootstrapFunc lets the entrypoint plug in the actual server start
// function without pkg/agents importing internal/server. The
// returned shutdown is invoked from Stop.
type BootstrapFunc func(ctx context.Context, cfg EmbedConfig) (shutdown func(context.Context) error, err error)

// Embedded is the handle to a running in-process Agents control
// plane. Singleton-guarded — calling Embed twice without Stop in
// between returns ErrAlreadyEmbedded.
type Embedded struct {
	cfg      EmbedConfig
	shutdown func(context.Context) error
	stopOnce sync.Once
}

// ErrAlreadyEmbedded is returned by Embed when another instance is
// already running in this process. The fused-binary contract is
// one-per-process: same logger, same data dir, same identity
// middleware. If you need a second instance, run a second binary.
var ErrAlreadyEmbedded = errors.New("pkg/agents: already embedded in this process")

var embedded atomic.Bool

// Embed starts the Agents control plane in-process. Stop before
// exit. Idempotent against repeat Stop, single-instance per process.
//
// The shape mirrors github.com/hanzoai/tasks/pkg/tasks.Embed so the
// future fused hanzo binary wires every service through the same
// constructor signature.
func Embed(ctx context.Context, cfg EmbedConfig) (*Embedded, error) {
	if !embedded.CompareAndSwap(false, true) {
		return nil, ErrAlreadyEmbedded
	}

	cfg = cfg.withDefaults()

	if cfg.Bootstrap == nil {
		embedded.Store(false)
		return nil, fmt.Errorf("pkg/agents: EmbedConfig.Bootstrap must be supplied (cmd/agentd wires it)")
	}

	shutdown, err := cfg.Bootstrap(ctx, cfg)
	if err != nil {
		embedded.Store(false)
		return nil, fmt.Errorf("pkg/agents: bootstrap: %w", err)
	}

	cfg.Logger.Info("agents server started",
		"http", cfg.HTTPAddr,
		"data", cfg.DataDir,
		"postgres", cfg.PostgresDSN != "",
		"require_identity", cfg.RequireIdentity,
	)

	return &Embedded{cfg: cfg, shutdown: shutdown}, nil
}

// Stop shuts the server down. Idempotent. Safe to call multiple
// times. Releases the singleton guard so a subsequent Embed can
// proceed (used in tests).
func (e *Embedded) Stop(ctx context.Context) error {
	if e == nil {
		return nil
	}
	var err error
	e.stopOnce.Do(func() {
		if e.shutdown != nil {
			shutdownCtx, cancel := context.WithTimeout(ctx, e.cfg.ShutdownTimeout)
			defer cancel()
			err = e.shutdown(shutdownCtx)
		}
		embedded.Store(false)
	})
	return err
}

// Config returns the resolved configuration. Useful for tests and
// the entrypoint logger.
func (e *Embedded) Config() EmbedConfig {
	if e == nil {
		return EmbedConfig{}
	}
	return e.cfg
}

func (c EmbedConfig) withDefaults() EmbedConfig {
	if c.DataDir == "" {
		c.DataDir = "./hanzo-agents-data"
	}
	if c.HTTPAddr == "" {
		c.HTTPAddr = ":8080"
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	if c.ShutdownTimeout == 0 {
		c.ShutdownTimeout = 5 * time.Second
	}
	return c
}
