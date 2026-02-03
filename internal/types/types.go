// Package types defines core types for the tsgo TypeScript executor.
//
// This package contains the fundamental types used throughout tsgo:
//   - Engine types and configuration
//   - Execution results and errors
//   - Security policies
//   - Function definitions for injection
package types

import (
	"log/slog"
	"time"
)

// ============================================================================
// Engine Types
// ============================================================================

// EngineType represents the execution engine to use.
type EngineType int

const (
	// EngineAuto automatically selects the best engine based on heuristics.
	EngineAuto EngineType = iota
	// EngineGOJA uses the pure-Go GOJA JavaScript runtime.
	EngineGOJA
	// EngineBun uses an external Bun process for execution.
	EngineBun
)

// String returns the string representation of the engine type.
func (e EngineType) String() string {
	switch e {
	case EngineAuto:
		return "auto"
	case EngineGOJA:
		return "goja"
	case EngineBun:
		return "bun"
	default:
		return "unknown"
	}
}

// ============================================================================
// Duration Wrapper
// ============================================================================

// Duration wraps time.Duration for config purposes.
type Duration time.Duration

// Duration returns the underlying time.Duration.
func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

// ============================================================================
// Security
// ============================================================================

// SecurityPolicy defines the security constraints for script execution.
type SecurityPolicy struct {
	// NetworkAccess allows the script to make network requests.
	NetworkAccess bool `json:"networkAccess,omitempty"`
	// DiskAccess allows the script to read/write files.
	DiskAccess bool `json:"diskAccess,omitempty"`
	// AllowedPaths restricts file access to these paths (if DiskAccess is true).
	AllowedPaths []string `json:"allowedPaths,omitempty"`
	// RestrictedGlobals are globals that should be blocked from scripts.
	RestrictedGlobals []string `json:"restrictedGlobals,omitempty"`
	// AllowedGlobals are restricted globals that are explicitly allowed.
	// This provides an opt-in escape hatch for specific globals like "fetch" or "process".
	AllowedGlobals []string `json:"allowedGlobals,omitempty"`
	// MaxExecutionTime limits script execution time.
	MaxExecutionTime time.Duration `json:"maxExecutionTime,omitempty"`

	// NOTE: DiskAccess, AllowedPaths, and MaxExecutionTime are currently
	// not enforced by the runtime. NetworkAccess is enforced in Bun workers,
	// and RestrictedGlobals/AllowedGlobals are enforced by static validation.
}

// Default security policy values.
const (
	DefaultMaxExecutionTime = 30 * time.Second
)

// DefaultSecurityPolicy returns a restrictive default security policy.
func DefaultSecurityPolicy() SecurityPolicy {
	return SecurityPolicy{
		NetworkAccess:    false,
		DiskAccess:       false,
		AllowedPaths:     nil,
		MaxExecutionTime: DefaultMaxExecutionTime,
	}
}

// ============================================================================
// Function Injection
// ============================================================================

// FunctionDef defines a function that can be injected into the script execution context.
//
// There are two ways to define functions:
//
//  1. TSCode only (recommended): Define the function once in TypeScript/JavaScript.
//     Works for both GOJA and Bun engines with no code duplication.
//
//     tsgo.FunctionDef{
//     TSCode: `function sum(a: number, b: number): number { return a + b; }`,
//     }
//
//  2. TSCode + GoFunc (performance optimization): Provide both implementations.
//     GOJA uses GoFunc (faster, native Go), Bun uses TSCode.
//
//     tsgo.FunctionDef{
//     TSCode: `function sum(a: number, b: number): number { return a + b; }`,
//     GoFunc: func(a, b float64) float64 { return a + b },
//     }
//
// When GoFunc is nil, both engines execute TSCode by prepending it to the script.
// When GoFunc is provided, GOJA injects it as a native function for better performance.
type FunctionDef struct {
	// TSCode is the TypeScript/JavaScript implementation.
	// This is the primary definition and works for both engines.
	// Example: `function sum(a: number, b: number): number { return a + b; }`
	TSCode string

	// GoFunc is an optional Go function implementation for GOJA engine.
	// When provided, GOJA uses this instead of TSCode for better performance.
	// The function signature should match the TypeScript signature.
	// When nil, GOJA executes TSCode like Bun does.
	GoFunc any
}

// ============================================================================
// Configuration
// ============================================================================

