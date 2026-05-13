package httpapi

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed web_dist/*
var embeddedWebDist embed.FS

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api" || r.URL.Path == "/healthz" {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "not found", nil)
		return
	}
	dist, err := fs.Sub(embeddedWebDist, "web_dist")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "." || name == "" {
		name = "index.html"
	}
	if _, err := fs.Stat(dist, name); err == nil {
		http.FileServer(http.FS(dist)).ServeHTTP(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "not found", nil)
		return
	}
	_, _ = w.Write(data)
}
