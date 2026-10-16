package web

import (
	"io/fs"
	"net/http"
	"strings"
)

// NewStaticHandler serves the embedded web app build, falling back to
// index.html for any path that isn't a real asset — so a fresh load or
// reload of a client-side route (e.g. /accounts/:id/operations) still
// resolves to the app's own client-side router, which then takes over.
func NewStaticHandler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServerFS(sub)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(sub, path); err != nil {
			r2 := new(http.Request)
			*r2 = *r
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