// Default configuration values.
const (
	DefaultTimeout     = 30 * time.Second
	DefaultMemoryLimit = 64 * 1024 * 1024 // 64MB
)

// ExecutorConfig configures the TypeScript executor.
type ExecutorConfig struct {
	// Engine specifies which engine to use (EngineAuto for automatic selection).
	Engine EngineType
	// Timeout sets the execution timeout.
	Timeout Duration
	// MemoryLimit sets the memory limit in bytes.
	// Deprecated: MemoryLimit is currently not enforced by the runtime.
	// This field exists for future compatibility but has no effect.
	MemoryLimit int64
	// Globals are variables injected into the global scope.
	Globals map[string]any
	// Functions are callable functions injected into the global scope.
	// Use this for helper functions that scripts can call.
	Functions map[string]FunctionDef
	// Security defines the security policy for execution.
	Security SecurityPolicy
	// SourceMaps enables source map generation for error traces.
	SourceMaps bool
	// PoolSize sets the worker pool size.
	PoolSize int
	// Logger is an optional logger for debug output.
	// If nil, no debug logging is performed.
	Logger *slog.Logger
	// BackgroundWarmup starts Bun processes in background goroutines.
	// When true, New() returns immediately and processes are warmed up
	// in the background, reducing cold start latency from ~120ms to <1ms.
	// Only applies when using the Bun engine.
	BackgroundWarmup bool
}

// DefaultConfig returns the default executor configuration.
func DefaultConfig() ExecutorConfig {
	return ExecutorConfig{
		Engine:      EngineAuto,
		Timeout:     Duration(DefaultTimeout),
		MemoryLimit: DefaultMemoryLimit,
		SourceMaps:  true,
		PoolSize:    0, // 0 means use default based on CPU count
	}
}

// Validate checks if the configuration is valid.
func (c *ExecutorConfig) Validate() error {
	if c.Timeout < 0 {
		return &ExecutionError{Message: "timeout cannot be negative"}
	}
	if c.MemoryLimit < 0 {
		return &ExecutionError{Message: "memory limit cannot be negative"}
	}
	if c.PoolSize < 0 {
		return &ExecutionError{Message: "pool size cannot be negative"}
	}
	return nil
}

// ============================================================================
// Execution Results
// ============================================================================

// Result represents the result of a script execution.
type Result struct {
	// Value is the return value from script execution.
	Value any
	// Logs contains console output from the script.
	Logs []string
	// Metrics contains execution performance metrics.
	Metrics ExecutionMetrics
	// EngineUsed indicates which engine executed the script.
	EngineUsed EngineType
}

// ExecutionMetrics contains performance metrics for an execution.
type ExecutionMetrics struct {
	// TranspileTime is the time spent transpiling TypeScript to JavaScript.
	TranspileTime time.Duration
	// ExecutionTime is the time spent executing the JavaScript.
	ExecutionTime time.Duration
	// TotalTime is the total time from start to finish.
	TotalTime time.Duration
	// CacheHit indicates if the transpiled code was served from cache.
	CacheHit bool
}

// ============================================================================
// Executor Statistics
// ============================================================================

// ExecutorStats contains runtime statistics for the executor.
type ExecutorStats struct {
	// EngineConfigured is the engine type configured (Auto, GOJA, or Bun).
	EngineConfigured EngineType
	// GOJAActive indicates if the GOJA engine is initialized.
	GOJAActive bool
	// BunActive indicates if the Bun engine is initialized.
	BunActive bool
	// TranspilerCacheSize is the current number of entries in the transpiler cache.
	TranspilerCacheSize int
	// TranspilerCacheCapacity is the maximum capacity of the transpiler cache.
	TranspilerCacheCapacity int
}

// ============================================================================
// Errors
// ============================================================================

// ExecutionError represents an error during script execution.
type ExecutionError struct {
	// Message is the error message.
	Message string
	// Code is the original source code.
	Code string
	// Line is the line number where the error occurred.
	Line int
	// Column is the column number where the error occurred.
	Column int
	// OriginalLine is the line in the original TypeScript.
	OriginalLine int
	// Stack is the JavaScript stack trace.
	Stack string
	// Cause is the underlying error, if any.
	Cause error
}

// Error implements the error interface.
func (e *ExecutionError) Error() string {
	return e.Message
}

// Unwrap returns the underlying cause for error chain support.
func (e *ExecutionError) Unwrap() error {
	return e.Cause
}
