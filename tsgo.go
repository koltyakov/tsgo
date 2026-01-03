// Package tsgo provides a secure TypeScript execution library for Go.
//
// tsgo supports two execution tiers with automatic engine selection:
//   - GOJA: Pure Go JavaScript runtime (fastest, safest)
//   - Bun: High-performance TypeScript runtime (requires Bun installed)
//
// Basic usage:
//
//	executor := tsgo.New()
//	result, err := executor.Execute(ctx, `
//	    const greeting = "Hello, TypeScript!";
//	    greeting;
//	`)
//
// With configuration:
//
//	executor := tsgo.New(
//	    tsgo.WithEngine(tsgo.EngineGOJA),
//	    tsgo.WithTimeout(5 * time.Second),
//	    tsgo.WithGlobals(map[string]any{"userId": 123}),
//	)
package tsgo

import (
	"context"
	"time"

	"github.com/koltyakov/tsgo/internal/engine"
	"github.com/koltyakov/tsgo/internal/engine/bun"
	"github.com/koltyakov/tsgo/internal/engine/goja"
	"github.com/koltyakov/tsgo/internal/monaco"
	"github.com/koltyakov/tsgo/internal/sandbox"
	"github.com/koltyakov/tsgo/internal/selector"
	"github.com/koltyakov/tsgo/internal/sourcemap"
	"github.com/koltyakov/tsgo/internal/transpiler"
	"github.com/koltyakov/tsgo/internal/typegen"
	"github.com/koltyakov/tsgo/internal/types"
)

// Re-export types for public API
type (
	// EngineType represents the TypeScript execution engine type.
	EngineType = types.EngineType

	// Config configures the TypeScript executor.
	Config = types.ExecutorConfig

	// Result represents the result of TypeScript execution.
	Result = types.Result

	// ExecutionError represents an error that occurred during execution.
	ExecutionError = types.ExecutionError

	// SecurityPolicy defines security restrictions for script execution.
	SecurityPolicy = types.SecurityPolicy
)

// Engine type constants
const (
	// EngineAuto automatically selects the best engine for each script.
	EngineAuto = types.EngineAuto

	// EngineGOJA uses the pure-Go GOJA JavaScript runtime.
	EngineGOJA = types.EngineGOJA

	// EngineBun uses the Bun TypeScript runtime (requires Bun installation).
	EngineBun = types.EngineBun
)

// Executor provides TypeScript execution capabilities.
type Executor struct {
	config     types.ExecutorConfig
	transpiler *transpiler.Transpiler
	gojaEng    *goja.Engine
	bunEng     *bun.Engine
	selector   *selector.Selector
}

// Option configures an Executor.
type Option func(*types.ExecutorConfig)

// WithEngine sets the execution engine.
func WithEngine(eng EngineType) Option {
	return func(c *types.ExecutorConfig) {
		c.Engine = eng
	}
}

// WithTimeout sets the execution timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *types.ExecutorConfig) {
		c.Timeout = types.Duration(d)
	}
}

// WithMemoryLimit sets the memory limit in bytes.
func WithMemoryLimit(bytes int64) Option {
	return func(c *types.ExecutorConfig) {
		c.MemoryLimit = bytes
	}
}

// WithGlobals sets the global variables available to scripts.
func WithGlobals(globals map[string]any) Option {
	return func(c *types.ExecutorConfig) {
		c.Globals = globals
	}
}

// WithSecurity sets the security policy.
func WithSecurity(policy SecurityPolicy) Option {
	return func(c *types.ExecutorConfig) {
		c.Security = policy
	}
}

// WithSourceMaps enables or disables source map generation.
func WithSourceMaps(enabled bool) Option {
	return func(c *types.ExecutorConfig) {
		c.SourceMaps = enabled
	}
}

// WithPoolSize sets the worker pool size.
func WithPoolSize(size int) Option {
	return func(c *types.ExecutorConfig) {
		c.PoolSize = size
	}
}

