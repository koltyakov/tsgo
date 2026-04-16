// Package tsgo provides a secure TypeScript execution library for Go.
//
// tsgo supports two execution tiers with automatic engine selection:
//   - GOJA: Pure Go JavaScript runtime (fastest, safest)
//   - Bun: High-performance TypeScript runtime (requires Bun installed)
//
// # Basic Usage
//
//	executor := tsgo.New()
//	result, err := executor.Execute(ctx, `
//	    const greeting = "Hello, TypeScript!";
//	    greeting;
//	`)
//
// # Configuration
//
//	executor := tsgo.New(
//	    tsgo.WithEngine(tsgo.EngineGOJA),
//	    tsgo.WithTimeout(5 * time.Second),
//	    tsgo.WithGlobals(map[string]any{"userId": 123}),
//	)
//
// # Type Inference
//
//	contract, err := tsgo.AnalyzeContract(code)
//	ts := contract.ToTypeScript()  // TypeScript type definition
//	js := contract.ToJSONSchema()  // JSON Schema
package tsgo

import (
	"context"
	"errors"
	"log/slog"
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

// ============================================================================
// Type Re-exports
// ============================================================================

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

	// ExecutorStats contains runtime statistics for the executor.
	ExecutorStats = types.ExecutorStats
)

// ============================================================================
// Engine Constants
// ============================================================================

const (
	// EngineAuto automatically selects the best engine for each script.
	EngineAuto = types.EngineAuto

	// EngineGOJA uses the pure-Go GOJA JavaScript runtime.
	EngineGOJA = types.EngineGOJA

	// EngineBun uses the Bun TypeScript runtime (requires Bun installation).
	EngineBun = types.EngineBun
)

// ============================================================================
// Executor Errors
// ============================================================================

var (
	// ErrExecutorClosed is returned when Execute is called on a closed executor.
	ErrExecutorClosed = errors.New("executor is closed")

	// ErrEmptyCode is returned when empty code is provided.
	ErrEmptyCode = errors.New("code cannot be empty")
)

// ============================================================================
// Executor
// ============================================================================

// Executor provides TypeScript execution capabilities.
// It is safe for concurrent use and manages engine lifecycle automatically.
type Executor struct {
	config     types.ExecutorConfig
	transpiler *transpiler.Transpiler
	selector   *selector.Selector

	// Precomputed from config.Functions — immutable after New.
	tsPrelude   string         // TSCode blocks concatenated with trailing newlines
	goFunctions map[string]any // GoFunc-backed entries, ready for goja injection
	gojaGlobals map[string]any // config.Globals ∪ goFunctions (for GOJA path)

	gojaOnce sync.Once
	gojaEng  *goja.Engine

	bunOnce    sync.Once
	bunEng     *bun.Engine
	bunInitErr error

	mu               sync.Mutex // Protects closed flag and Close() sequencing
	closed           bool
	activeExecutions sync.WaitGroup
}

// ============================================================================
// Options
// ============================================================================

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
//
// Deprecated: MemoryLimit is currently not enforced by the runtime.
// This option exists for future compatibility but has no effect.
// Use WithTimeout to limit execution time instead.
func WithMemoryLimit(bytes int64) Option {
	return func(c *types.ExecutorConfig) {
		c.MemoryLimit = bytes
	}
}

// WithGlobals sets the global variables available to scripts.
func WithGlobals(globals map[string]any) Option {
	return func(c *types.ExecutorConfig) {
		c.Globals = maps.Clone(globals)
	}
}

