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
	"errors"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/koltyakov/tsgo/internal/contract"
	"github.com/koltyakov/tsgo/internal/engine"
	"github.com/koltyakov/tsgo/internal/engine/bun"
	"github.com/koltyakov/tsgo/internal/engine/goja"
	"github.com/koltyakov/tsgo/internal/monaco"
	"github.com/koltyakov/tsgo/internal/sandbox"
	"github.com/koltyakov/tsgo/internal/selector"
	"github.com/koltyakov/tsgo/internal/sourcemap"
	"github.com/koltyakov/tsgo/internal/transpiler"
	"github.com/koltyakov/tsgo/internal/typegen"
	"github.com/koltyakov/tsgo/internal/typeinfer"
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

	// FunctionDef defines a function injectable into the script context.
	// It provides both Go (for GOJA) and TypeScript (for Bun) implementations.
	FunctionDef = types.FunctionDef
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
// It is safe for concurrent use.
type Executor struct {
	config     types.ExecutorConfig
	transpiler *transpiler.Transpiler
	gojaEng    *goja.Engine
	bunEng     *bun.Engine
	selector   *selector.Selector

	// mu protects engine initialization to prevent data races.
	mu     sync.RWMutex
	closed bool
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

// WithFunctions sets the callable functions available to scripts.
// Each function must provide both a Go implementation (for GOJA) and
// TypeScript code (for Bun engine).
func WithFunctions(functions map[string]types.FunctionDef) Option {
	return func(c *types.ExecutorConfig) {
		c.Functions = functions
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

// ErrExecutorClosed is returned when Execute is called on a closed executor.
var ErrExecutorClosed = errors.New("executor is closed")

// ErrEmptyCode is returned when empty code is provided.
var ErrEmptyCode = errors.New("code cannot be empty")

// Execute runs TypeScript code and returns the result.
// It is safe for concurrent use.
func (e *Executor) Execute(ctx context.Context, code string) (*Result, error) {
	// Check if executor is closed
	e.mu.RLock()
	if e.closed {
		e.mu.RUnlock()
		return nil, ErrExecutorClosed
	}
	e.mu.RUnlock()

	// Validate input
	if len(code) == 0 {
		return nil, ErrEmptyCode
	}

	// Check context before expensive operations
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Validate security policy
	if err := sandbox.ValidateCode(code, e.config.Security.RestrictedGlobals); err != nil {
		return nil, &ExecutionError{
			Message: err.Error(),
			Code:    code,
		}
	}

	// Build TSCode prelude for functions that don't have GoFunc
	// This needs to be prepended BEFORE transpilation so TypeScript is processed
	codeToTranspile := code
	var tsFunctionPrelude strings.Builder
	goFunctions := make(map[string]any) // Functions with GoFunc for GOJA

	for name, fn := range e.config.Functions {
		if fn.GoFunc != nil {
			goFunctions[name] = fn.GoFunc
		}
		if fn.TSCode != "" {
			tsFunctionPrelude.WriteString(fn.TSCode)
			tsFunctionPrelude.WriteByte('\n')
		}
	}

	if tsFunctionPrelude.Len() > 0 {
		codeToTranspile = tsFunctionPrelude.String() + codeToTranspile
	}

	// Select engine first (needed for transpilation format decision)
	engineType := e.config.Engine
	if engineType == EngineAuto {
		engineType = e.selector.Select(code)
	}

	// Check for unsupported features when GOJA is explicitly configured
	// (auto-selection would have chosen Bun if these features were detected)
	hasTopLevelAwait := goja.ContainsTopLevelAwait(code)
	if engineType == EngineGOJA {
		unsupportedFeatures := goja.DetectUnsupportedFeatures(code)
		if len(unsupportedFeatures) > 0 {
			return nil, &ExecutionError{
				Message: goja.FormatUnsupportedFeaturesError(unsupportedFeatures),
				Code:    code,
			}
		}
	}

	// Transpile TypeScript to JavaScript
	// Use ESM format for Bun with top-level await (IIFE doesn't support it)
	var js, sourceMap string
	var err error
	if engineType == EngineBun && hasTopLevelAwait {
		js, sourceMap, err = e.transpiler.TranspileESM(codeToTranspile)
	} else {
		js, sourceMap, err = e.transpiler.Transpile(codeToTranspile)
	}
	if err != nil {
		return nil, &ExecutionError{
			Message: "transpilation failed: " + err.Error(),
			Code:    code,
		}
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

	// Prepare globals and functions for execution
	globals := e.config.Globals
	if globals == nil {
		globals = make(map[string]any)
	}
	jsToExecute := js

	// Inject functions based on engine type
	if len(e.config.Functions) > 0 && engineType == EngineGOJA && len(goFunctions) > 0 {
		// For GOJA: merge Go functions into globals for performance
		// TSCode is already transpiled into the JS, GoFunc overrides it
		merged := make(map[string]any, len(globals)+len(goFunctions))
		maps.Copy(merged, globals)
		maps.Copy(merged, goFunctions)
		globals = merged
	}

	// Final context check before execution
	if err := execCtx.Err(); err != nil {
		return nil, err
	}

	// Execute
	result, err := eng.Execute(execCtx, jsToExecute, globals)
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
// Uses double-checked locking for thread-safe lazy initialization.
func (e *Executor) getEngine(engineType EngineType) (engine.Engine, error) {
	switch engineType {
	case EngineGOJA:
		// Fast path: read lock to check if engine exists
		e.mu.RLock()
		if e.gojaEng != nil {
			eng := e.gojaEng
			e.mu.RUnlock()
			return eng, nil
		}
		e.mu.RUnlock()

		// Slow path: write lock to create engine
		e.mu.Lock()
		defer e.mu.Unlock()

		// Double-check after acquiring write lock
		if e.gojaEng == nil {
			e.gojaEng = goja.New(goja.Config{
				PoolSize: e.config.PoolSize,
			})
		}
		return e.gojaEng, nil

	case EngineBun:
		// Fast path: read lock to check if engine exists
		e.mu.RLock()
		if e.bunEng != nil {
			eng := e.bunEng
			e.mu.RUnlock()
			return eng, nil
		}
		e.mu.RUnlock()

		// Slow path: write lock to create engine
		e.mu.Lock()
		defer e.mu.Unlock()

		// Double-check after acquiring write lock
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
// It is idempotent and safe to call multiple times.
func (e *Executor) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return nil // Already closed
	}
	e.closed = true

	var errs []error

	if e.gojaEng != nil {
		if err := e.gojaEng.Close(); err != nil {
			errs = append(errs, err)
		}
		e.gojaEng = nil
	}

	if e.bunEng != nil {
		if err := e.bunEng.Close(); err != nil {
			errs = append(errs, err)
		}
		e.bunEng = nil
	}

	return errors.Join(errs...)
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

// UnsupportedFeature represents a feature not supported by GOJA engine.
type UnsupportedFeature = goja.UnsupportedFeature

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

// --- Contract Generation ---

// Contract represents the extracted contract from a TypeScript script.
// It includes the output type definition and expected inputs.
type Contract = contract.Contract

// TypeDef represents a TypeScript type definition.
type TypeDef = contract.TypeDef

// ContractProperty represents a property in a contract type.
type ContractProperty = contract.Property

// JSONSchema represents a JSON Schema definition.
type JSONSchema = contract.JSONSchema

// ContractAnalyzer extracts contract definitions from TypeScript code.
type ContractAnalyzer = contract.Analyzer

// NewContractAnalyzer creates a new contract analyzer.
func NewContractAnalyzer() *ContractAnalyzer {
	return contract.NewAnalyzer()
}

// --- Type Inference ---

// TypeInferrer provides TypeScript type inference using the TS Compiler API.
// It requires Bun to be installed for accurate type inference.
type TypeInferrer = typeinfer.Inferrer

// InferenceResult represents the result of type inference.
type InferenceResult = typeinfer.InferenceResult

// NewTypeInferrer creates a new TypeScript type inferrer.
// Returns nil if Bun is not available.
func NewTypeInferrer() *TypeInferrer {
	if !typeinfer.IsBunAvailable() {
		return nil
	}
	return typeinfer.NewInferrer()
}

// IsBunAvailable checks if Bun runtime is available for TypeScript inference.
func IsBunAvailable() bool {
	return typeinfer.IsBunAvailable()
}

// AnalyzeContract extracts the contract definition from TypeScript code.
// It uses the TypeScript Compiler API via Bun for accurate type inference
// when available, falling back to the Go-based regex analyzer otherwise.
//
// Example:
//
//	contract, err := tsgo.AnalyzeContract(`
//	    interface User {
//	        id: number;
//	        name: string;
//	    }
//	    const user: User = { id: 1, name: "Alice" };
//	    export default user;
//	`)
//	// contract.Type describes the User interface
//	// contract.ToTypeScript() returns TypeScript type definition
//	// contract.ToJSONSchema() returns JSON Schema
func AnalyzeContract(code string) (*Contract, error) {
	return AnalyzeContractWithContext(context.Background(), code)
}

// AnalyzeContractWithContext extracts the contract definition from TypeScript code
// with a context for cancellation/timeout support.
func AnalyzeContractWithContext(ctx context.Context, code string) (*Contract, error) {
	// Try TypeScript Compiler API first (more accurate) if Bun is available
	if typeinfer.IsBunAvailable() {
		inferrer := typeinfer.NewInferrer()
		result, err := inferrer.InferDefaultExport(ctx, code)
		if err == nil && result.Error == "" {
			// Convert inference result to Contract
			return inferenceResultToContract(result), nil
		}
		// Fall through to Go-based analyzer on error
	}

	// Fallback: Use Go-based regex analyzer
	return contract.NewAnalyzer().Analyze(code)
}

// InferenceResultToContract converts a TypeInferrer result to a Contract.
// This is useful when you want to use the Contract API methods like ToTypeScript()
// or ToJSONSchema() with an inference result.
func InferenceResultToContract(result *InferenceResult) *Contract {
	return inferenceResultToContract(result)
}

// inferenceResultToContract converts a TypeInferrer result to a Contract
func inferenceResultToContract(result *typeinfer.InferenceResult) *Contract {
	typeDef := contract.ParseTypeString(result.Type, result.Kind)

	// If properties are provided, use them (more accurate)
	if len(result.Properties) > 0 && typeDef.Kind == "object" {
		typeDef.Properties = nil
		for _, prop := range result.Properties {
			propTypeDef := contract.ParseTypeString(prop.Type, "")
			typeDef.Properties = append(typeDef.Properties, contract.Property{
				Name:     prop.Name,
				Type:     propTypeDef,
				Required: !prop.Optional,
			})
		}
	}

	// Set element type for arrays
	if result.ElementType != "" {
		typeDef.ElementType = contract.ParseTypeString(result.ElementType, "")
	}

	return &Contract{
		Name: "Result",
		Type: typeDef,
	}
}
