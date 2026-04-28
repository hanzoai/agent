// Package admin exposes the embedded Hanzo Agents admin SPA bundle
// (built from ~/work/hanzo/gui/apps/admin-agent) as an http.Handler.
//
// Two UI surfaces ship inside agentd:
//   - web/client (the existing 79k LOC product UI: deck.gl workflow
//     viz, react-flow editor, xterm console) → mounted at '/'.
//   - admin/    (this package, the operator chrome: orgs, API keys,
//     billing, observability) → mounted at '/_/agents/'.
//
// Both are baked into the binary via //go:embed. scripts/sync-admin-ui.sh
// populates admin/dist/ from the gui workspace before each build.
//
// Mount example (cmd/agentd or internal/server):
//
//	mux.Handle("/_/agents/", http.StripPrefix("/_/agents", admin.Handler()))
package admin

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// FS returns the embedded built-UI filesystem rooted at dist/.
// Empty when scripts/sync-admin-ui.sh has not been run (dev workflow:
// run the Vite dev server inside ~/work/hanzo/gui/apps/admin-agent).
func FS() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		// Impossible at runtime because embed.FS entries are
		// validated at compile time; if dist/ is missing the
		// binary simply carries an empty FS.
		return distFS
	}
	return sub
}

// Handler returns an http.Handler that serves the embedded SPA.
//
// Behaviour matches ~/work/hanzo/tasks/ui/embed.go:
//   - Hashed assets under /assets/ get long-lived immutable cache.
//   - Anything else falls through to /index.html so the React
//     router handles the deep link client-side.
//   - Missing index.html → 503 so operators notice in staging
//     before shipping a blank UI to production.
func Handler() http.Handler {
	root := FS()
	fileServer := http.FileServer(http.FS(root))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		reqPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if reqPath == "" {
			reqPath = "index.html"
		}

		if _, err := fs.Stat(root, reqPath); err != nil {
			serveIndex(w, r, root)
			return
		}

		if strings.HasPrefix(reqPath, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		fileServer.ServeHTTP(w, r)
	})
}

// IsBuilt reports whether the admin SPA was synced before compile
// time. False means scripts/sync-admin-ui.sh has not run.
func IsBuilt() bool {
	root := FS()
	_, err := fs.Stat(root, "index.html")
	return err == nil
}

func serveIndex(w http.ResponseWriter, r *http.Request, root fs.FS) {
	data, err := fs.ReadFile(root, "index.html")
	if err != nil {
		http.Error(w, "agents admin UI not built (run scripts/sync-admin-ui.sh)", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
	_ = r
}
