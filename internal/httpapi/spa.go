package httpapi

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// hashedAssetPrefix is where Vite writes files whose name contains a content
// hash. A new build gives them new names, so they can be cached forever.
const hashedAssetPrefix = "assets/"

// SPAHandler serves static assets, falling back to index.html for any path
// that is not a real file so client-side routes survive a page reload.
func SPAHandler(assets fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(assets))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" {
			name = "."
		}
		if _, err := fs.Stat(assets, name); err != nil {
			r = r.Clone(r.Context())
			r.URL.Path = "/"
			name = "."
		}
		w.Header().Set("Cache-Control", cacheControlFor(name))
		// Go's mime table has no .webmanifest entry, so the file server would
		// guess text/plain and the browser would have no app to install.
		if strings.HasSuffix(name, ".webmanifest") {
			w.Header().Set("Content-Type", "application/manifest+json")
		}
		fileServer.ServeHTTP(w, r)
	})
}

// cacheControlFor keeps the shell revalidating and lets the hashed bundle be
// cached forever.
//
// The service worker and index.html are how a new deploy reaches a device at
// all. A CDN or a browser heuristic holding either one is how a PWA gets stuck
// on last week's bundle, with no way to push a fix to a coach who only ever
// opens it from a home screen.
func cacheControlFor(name string) string {
	if strings.HasPrefix(name, hashedAssetPrefix) {
		return "public, max-age=31536000, immutable"
	}
	return "no-cache"
}
