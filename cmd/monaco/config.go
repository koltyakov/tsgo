package main

import "time"

// Configuration constants.
const (
	codeSeparator   = "// --- Code ---"
	defaultAddr     = "localhost:8080"
	executorTimeout = 5 * time.Second
	warmupTimeout   = 10 * time.Second
)

// MIME types for static file serving.
var mimeTypes = map[string]string{
	".html": "text/html; charset=utf-8",
	".css":  "text/css; charset=utf-8",
	".js":   "application/javascript; charset=utf-8",
	".ts":   "text/plain; charset=utf-8",
	".json": "application/json; charset=utf-8",
}
