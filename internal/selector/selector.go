// Package selector provides adaptive engine selection based on code analysis.
package selector

import (
	"strings"

	"github.com/koltyakov/tsgo/internal/types"
)

// Selector selects the best execution engine for a given script.
type Selector struct{}

// New creates a new engine selector.
func New() *Selector {
	return &Selector{}
}

// Select analyzes code and returns the recommended engine type.
// Optimized to minimize string scanning passes.
func (s *Selector) Select(code string) types.EngineType {
	// Quick check: if no interesting characters, default to GOJA
	if strings.IndexByte(code, 'f') == -1 &&
		strings.IndexByte(code, 'W') == -1 &&
		strings.IndexByte(code, 'a') == -1 &&
		strings.IndexByte(code, 'r') == -1 &&
		strings.IndexByte(code, 'e') == -1 {
		return types.EngineGOJA
	}

	// Check for network operations - prefer Bun (needs real fetch)
	if strings.Contains(code, "fetch(") || strings.Contains(code, "WebSocket") {
		return types.EngineBun
	}

	// Check for file system operations - prefer Bun
	if strings.Contains(code, "readFile") || strings.Contains(code, "writeFile") {
		return types.EngineBun
	}

	// Default to GOJA for all other scripts (handles async/await, eval checks internally)
	return types.EngineGOJA
}

// Complexity estimates the computational complexity of code.
func (s *Selector) Complexity(code string) int {
	complexity := 0

	// Count loops
	complexity += strings.Count(code, "for ")
	complexity += strings.Count(code, "while ")
	complexity += strings.Count(code, "do {")

	// Count function definitions
	complexity += strings.Count(code, "function ")
	complexity += strings.Count(code, "=>")

	// Count conditionals
	complexity += strings.Count(code, "if (")
	complexity += strings.Count(code, "switch (")

	return complexity
}
