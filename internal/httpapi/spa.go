package httpapi

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

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
		}
		fileServer.ServeHTTP(w, r)
	})
}
