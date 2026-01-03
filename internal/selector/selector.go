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

// Select analyzes code and returns the recommended engine type.
// Uses single-pass scanning for efficiency.
func (s *Selector) Select(code string) types.EngineType {
	// Single-pass scan for all patterns
	n := len(code)
	if n == 0 {
		return types.EngineGOJA
	}

	for i := range n {
		c := code[i]

		// Check for 'a' - async, await
		if c == 'a' && i+5 < n {
			// Check for "async " (6 chars)
			if i+6 <= n && code[i:i+6] == "async " {
				return types.EngineBun
			}
			// Check for "await " (6 chars)
			if i+6 <= n && code[i:i+6] == "await " {
				return types.EngineBun
			}
		}

		// Check for 'f' - fetch(
		if c == 'f' && i+6 < n && code[i:i+6] == "fetch(" {
			return types.EngineBun
		}

		// Check for 'W' - WebSocket
		if c == 'W' && i+9 <= n && code[i:i+9] == "WebSocket" {
			return types.EngineBun
		}

		// Check for 'r' - readFile
		if c == 'r' && i+8 <= n && code[i:i+8] == "readFile" {
			return types.EngineBun
		}

		// Check for 'w' - writeFile
		if c == 'w' && i+9 <= n && code[i:i+9] == "writeFile" {
			return types.EngineBun
		}
	}

	// Default to GOJA for sync scripts
	return types.EngineGOJA
}

// Complexity estimates the computational complexity of code.
// Uses single-pass counting for efficiency.
func (s *Selector) Complexity(code string) int {
	complexity := 0
	n := len(code)

	for i := 0; i < n; i++ {
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