// WithFunctions sets the callable functions available to scripts.
// Each function must provide both a Go implementation (for GOJA) and
// TypeScript code (for Bun engine).
func WithFunctions(functions map[string]types.FunctionDef) Option {
	return func(c *types.ExecutorConfig) {
		c.Functions = maps.Clone(functions)
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

// WithDebugLogger sets a logger for debug output.
// This enables detailed logging of engine selection, transpilation,
// cache hits/misses, and execution phases for troubleshooting.
//
// Passing nil is a no-op; the executor's default discard logger is preserved.
//
// Example:
//
//	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
//	    Level: slog.LevelDebug,
//	}))
//	executor := tsgo.New(tsgo.WithDebugLogger(logger))
func WithDebugLogger(logger *slog.Logger) Option {
	return func(c *types.ExecutorConfig) {
		if logger == nil {
			return
		}
		c.Logger = logger
	}
}

// WithBackgroundWarmup enables background warmup for the Bun engine.
// When enabled, Bun worker processes are started in background goroutines,
// allowing New() to return immediately (~1ms instead of ~120ms).
// This is useful when low initialization latency is more important than
// immediate readiness for the first request.
//
// Example:
//
//	executor := tsgo.New(
//	    tsgo.WithEngine(tsgo.EngineBun),
//	    tsgo.WithBackgroundWarmup(true),
//	)
func WithBackgroundWarmup(enabled bool) Option {
	return func(c *types.ExecutorConfig) {
		c.BackgroundWarmup = enabled
	}
}

// ============================================================================
// Constructor
// ============================================================================

// New creates a new TypeScript executor with the given options.
// It panics if options produce an invalid configuration.
func New(opts ...Option) *Executor {
	executor, err := NewWithError(opts...)
	if err != nil {
		panic(err)
	}
	return executor
}

// NewWithError creates a new TypeScript executor with the given options.
// It returns an error if configuration validation fails.
func NewWithError(opts ...Option) (*Executor, error) {
	config := types.DefaultConfig()
	for _, opt := range opts {
		opt(&config)
	}
	cloneConfigMaps(&config)
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if config.Security.RestrictedGlobals == nil {
		config.Security.RestrictedGlobals = sandbox.RestrictedGlobals()
	}
	if config.Logger == nil {
		config.Logger = slog.New(slog.DiscardHandler)
	}

	tsPrelude, goFunctions := precomputeFunctions(config.Functions)
	gojaGlobals := mergeGlobals(config.Globals, goFunctions)

	return &Executor{
		config:      config,
		transpiler:  transpiler.New(),
		selector:    selector.New(),
		tsPrelude:   tsPrelude,
		goFunctions: goFunctions,
		gojaGlobals: gojaGlobals,
	}, nil
}

// precomputeFunctions builds the TS prelude and Go function map once at
// construction. Config.Functions is immutable after New, so there's no reason
// to rebuild these on every Execute.
func precomputeFunctions(functions map[string]types.FunctionDef) (prelude string, goFunctions map[string]any) {
	if len(functions) == 0 {
		return "", nil
	}
	var sb strings.Builder
	goFunctions = make(map[string]any, len(functions))
	for name, fn := range functions {
		if fn.GoFunc != nil {
			goFunctions[name] = fn.GoFunc
		}
		if fn.TSCode != "" {
			sb.WriteString(fn.TSCode)
			sb.WriteByte('\n')
		}
	}
	if len(goFunctions) == 0 {
		goFunctions = nil
	}
	return sb.String(), goFunctions
}

// mergeGlobals unions base globals with Go function bindings. Go functions
// take precedence over any same-named global, mirroring prior behavior.
func mergeGlobals(base, goFunctions map[string]any) map[string]any {
	if len(goFunctions) == 0 {
		if base == nil {
			return nil
		}
		return maps.Clone(base)
	}
	merged := make(map[string]any, len(base)+len(goFunctions))
	maps.Copy(merged, base)
	maps.Copy(merged, goFunctions)
	return merged
}

func cloneConfigMaps(config *types.ExecutorConfig) {
	config.Globals = maps.Clone(config.Globals)
	config.Functions = maps.Clone(config.Functions)
}

// ============================================================================
// Execution
// ============================================================================

// ExecuteOptions are per-call overrides for a single Execute.
// They layer on top of the Executor's base configuration without mutating it
// and without invalidating the transpile/engine caches, which makes them
// suitable for request-scoped values (e.g. a per-user userId).
type ExecuteOptions struct {
	// Globals are per-call globals that are merged on top of the executor's
	// base Globals (and Go function bindings for GOJA). Keys in Globals
	// override same-named keys in the base set for this call only.
	Globals map[string]any

	// Timeout overrides the executor's configured Timeout for this call.
	// A zero value means "use the executor's configured timeout".
	// Use a negative value to explicitly disable the timeout for one call.
	Timeout time.Duration
}

// Execute runs TypeScript code and returns the result.
// It is safe for concurrent use.
func (e *Executor) Execute(ctx context.Context, code string) (*Result, error) {
	return e.ExecuteWith(ctx, code, ExecuteOptions{})
}

// ExecuteWith runs TypeScript code with per-call option overrides.
// See ExecuteOptions for the set of overridable fields. It is safe for
// concurrent use and shares the same engine/transpile caches as Execute.
func (e *Executor) ExecuteWith(ctx context.Context, code string, opts ExecuteOptions) (*Result, error) {
	overallStart := time.Now()

	if err := e.acquireExecutionSlot(code); err != nil {
		return nil, err
	}
	defer e.activeExecutions.Done()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := e.validateSecurity(code); err != nil {
		return nil, err
	}

	// Prepend TS prelude (precomputed at New) to the user's code.
	codeToTranspile := code
	if e.tsPrelude != "" {
		codeToTranspile = e.tsPrelude + code
	}

	// Select engine (needed for transpilation format decision)
	engineType := e.selectEngine(codeToTranspile)
	e.config.Logger.Debug("engine selected", "engine", engineType.String())

	// Get or create engine, then let it reject unsupported source features
	// before we spend any cycles transpiling.
	eng, err := e.getEngine(engineType)
	if err != nil {
		return nil, err
	}
	if err := eng.Validate(codeToTranspile); err != nil {
		return nil, err
	}

	// Transpile TypeScript to JavaScript. The engine dictates the module
	// format it wants (IIFE vs ESM); the Executor just dispatches.
	wantsESM := eng.WantsESM(codeToTranspile)
	js, sourceMap, cacheHit, transpileTime, err := e.transpileCode(codeToTranspile, wantsESM)
	if err != nil {
		return nil, e.wrapTranspileError(err, code)
	}
	e.config.Logger.Debug("transpilation complete",
		"cacheHit", cacheHit,
		"transpileTime", transpileTime,
		"esm", wantsESM,
	)

	// Prepare execution context and globals
	execCtx, cancel := e.prepareContext(ctx, opts.Timeout)
	defer cancel()
	globals := e.globalsFor(engineType, opts.Globals)

	// Final context check before execution
	if err := execCtx.Err(); err != nil {
		return nil, err
	}

	// Execute
	result, err := eng.Execute(execCtx, js, globals)
	if err != nil {
		e.config.Logger.Debug("execution failed", "error", err.Error())
		return nil, e.mapExecutionError(err, sourceMap, code)
	}

	result.Metrics.TranspileTime = transpileTime
	result.Metrics.CacheHit = cacheHit
	result.Metrics.TotalTime = time.Since(overallStart)
	if e.config.SourceMaps {
		result.SourceMap = sourceMap
	}

	e.config.Logger.Debug("execution complete",
		"engine", engineType.String(),
		"totalTime", result.Metrics.TotalTime,
		"executionTime", result.Metrics.ExecutionTime,
		"transpileTime", result.Metrics.TranspileTime,
		"cacheHit", result.Metrics.CacheHit,
	)

	return result, nil
}

// acquireExecutionSlot validates execution state and tracks active calls.
func (e *Executor) acquireExecutionSlot(code string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return ErrExecutorClosed
	}
	if len(code) == 0 {
		return ErrEmptyCode
	}

	e.activeExecutions.Add(1)
	return nil
}

// validateSecurity checks the code against security policy.
func (e *Executor) validateSecurity(code string) error {
	restricted := e.config.Security.RestrictedGlobals
	if len(e.config.Security.AllowedGlobals) > 0 {
		restricted = filterRestrictedGlobals(restricted, e.config.Security.AllowedGlobals)
	}

	if err := sandbox.ValidateCode(code, restricted); err != nil {
		return &ExecutionError{
			Message: err.Error(),
			Code:    code,
		}
	}
	return nil
}

func filterRestrictedGlobals(restricted, allowed []string) []string {
	if len(restricted) == 0 || len(allowed) == 0 {
		return restricted
	}

	allowedSet := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allowedSet[name] = struct{}{}
	}

	filtered := make([]string, 0, len(restricted))
	for _, name := range restricted {
		if _, ok := allowedSet[name]; ok {
			continue
		}
		filtered = append(filtered, name)
	}

	return filtered
}

