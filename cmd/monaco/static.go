package main

import (
	"embed"
	"net/http"
	"path/filepath"
	"strings"
)

//go:embed index.html styles.css app.js samples.json samples/*.ts
var content embed.FS

// serveStatic handles embedded static file serving.
func (s *server) serveStatic(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/" {
		path = "/index.html"
	}
	filename := strings.TrimPrefix(path, "/")

	data, err := content.ReadFile(filename)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	ext := filepath.Ext(filename)
	if mimeType, ok := mimeTypes[ext]; ok {
		w.Header().Set("Content-Type", mimeType)
	}

	_, _ = w.Write(data)
}