// New creates a new TypeScript executor with the given options.
func New(opts ...Option) *Executor {
	config := types.DefaultConfig()
	for _, opt := range opts {
		opt(&config)
	}

	return &Executor{
		config:     config,
		transpiler: transpiler.New(),
		selector:   selector.New(),
	}
}

// Execute runs TypeScript code and returns the result.
func (e *Executor) Execute(ctx context.Context, code string) (*Result, error) {
	// Validate security policy
	if err := sandbox.ValidateCode(code, e.config.Security.RestrictedGlobals); err != nil {
		return nil, &ExecutionError{
			Message: err.Error(),
			Code:    code,
		}
	}

	// Transpile TypeScript to JavaScript
	js, sourceMap, err := e.transpiler.Transpile(code)
	if err != nil {
		return nil, &ExecutionError{
			Message: "transpilation failed: " + err.Error(),
			Code:    code,
		}
	}

	// Select engine
	engineType := e.config.Engine
	if engineType == EngineAuto {
		engineType = e.selector.Select(code)
	}

	// Get or create engine
	eng, err := e.getEngine(engineType)
	if err != nil {
		return nil, err
	}

	// Execute with timeout
	execCtx := ctx
	if e.config.Timeout > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, e.config.Timeout.Duration())
		defer cancel()
	}

	// Execute
	result, err := eng.Execute(execCtx, js, e.config.Globals)
	if err != nil {
		// Map error through source map if available
		if sourceMap != "" && e.config.SourceMaps {
			sm, parseErr := sourcemap.ParseBase64(sourceMap)
			if parseErr == nil {
				mappedErr := sourcemap.MapError(err, sm)
				return nil, &ExecutionError{
					Message: mappedErr.Error(),
					Code:    code,
				}
			}
		}
		return nil, err
	}

	return result, nil
}

// getEngine returns the appropriate engine, creating it if necessary.
func (e *Executor) getEngine(engineType EngineType) (engine.Engine, error) {
	switch engineType {
	case EngineGOJA:
		if e.gojaEng == nil {
			e.gojaEng = goja.New(goja.Config{
				PoolSize: e.config.PoolSize,
			})
		}
		return e.gojaEng, nil

	case EngineBun:
		if e.bunEng == nil {
			eng, err := bun.New(bun.Config{
				PoolSize: e.config.PoolSize,
			})
			if err != nil {
				return nil, err
			}
			e.bunEng = eng
		}
		return e.bunEng, nil

	default:
		return nil, &ExecutionError{
			Message: "unknown engine type",
		}
	}
}

// Close releases resources held by the executor.
func (e *Executor) Close() error {
	var lastErr error

	if e.gojaEng != nil {
		if err := e.gojaEng.Close(); err != nil {
			lastErr = err
		}
	}

	if e.bunEng != nil {
		if err := e.bunEng.Close(); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// --- Type Generation ---

// TypeBuilder creates TypeScript type definitions for Monaco integration.
type TypeBuilder = typegen.Builder

// NewTypeBuilder creates a new TypeBuilder.
func NewTypeBuilder() *TypeBuilder {
	return typegen.NewBuilder()
}

// GenerateContextTypes generates TypeScript declarations for global context values.
func GenerateContextTypes(globals map[string]any) string {
	return typegen.GenerateContextDTS(globals)
}

// --- Monaco Integration ---

// MonacoConfig configures Monaco editor integration.
type MonacoConfig = monaco.Config

// DefaultMonacoConfig returns the default Monaco configuration.
func DefaultMonacoConfig() MonacoConfig {
	return monaco.DefaultConfig()
}

// MonacoHandler provides WebSocket-based Monaco integration.
type MonacoHandler = monaco.Handler

// NewMonacoHandler creates a new Monaco handler.
func NewMonacoHandler() *MonacoHandler {
	return monaco.NewHandler()
}

// ServeMonaco starts the Monaco integration HTTP server.
func ServeMonaco(cfg MonacoConfig) error {
	return monaco.Serve(cfg)
}

// MonacoClientScript returns JavaScript for Monaco integration.
func MonacoClientScript() string {
	return monaco.ClientScript()
}
