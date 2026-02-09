// Package selector provides adaptive engine selection based on code analysis.
//
// The selector analyzes TypeScript code to determine the best execution engine:
//   - GOJA: Pure-Go JavaScript runtime, best for simple synchronous code
//   - Bun: External process, required for async/await, fetch, WebSocket, file I/O
//
// Selection is based on single-pass string scanning for efficiency.
package selector

import (
	"github.com/koltyakov/tsgo/internal/codeanalyzer"
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
// Uses shared single-pass token analysis for consistency.
func (s *Selector) Select(code string) types.EngineType {
	if len(code) == 0 {
		return types.EngineGOJA
	}

	analysis := codeanalyzer.Analyze(code)
	if analysis.HasAsync || analysis.HasAwait ||
		analysis.HasCall("fetch") ||
		analysis.HasIdentifier("WebSocket") ||
		analysis.HasCall("readFile") ||
		analysis.HasCall("writeFile") {
		return types.EngineBun
	}

	// Default to GOJA for sync scripts
	return types.EngineGOJA
}

// ============================================================================
// Complexity Analysis
// ============================================================================

// Complexity estimates the computational complexity of code.
// Uses shared single-pass token analysis for consistency.
func (s *Selector) Complexity(code string) int {
	return codeanalyzer.Analyze(code).Complexity
}
