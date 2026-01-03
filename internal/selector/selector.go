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
func (s *Selector) Select(code string) types.EngineType {
	// Check for network operations - prefer Bun (needs real fetch)
	if strings.Contains(code, "fetch(") || strings.Contains(code, "WebSocket") {
		return types.EngineBun
	}

	// Check for async/await without network - GOJA handles this
	if strings.Contains(code, "async ") || strings.Contains(code, "await ") {
		// GOJA can handle simple async/await
		return types.EngineGOJA
	}

	// Check for file system operations - prefer Bun
	if strings.Contains(code, "readFile") || strings.Contains(code, "writeFile") {
		return types.EngineBun
	}

	// Check for untrusted code indicators - use WASM sandbox
	if strings.Contains(code, "eval(") || strings.Contains(code, "Function(") {
		return types.EngineWASM
	}

	// Default to GOJA for simple scripts
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
