// Package engine defines the interface for TypeScript execution engines.
//
// This package provides the common Engine interface that both GOJA (pure Go)
// and Bun (external process) engines implement.
package engine

import (
	"context"

	"github.com/koltyakov/tsgo/internal/types"
)

// ============================================================================
// Engine Interface
// ============================================================================

// Engine represents a TypeScript/JavaScript execution engine.
// All engine implementations must be safe for concurrent use.
type Engine interface {
	// Execute runs JavaScript code with the given globals and returns the result.
	// The code parameter should be pre-transpiled JavaScript.
	// Globals are injected into the execution context.
	Execute(ctx context.Context, code string, globals map[string]any) (*types.Result, error)

	// Close releases resources held by the engine.
	// It should be idempotent and safe to call multiple times.
	Close() error
}
