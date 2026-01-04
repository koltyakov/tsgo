// Package selector provides adaptive engine selection based on code analysis.
//
// The selector analyzes TypeScript code to determine the best execution engine:
//   - GOJA: Pure-Go JavaScript runtime, best for simple synchronous code
//   - Bun: External process, required for async/await, fetch, WebSocket, file I/O
//
// Selection is based on single-pass string scanning for efficiency.
package selector

import (
	"github.com/koltyakov/tsgo/internal/types"
)

// ============================================================================
// Selector
// ============================================================================

// Selector selects the best execution engine for a given script.
type Selector struct{}

// New creates a new engine selector.
func New() *Selector {
	return &Selector{}
}

// ============================================================================
// Engine Selection
// ============================================================================

// Select analyzes code and returns the recommended engine type.
// Uses single-pass scanning with direct string indexing for efficiency.
func (s *Selector) Select(code string) types.EngineType {
	n := len(code)
	if n == 0 {
		return types.EngineGOJA
	}

	// Direct string indexing is as fast as byte slice in Go
	for i := range n {
		c := code[i]

		switch c {
		case 'a':
			// Check for "async " or "await " (6 chars each)
			if i+6 <= n {
				sub := code[i : i+6]
				if sub == "async " || sub == "await " {
					return types.EngineBun
				}
			}
		case 'f':
			// Check for "fetch(" (6 chars)
			if i+6 <= n && code[i:i+6] == "fetch(" {
				return types.EngineBun
			}
		case 'W':
			// Check for "WebSocket" (9 chars)
			if i+9 <= n && code[i:i+9] == "WebSocket" {
				return types.EngineBun
			}
		case 'r':
			// Check for "readFile" (8 chars)
			if i+8 <= n && code[i:i+8] == "readFile" {
				return types.EngineBun
			}
		case 'w':
			// Check for "writeFile" (9 chars)
			if i+9 <= n && code[i:i+9] == "writeFile" {
				return types.EngineBun
			}
		}
	}

	// Default to GOJA for sync scripts
	return types.EngineGOJA
}

// ============================================================================
// Complexity Analysis
// ============================================================================

// Complexity estimates the computational complexity of code.
// Uses single-pass counting for efficiency.
func (s *Selector) Complexity(code string) int {
	complexity := 0
	n := len(code)

	for i := range n {
		c := code[i]

		switch c {
		case 'f':
			// "for " or "function "
			if i+4 <= n && code[i:i+4] == "for " {
				complexity++
			} else if i+9 <= n && code[i:i+9] == "function " {
				complexity++
			}
		case 'w':
			// "while "
			if i+6 <= n && code[i:i+6] == "while " {
				complexity++
			}
		case 'd':
			// "do {"
			if i+4 <= n && code[i:i+4] == "do {" {
				complexity++
			}
		case '=':
			// "=>" (arrow function)
			if i+1 < n && code[i+1] == '>' {
				complexity++
			}
		case 'i':
			// "if ("
			if i+4 <= n && code[i:i+4] == "if (" {
				complexity++
			}
		case 's':
			// "switch ("
			if i+8 <= n && code[i:i+8] == "switch (" {
				complexity++
			}
		}
	}

	return complexity
}
