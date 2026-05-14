package server

import (
	"errors"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// SPAHandler serves a Vite-style single-page app from sub: assets resolve to
// their real file, unknown paths fall back to index.html so client-side
// routes like /runs/abc render the shell instead of a hard 404. The fallback
// is gated on GET/HEAD and a non-/api/ prefix so JSON endpoints stay
// authoritative 404s rather than masquerading as HTML.
func SPAHandler(sub fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			fileServer.ServeHTTP(w, r)
			return
		}
		clean := path.Clean(r.URL.Path)
		if clean == "/" || clean == "." {
			fileServer.ServeHTTP(w, r)
			return
		}
		rel := strings.TrimPrefix(clean, "/")
		if f, err := sub.Open(rel); err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		} else if !errors.Is(err, fs.ErrNotExist) {
			fileServer.ServeHTTP(w, r)
			return
		}
		serveIndex(w, r, sub)
	})
}

func serveIndex(w http.ResponseWriter, r *http.Request, sub fs.FS) {
	data, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(data)
}
