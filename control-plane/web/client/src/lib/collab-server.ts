/**
 * Minimal y-websocket server for development.
 *
 * In production, this can be:
 *  1. Run as a standalone service via `npx y-websocket`
 *  2. Integrated into the Go control-plane binary as a WebSocket handler
 *  3. Deployed as a separate K8s pod (e.g. y-sweet, hocuspocus, etc.)
 *
 * This file documents the expected protocol and can be used as a reference
 * for integrating y-websocket into the backend.
 *
 * To run a dev server:
 *   npx y-websocket --port 1234
 *
 * Or with persistence (LevelDB):
 *   YPERSISTENCE=./yjs-data npx y-websocket --port 1234
 *
 * Environment variables:
 *   VITE_COLLAB_WS_URL  — client-side: WebSocket URL (default ws://localhost:1234)
 *   HOST                — server-side: bind address (default 0.0.0.0)
 *   PORT                — server-side: port (default 1234)
 *   YPERSISTENCE        — server-side: persistence directory (optional)
 */

// This is a documentation-only module.
// The actual y-websocket server is provided by the `y-websocket` npm package.
//
// Usage in package.json scripts:
//   "collab:dev": "npx y-websocket --port 1234"
//
// Usage with Docker:
//   FROM node:22-alpine
//   RUN npm install -g y-websocket
//   CMD ["y-websocket"]
//   EXPOSE 1234

export const COLLAB_SERVER_DEFAULTS = {
  host: '0.0.0.0',
  port: 1234,
  wsPath: '/',
} as const
