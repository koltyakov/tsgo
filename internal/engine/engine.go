// Package engine defines the interface for TypeScript execution engines.
package engine

import (
	"context"

	"github.com/koltyakov/tsgo/internal/types"
)

// Engine represents a TypeScript/JavaScript execution engine.
type Engine interface {
	// Execute runs JavaScript code with the given globals and returns the result.
	Execute(ctx context.Context, code string, globals map[string]any) (*types.Result, error)

	// Close releases resources held by the engine.
	Close() error
}
