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
	// Validate inspects pre-transpile source code for features this engine
	// cannot run. It is called once per Execute, before transpilation, so
	// the check sees user TypeScript as written. Returning a non-nil error
	// short-circuits Execute with that error. Engines with no unsupported
	// features should return nil.
	Validate(code string) error

	// WantsESM reports whether the given source must be transpiled as an
	// ES module instead of the default IIFE wrapper. Engines that don't
	// support ESM or top-level await should always return false.
	WantsESM(code string) bool

	// Execute runs JavaScript code with the given globals and returns the result.
	// The code parameter should be pre-transpiled JavaScript.
	// Globals are injected into the execution context.
	Execute(ctx context.Context, code string, globals map[string]any) (*types.Result, error)

	// Close releases resources held by the engine.
	// It should be idempotent and safe to call multiple times.
	Close() error
}
