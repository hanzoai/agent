//go:build embedded

// UI embedding and route registration for HanzoAgents (embedded build).

package client

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed dist/* dist/**
var UIFiles embed.FS

// RegisterUIRoutes registers the UI routes with the Gin engine.
func RegisterUIRoutes(router *gin.Engine) {
	fmt.Println("Registering embedded UI routes...")

	// Create a sub-filesystem that strips the "dist" prefix
	uiFS, err := fs.Sub(UIFiles, "dist")
	if err != nil {
		panic("Failed to create UI filesystem: " + err.Error())
	}

	fileServer := http.FileServer(http.FS(uiFS))
	serveIndex := func(c *gin.Context) {
		indexHTML, err := UIFiles.ReadFile("dist/index.html")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to load UI index",
			})
			return
		}
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, string(indexHTML))
	}

	router.GET("/ui/*filepath", func(c *gin.Context) {
		path := c.Param("filepath")

		// If accessing root UI path or a directory, serve index.html
		if path == "/" || path == "" || strings.HasSuffix(path, "/") {
			serveIndex(c)
			return
		}

		// Check if it's a static asset by looking for common web asset file extensions
		// This prevents reasoner IDs with dots (like "deepresearchagent.meta_research_methodology_reasoner")
		// from being treated as static assets
		pathLower := strings.ToLower(path)
		isStaticAsset := strings.HasSuffix(pathLower, ".js") ||
			strings.HasSuffix(pathLower, ".css") ||
			strings.HasSuffix(pathLower, ".html") ||
			strings.HasSuffix(pathLower, ".ico") ||
			strings.HasSuffix(pathLower, ".png") ||
			strings.HasSuffix(pathLower, ".jpg") ||
			strings.HasSuffix(pathLower, ".jpeg") ||
			strings.HasSuffix(pathLower, ".gif") ||
			strings.HasSuffix(pathLower, ".svg") ||
			strings.HasSuffix(pathLower, ".woff") ||
			strings.HasSuffix(pathLower, ".woff2") ||
			strings.HasSuffix(pathLower, ".ttf") ||
			strings.HasSuffix(pathLower, ".eot") ||
			strings.HasSuffix(pathLower, ".map") ||
			strings.HasSuffix(pathLower, ".json") ||
			strings.HasSuffix(pathLower, ".xml") ||
			strings.HasSuffix(pathLower, ".txt")

		if isStaticAsset {
			// Try to serve the static file
			http.StripPrefix("/ui", fileServer).ServeHTTP(c.Writer, c.Request)
			return
		}

		// For all other paths (SPA routes), serve index.html
		serveIndex(c)
	})

	// Root serves the same Canvas SPA as /ui/
	router.GET("/", func(c *gin.Context) {
		serveIndex(c)
	})

	// SPA fallback for both /ui/* and root-based routes.
	router.NoRoute(func(c *gin.Context) {
		path := strings.ToLower(c.Request.URL.Path)
		if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/metrics") || strings.HasPrefix(path, "/health") {
			c.JSON(http.StatusNotFound, gin.H{"error": "endpoint not found"})
			return
		}

		// Serve static assets regardless of /ui prefix.
		isStaticAsset := strings.HasSuffix(path, ".js") ||
			strings.HasSuffix(path, ".css") ||
			strings.HasSuffix(path, ".html") ||
			strings.HasSuffix(path, ".ico") ||
			strings.HasSuffix(path, ".png") ||
			strings.HasSuffix(path, ".jpg") ||
			strings.HasSuffix(path, ".jpeg") ||
			strings.HasSuffix(path, ".gif") ||
			strings.HasSuffix(path, ".svg") ||
			strings.HasSuffix(path, ".woff") ||
			strings.HasSuffix(path, ".woff2") ||
			strings.HasSuffix(path, ".ttf") ||
			strings.HasSuffix(path, ".eot") ||
			strings.HasSuffix(path, ".map") ||
			strings.HasSuffix(path, ".json") ||
			strings.HasSuffix(path, ".xml") ||
			strings.HasSuffix(path, ".txt")

		if isStaticAsset {
			// /ui/* static files.
			if strings.HasPrefix(path, "/ui/") {
				http.StripPrefix("/ui", fileServer).ServeHTTP(c.Writer, c.Request)
				return
			}
			// Root static files (for Vite base="/").
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}

		serveIndex(c)
	})
}

// IsUIEmbedded checks if UI files are embedded in the binary.
func IsUIEmbedded() bool {
	// Try to read a file that should exist in the embedded UI
	_, err := UIFiles.ReadFile("dist/index.html")
	return err == nil
}