// selectEngine determines which engine to use for execution.
func (e *Executor) selectEngine(code string) EngineType {
	if e.config.Engine == EngineAuto {
		return e.selector.Select(code)
	}
	return e.config.Engine
}

// transpileCode converts TypeScript to JavaScript. When wantsESM is true the
// output is an ES module (needed for top-level await); otherwise the default
// IIFE wrapper is used.
func (e *Executor) transpileCode(code string, wantsESM bool) (js, sourceMap string, cacheHit bool, transpileTime time.Duration, err error) {
	if wantsESM {
		return e.transpiler.TranspileESM(code)
	}
	return e.transpiler.Transpile(code)
}

// wrapTranspileError wraps a transpilation error in an ExecutionError.
func (e *Executor) wrapTranspileError(err error, code string) error {
	return &ExecutionError{
		Message: "transpilation failed: " + err.Error(),
		Code:    code,
	}
}

// prepareContext creates an execution context with timeout if configured.
// A non-zero override takes precedence; a negative override disables the
// timeout for this call.
func (e *Executor) prepareContext(ctx context.Context, override time.Duration) (context.Context, context.CancelFunc) {
	timeout := e.config.Timeout.Duration()
	if override != 0 {
		timeout = override
	}
	if timeout > 0 {
		return context.WithTimeout(ctx, timeout)
	}
	return ctx, func() {}
}

