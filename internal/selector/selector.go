// Package selector provides adaptive engine selection based on code analysis.
package selector

import (
	"github.com/koltyakov/tsgo/internal/types"
)

// Selector selects the best execution engine for a given script.
type Selector struct{}

// New creates a new engine selector.
func New() *Selector {
	return &Selector{}
}

// Bun-requiring patterns as byte slices for zero-allocation comparison
var (
	patternAsync     = []byte("async ")
	patternAwait     = []byte("await ")
	patternFetch     = []byte("fetch(")
	patternWebSocket = []byte("WebSocket")
	patternReadFile  = []byte("readFile")
	patternWriteFile = []byte("writeFile")
)

// Select analyzes code and returns the recommended engine type.
// Uses single-pass scanning with byte-level comparison for efficiency.
func (s *Selector) Select(code string) types.EngineType {
	n := len(code)
	if n == 0 {
		return types.EngineGOJA
	}

	// Convert to bytes once for faster indexing
	b := []byte(code)

	for i := range n {
		c := b[i]

		switch c {
		case 'a':
			// Check for "async " or "await " (6 chars each)
			if i+6 <= n {
				if matchBytes(b[i:i+6], patternAsync) || matchBytes(b[i:i+6], patternAwait) {
					return types.EngineBun
				}
			}
		case 'f':
			// Check for "fetch(" (6 chars)
			if i+6 <= n && matchBytes(b[i:i+6], patternFetch) {
				return types.EngineBun
			}
		case 'W':
			// Check for "WebSocket" (9 chars)
			if i+9 <= n && matchBytes(b[i:i+9], patternWebSocket) {
				return types.EngineBun
			}
		case 'r':
			// Check for "readFile" (8 chars)
			if i+8 <= n && matchBytes(b[i:i+8], patternReadFile) {
				return types.EngineBun
			}
		case 'w':
			// Check for "writeFile" (9 chars)
			if i+9 <= n && matchBytes(b[i:i+9], patternWriteFile) {
				return types.EngineBun
			}
		}
	}

	// Default to GOJA for sync scripts
	return types.EngineGOJA
}

// matchBytes compares two byte slices for equality.
// Inlined for performance in hot path.
func matchBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Complexity estimates the computational complexity of code.
// Uses single-pass counting for efficiency.
func (s *Selector) Complexity(code string) int {
	complexity := 0
	n := len(code)
	b := []byte(code)

	for i := range n {
		c := b[i]

		switch c {
		case 'f':
			// "for " or "function "
			if i+4 <= n && string(b[i:i+4]) == "for " {
				complexity++
			} else if i+9 <= n && string(b[i:i+9]) == "function " {
				complexity++
			}
		case 'w':
			// "while "
			if i+6 <= n && string(b[i:i+6]) == "while " {
				complexity++
			}
		case 'd':
			// "do {"
			if i+4 <= n && string(b[i:i+4]) == "do {" {
				complexity++
			}
		case '=':
			// "=>" (arrow function)
			if i+1 < n && b[i+1] == '>' {
				complexity++
			}
		case 'i':
			// "if ("
			if i+4 <= n && string(b[i:i+4]) == "if (" {
				complexity++
			}
		case 's':
			// "switch ("
			if i+8 <= n && string(b[i:i+8]) == "switch (" {
				complexity++
			}
		}
	}

	return complexity
}
