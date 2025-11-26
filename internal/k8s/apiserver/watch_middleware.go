package apiserver

import (
	"net/http"
	"strings"
)

// WithoutWatchBookmarks forces allowWatchBookmarks=false on watch requests so kubectl
// doesn't emit <unknown> bookmark rows.
func WithoutWatchBookmarks(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet {
			q := req.URL.Query()
			if strings.EqualFold(q.Get("watch"), "true") {
				if v := q.Get("allowWatchBookmarks"); v == "" || strings.EqualFold(v, "true") {
					q.Set("allowWatchBookmarks", "false")
					req.URL.RawQuery = q.Encode()
				}
			}
		}
		handler.ServeHTTP(w, req)
	})
}