// globalsFor returns the globals map for the selected engine, layering the
// per-call overrides on top of the executor's cached base map. For GOJA the
// base includes Go-backed function bindings. For Bun the base is just the
// configured Globals — TSCode has already been prepended as the prelude.
//
// If overrides is nil the cached base map is returned directly (callers must
// not mutate it). Otherwise a fresh merged map is returned.
func (e *Executor) globalsFor(engineType EngineType, overrides map[string]any) map[string]any {
	base := e.config.Globals
	if engineType == EngineGOJA {
		base = e.gojaGlobals
	}
	if len(overrides) == 0 {
		return base
	}
	merged := make(map[string]any, len(base)+len(overrides))
	maps.Copy(merged, base)
	maps.Copy(merged, overrides)
	return merged
}

// mapExecutionError maps errors through source maps when available.
func (e *Executor) mapExecutionError(err error, sourceMap, code string) error {
	if sourceMap != "" && e.config.SourceMaps {
		sm, parseErr := sourcemap.ParseBase64(sourceMap)
		if parseErr == nil {
			mappedErr := sourcemap.MapError(err, sm)
			return &ExecutionError{
				Message: mappedErr.Error(),
				Code:    code,
			}
		}
	}
	return err
}

// ============================================================================
// Engine Management
// ============================================================================

// getEngine returns the appropriate engine, creating it if necessary.
// Uses double-checked locking for thread-safe lazy initialization.
func (e *Executor) getEngine(engineType EngineType) (engine.Engine, error) {
	switch engineType {
	case EngineGOJA:
		return e.getOrCreateGOJA()
	case EngineBun:
		return e.getOrCreateBun()
	default:
		return nil, &ExecutionError{Message: "unknown engine type"}
	}
}

// getOrCreateGOJA returns the GOJA engine, creating it at most once.
func (e *Executor) getOrCreateGOJA() (*goja.Engine, error) {
	e.gojaOnce.Do(func() {
		e.gojaEng = goja.New(goja.Config{
			PoolSize: e.config.PoolSize,
		})
	})
	return e.gojaEng, nil
}

// getOrCreateBun returns the Bun engine, creating it at most once.
// Initialization errors are memoized so repeat callers see the same failure.
func (e *Executor) getOrCreateBun() (*bun.Engine, error) {
	e.bunOnce.Do(func() {
		e.bunEng, e.bunInitErr = bun.New(bun.Config{
			PoolSize:         e.config.PoolSize,
			Security:         e.config.Security,
			BackgroundWarmup: e.config.BackgroundWarmup,
		})
	})
	return e.bunEng, e.bunInitErr
}

// ============================================================================
// Lifecycle
// ============================================================================

// Close releases resources held by the executor.
// It is idempotent and safe to call multiple times.
func (e *Executor) Close() error {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil // Already closed
	}
	e.closed = true
	e.mu.Unlock()

	// Wait for in-flight executions to complete before tearing down engines.
	e.activeExecutions.Wait()

	e.mu.Lock()
	defer e.mu.Unlock()

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

// Stats returns runtime statistics for the executor.
// This is useful for monitoring, debugging, and capacity planning.
//
// Example:
//
//	stats := executor.Stats()
//	fmt.Printf("Cache size: %d/%d\n", stats.TranspilerCacheSize, stats.TranspilerCacheCapacity)
func (e *Executor) Stats() types.ExecutorStats {
	cacheSize, cacheCapacity := e.transpiler.CacheStats()

	e.mu.Lock()
	defer e.mu.Unlock()

	return types.ExecutorStats{
		EngineConfigured:        e.config.Engine,
		GOJAActive:              e.gojaEng != nil,
		BunActive:               e.bunEng != nil,
		TranspilerCacheSize:     cacheSize,
		TranspilerCacheCapacity: cacheCapacity,
	}
}

// ============================================================================
// Type Generation
// ============================================================================

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

// ============================================================================
// Monaco Integration
// ============================================================================

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

// ============================================================================
// Contract Analysis
// ============================================================================

// Contract represents the extracted contract from a TypeScript script.
// It includes the output type definition and expected inputs. The Source
// field tells you whether the contract came from the TypeScript Compiler API
// (ContractSourceTSCompiler, most accurate) or the Go-based heuristic
// fallback (ContractSourceHeuristic, best-effort).
type Contract = contract.Contract

// ContractSource identifies which analyzer produced a Contract.
type ContractSource = contract.ContractSource

const (
	// ContractSourceTSCompiler indicates the Contract was produced by the TypeScript Compiler API (requires Bun).
	ContractSourceTSCompiler = contract.SourceTSCompiler
	// ContractSourceHeuristic indicates the Contract was produced by the Go-based heuristic analyzer.
	ContractSourceHeuristic = contract.SourceHeuristic
)

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

// ============================================================================
// Type Inference
// ============================================================================

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

// ============================================================================
// Contract Generation Helpers
// ============================================================================

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
		defer func() { _ = inferrer.Close() }()

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
		Name:   "Result",
		Type:   typeDef,
		Source: contract.SourceTSCompiler,
	}
}
