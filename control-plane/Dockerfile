# Multi-stage build: Node.js frontend + Go backend
# Result: single binary with embedded UI serving on :8080

# ── Stage 1: Build frontend ──────────────────────────────────────
FROM --platform=$BUILDPLATFORM node:22-alpine AS frontend

WORKDIR /app/web/client
COPY web/client/package.json web/client/package-lock.json ./
RUN npm ci --prefer-offline
COPY web/client/ ./
RUN npx vite build

# ── Stage 2: Build Go binary ─────────────────────────────────────
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS backend

RUN apk add --no-cache git ca-certificates

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Copy built frontend into the embed location
COPY --from=frontend /app/web/client/dist web/client/dist/

# Build with embedded UI tag
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build \
    -tags embedded \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
    -o /hanzo-agents-server \
    ./cmd/hanzo-agents-server

# ── Stage 3: Runtime ─────────────────────────────────────────────
FROM --platform=$TARGETPLATFORM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata
RUN adduser -D -u 1000 hanzo

COPY --from=backend /hanzo-agents-server /usr/local/bin/hanzo-agents-server

# Default config location
RUN mkdir -p /var/hanzo-agents/data /var/hanzo-agents/config /var/hanzo-agents/logs \
    && chown -R hanzo:hanzo /var/hanzo-agents

USER hanzo
WORKDIR /var/hanzo-agents

EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=3s --start-period=5s \
    CMD wget -qO- http://localhost:8080/health || exit 1

ENTRYPOINT ["hanzo-agents-server"]
CMD ["serve"]
