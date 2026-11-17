//go:build webui

package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// Handler returns an http.Handler that serves the embedded web UI files.
// For any path that does not match a real file, it serves index.html
// to support client-side (hash) routing.
func Handler() http.Handler {
	sub, _ := fs.Sub(distFS, "dist")
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		// Try to open the requested file.
		f, err := sub.Open(path)
		if err != nil {
			// SPA fallback: serve index.html for client-side routing.
			w.Header().Set("Cache-Control", "no-cache")
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}
		_ = f.Close()

		// Hashed assets (Vite output) can be cached aggressively.
		if strings.HasPrefix(path, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else if path == "index.html" || path == "sw.js" || path == "manifest.webmanifest" {
			// Revalidate the shell, service worker and manifest so installed
			// apps pick up new builds.
			w.Header().Set("Cache-Control", "no-cache")
		}
		if path == "manifest.webmanifest" {
			w.Header().Set("Content-Type", "application/manifest+json")
		}

		fileServer.ServeHTTP(w, r)
	})
}
