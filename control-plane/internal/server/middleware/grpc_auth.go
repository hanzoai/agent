//go:build grpc

// Copyright © 2026 Hanzo AI. MIT License.

package middleware

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/hanzoai/agents/control-plane/pkg/auth"
)

// APIKeyUnaryInterceptor enforces API key authentication on gRPC
// unary calls. Mirrors the HTTP APIKeyAuth shape: takes a Validator,
// upgrades a legacy single key string when no validator is wired,
// and rejects with PermissionDenied (gRPC equivalent of 403) when
// the key's bound OrgID differs from the request metadata's
// x-org-id.
//
// All key compares are constant-time (bcrypt at the Store layer,
// subtle.ConstantTimeCompare for the static path) to avoid leaking
// the prefix length of the supplied key on a timing side channel.
func APIKeyUnaryInterceptor(apiKey string, opts ...grpcAuthOption) grpc.UnaryServerInterceptor {
	cfg := grpcAuthConfig{}
	for _, o := range opts {
		o(&cfg)
	}
	validator := cfg.validator
	if validator == nil && apiKey != "" {
		validator = auth.NewStaticValidator(apiKey)
	}

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// No auth configured, allow everything.
		if validator == nil {
			return handler(ctx, req)
		}

		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}

		raw := mdAPIKey(md)
		if raw == "" {
			return nil, status.Error(codes.Unauthenticated, "invalid or missing api key")
		}

		key, err := validator.Validate(ctx, raw)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid or missing api key")
		}

		if key.OrgID != "" {
			requestOrg := mdOrgID(md)
			if err := key.CheckOrg(requestOrg); err != nil {
				if errors.Is(err, auth.ErrOrgMismatch) {
					return nil, status.Error(codes.PermissionDenied, "api key bound to a different org than request claims")
				}
				return nil, status.Error(codes.Unauthenticated, "api key validation failed")
			}
			ctx = auth.WithOrgContext(ctx, key.OrgID)
		}

		return handler(ctx, req)
	}
}

// grpcAuthOption is the small functional-options surface for
// the gRPC interceptor. Lets callers inject a Store-backed validator
// while keeping the legacy `APIKeyUnaryInterceptor("static-key")` shape
// working unchanged.
type grpcAuthOption func(*grpcAuthConfig)

type grpcAuthConfig struct {
	validator auth.Validator
}

// WithValidator wires a Store-backed (or test-fake) Validator into
// the gRPC interceptor.
func WithValidator(v auth.Validator) grpcAuthOption {
	return func(c *grpcAuthConfig) { c.validator = v }
}

// mdAPIKey extracts the API key from x-api-key or authorization
// metadata. gRPC lower-cases metadata keys on the wire.
func mdAPIKey(md metadata.MD) string {
	if vs := md.Get("x-api-key"); len(vs) > 0 && vs[0] != "" {
		return vs[0]
	}
	if vs := md.Get("authorization"); len(vs) > 0 && strings.HasPrefix(vs[0], "Bearer ") {
		t := strings.TrimPrefix(vs[0], "Bearer ")
		if t != "" {
			return t
		}
	}
	return ""
}

// mdOrgID reads the canonical X-Org-Id metadata header. Empty when
// the caller did not assert an org — that is allowed (the key's
// bound OrgID still wins).
func mdOrgID(md metadata.MD) string {
	if vs := md.Get("x-org-id"); len(vs) > 0 {
		return vs[0]
	}
	return ""
}
