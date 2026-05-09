// Package goja provides a GOJA-based JavaScript execution engine.
//
// GOJA is a pure-Go JavaScript runtime that provides fast, sandboxed execution
// without requiring external dependencies. It's ideal for simple synchronous
// scripts but lacks support for async/await and certain Web APIs.
package goja

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dop251/goja"
	"github.com/koltyakov/tsgo/internal/types"
)

// ============================================================================
// Errors
// ============================================================================

// ErrPoolClosed is returned when trying to acquire from a closed pool.
var ErrPoolClosed = errors.New("goja: pool is closed")

// ============================================================================
// Configuration
// ============================================================================

// Pool size limits.
const (
	MinPoolSize     = 2
	MaxPoolSize     = 16
	DefaultPoolSize = 0 // 0 means auto-detect based on CPU count
)

// Config configures the GOJA engine.
type Config struct {
	// PoolSize sets the number of pre-warmed GOJA runtimes.
	// If <= 0, defaults to number of CPUs (capped at MaxPoolSize).
	PoolSize int
}

// ============================================================================
// Engine
// ============================================================================

// Engine executes JavaScript using the pure-Go GOJA runtime.
// It maintains a pool of pre-warmed runtimes for efficient execution.
type Engine struct {
	config Config
	pool   *pool
}

// New creates a new GOJA execution engine.
func New(cfg Config) *Engine {
	poolSize := cfg.PoolSize
	if poolSize <= 0 {
		poolSize = max(min(runtime.NumCPU(), MaxPoolSize), MinPoolSize)
	}

	return &Engine{
		config: Config{PoolSize: poolSize},
		pool:   newPool(poolSize),
	}
}

// Validate checks pre-transpile source for features GOJA cannot run
// (async/await, fetch, WebSocket, filesystem APIs, timers). Returns an
// *types.ExecutionError describing the offending features, or nil.
func (e *Engine) Validate(code string) error {
	features := DetectUnsupportedFeatures(code)
	if len(features) == 0 {
		return nil
	}
	return &types.ExecutionError{
		Message: FormatUnsupportedFeaturesError(features),
		Code:    code,
	}
}

// WantsESM always reports false: GOJA has no ESM loader and top-level
// await would be rejected by Validate up-front anyway.
func (e *Engine) WantsESM(code string) bool {
	return false
}

// Execute runs JavaScript code in a GOJA runtime.
func (e *Engine) Execute(ctx context.Context, code string, globals map[string]any) (*types.Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	start := time.Now()

	// Get runtime from pool
	pooledRT, release, err := e.pool.acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire runtime: %w", err)
	}
	defer release()

	rt := pooledRT.runtime
	rt.ClearInterrupt()

	// Set up cancellation for contexts with deadlines
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		done := make(chan struct{})
		defer close(done)

		go func() {
			select {
			case <-ctx.Done():
				rt.Interrupt("execution timeout")
			case <-done:
			}
		}()
	}

	// Inject globals (tracked for cleanup)
	if err := pooledRT.setGlobals(globals); err != nil {
		return nil, err
	}

	// Wrap code to extract default export
	wrappedCode := wrapCodeForExport(code)

	// Execute
	val, err := rt.RunString(wrappedCode)

	result := &types.Result{
		Metrics: types.ExecutionMetrics{
			ExecutionTime: time.Since(start),
			TotalTime:     time.Since(start),
		},
		EngineUsed: types.EngineGOJA,
	}

	if err != nil {
		return nil, wrapJSError(err)
	}

	result.Value = val.Export()
	return result, nil
}

// Close releases engine resources.
func (e *Engine) Close() error {
	e.pool.close()
	return nil
}

// wrapJSError converts a GOJA error to an ExecutionError.
func wrapJSError(err error) error {
	if jsErr, ok := err.(*goja.Exception); ok {
		return &types.ExecutionError{
			Message: jsErr.Error(),
			Stack:   jsErr.String(),
		}
	}
	return &types.ExecutionError{Message: err.Error()}
}

// ============================================================================
// Code Wrapping
// ============================================================================

// Wrapper templates for extracting default exports.
// Split into prefix/suffix for efficient concatenation.
const (
	// tsgoExportsWrapperPrefix/Suffix handles esbuild IIFE output with GlobalName="__tsgo_exports__"
	tsgoExportsWrapperPrefix = `(function(){var __last_result__;`
	tsgoExportsWrapperSuffix = `
function __checkAsync__(v){if(v&&typeof v.then==='function'){throw new Error('Async functions are not supported by GOJA engine. Use Bun engine instead.')}return v}
if(typeof __tsgo_exports__!=='undefined'&&__tsgo_exports__!==null){if(__tsgo_exports__&&typeof __tsgo_exports__.default!=='undefined'){var __d__=__tsgo_exports__.default;if(typeof __d__==='function')return __checkAsync__(__d__());return __d__}if(Object.keys(__tsgo_exports__).length>0)return __tsgo_exports__}return __last_result__})()`

	// moduleExportsWrapperPrefix/Suffix handles CommonJS-style exports
	moduleExportsWrapperPrefix = `(function(){var exports={},module={exports:exports},__default__,__last_result__;`
	moduleExportsWrapperSuffix = `
function __checkAsync__(v){if(v&&typeof v.then==='function'){throw new Error('Async functions are not supported by GOJA engine. Use Bun engine instead.')}return v}
function __i__(v){return typeof v==='function'?__checkAsync__(v()):v}if(typeof __default__!=='undefined')return __i__(__default__);if(typeof module.exports.default!=='undefined')return __i__(module.exports.default);if(typeof exports.default!=='undefined')return __i__(exports.default);if(Object.keys(module.exports).length>0)return module.exports;return __last_result__})()`
)

// wrapCodeForExport wraps code to extract the default export.
// Optimized to minimize string allocations.
func wrapCodeForExport(code string) string {
	// Fast path: check code length to avoid TrimSpace on large code
	if len(code) == 0 {
		return code
	}

	// Find first and last non-whitespace for trimming check
	start := 0
	for start < len(code) && (code[start] == ' ' || code[start] == '\t' || code[start] == '\n' || code[start] == '\r') {
		start++
	}
	if start == len(code) {
		return code
	}
	end := len(code) - 1
	for end > start && (code[end] == ' ' || code[end] == '\t' || code[end] == '\n' || code[end] == '\r') {
		end--
	}
	trimmed := code[start : end+1]

	// Quick checks using first/last byte
	firstByte := trimmed[0]
	lastByte := trimmed[len(trimmed)-1]

	// If code is already an IIFE, return as-is
	if firstByte == '(' {
		if len(trimmed) > 12 && trimmed[1] == 'f' && strings.HasPrefix(trimmed, "(function") && strings.HasSuffix(trimmed, ")()") {
			return code
		}
		if lastByte == ')' {
			return code
		}
	}

	// Check if it's a simple expression (no statements)
	// Use IndexByte which is assembly-optimized
	if strings.IndexByte(trimmed, ';') == -1 && strings.IndexByte(trimmed, '\n') == -1 {
		return "(" + trimmed + ")"
	}

	// For esbuild IIFE output with GlobalName="__tsgo_exports__"
	// Check with IndexByte first for fast rejection
	if strings.IndexByte(code, '_') != -1 &&
		(strings.Contains(code, "var __tsgo_exports__") || strings.Contains(code, "__tsgo_exports__=")) {
		return tsgoExportsWrapperPrefix + code + tsgoExportsWrapperSuffix
	}

	// For code that goes through transpiler but doesn't use exports
	return moduleExportsWrapperPrefix + code + moduleExportsWrapperSuffix
}

// ============================================================================
// Runtime Pool
// ============================================================================

// pool manages pre-warmed GOJA runtimes.
type pool struct {
	runtimes  []*pooledRuntime
	available chan *pooledRuntime
	closed    atomic.Bool
}

// pooledRuntime wraps a GOJA runtime with tracking for context isolation.
type pooledRuntime struct {
	runtime      *goja.Runtime
	busy         atomic.Bool
	baseGlobals  map[string]struct{} // Globals set during createRuntime (never cleared)
	userGlobals  []string            // Globals set during execution (cleared on release)
	userGlobalMu sync.Mutex
}

// setGlobals injects globals into the runtime and tracks them for cleanup.
func (pr *pooledRuntime) setGlobals(globals map[string]any) error {
	pr.userGlobalMu.Lock()
	defer pr.userGlobalMu.Unlock()

	pr.userGlobals = pr.userGlobals[:0] // Reset slice, keep capacity
	for name, value := range globals {
		if err := pr.runtime.Set(name, value); err != nil {
			return fmt.Errorf("failed to set global %s: %w", name, err)
		}
		pr.userGlobals = append(pr.userGlobals, name)
	}
	return nil
}

func newPool(size int) *pool {
	p := &pool{
		runtimes:  make([]*pooledRuntime, size),
		available: make(chan *pooledRuntime, size),
	}

	for i := range size {
		rt := createRuntime()
		// Capture the base globals that should never be cleared
		baseGlobals := captureGlobalNames(rt)
		p.runtimes[i] = &pooledRuntime{
			runtime:     rt,
			baseGlobals: baseGlobals,
			userGlobals: make([]string, 0, 16), // pre-allocate for typical usage
		}
		p.available <- p.runtimes[i]
	}

	return p
}

func (p *pool) acquire(ctx context.Context) (*pooledRuntime, func(), error) {
	if p.closed.Load() {
		return nil, nil, ErrPoolClosed
	}

	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case r, ok := <-p.available:
		if !ok || p.closed.Load() {
			return nil, nil, ErrPoolClosed
		}
		if r.busy.CompareAndSwap(false, true) {
			return r, p.makeReleaseFunc(r), nil
		}
		// In case of unexpected busy state, retry once
		return p.acquire(ctx)
	}
}

// makeReleaseFunc creates a release function for a pooled runtime.
func (p *pool) makeReleaseFunc(r *pooledRuntime) func() {
	return func() {
		clearRuntime(r)
		r.busy.Store(false)
		if p.closed.Load() {
			return
		}
		select {
		case p.available <- r:
		default:
		}
	}
}

func (p *pool) close() {
	if !p.closed.CompareAndSwap(false, true) {
		return
	}
	close(p.available)
}

// ============================================================================
// Runtime Factory
// ============================================================================

// Globals that don't exist in GOJA and should be set to undefined.
var undefinedGlobals = []string{
	"fetch", "XMLHttpRequest", "WebSocket",
	"Worker", "SharedWorker", "ServiceWorker",
	"importScripts", "Deno", "Bun", "process", "require",
	"__dirname", "__filename",
}

// createRuntime creates a GOJA runtime with console support.
func createRuntime() *goja.Runtime {
	rt := goja.New()

	// Set up field name mapper for JSON tags
	rt.SetFieldNameMapper(goja.TagFieldNameMapper("json", true))

	// Set up console (no-op implementations)
	console := rt.NewObject()
	noopFn := func(call goja.FunctionCall) goja.Value { return goja.Undefined() }
	_ = console.Set("log", noopFn)
	_ = console.Set("error", noopFn)
	_ = console.Set("warn", noopFn)
	_ = console.Set("info", noopFn)
	_ = rt.Set("console", console)

	// Set undefined for globals that don't exist in GOJA
	for _, name := range undefinedGlobals {
		_ = rt.Set(name, goja.Undefined())
	}

	return rt
}

// captureGlobalNames returns a set of all global property names in the runtime.
// Used to identify base globals that should never be cleared.
func captureGlobalNames(rt *goja.Runtime) map[string]struct{} {
	globals := make(map[string]struct{})
	globalObj := rt.GlobalObject()
	for _, key := range globalObj.Keys() {
		globals[key] = struct{}{}
	}
	return globals
}

// ============================================================================
// Runtime Cleanup
// ============================================================================

// clearRuntime resets a pooled runtime for reuse, ensuring context isolation.
// It removes all user-injected globals and any globals created during execution.
func clearRuntime(pr *pooledRuntime) {
	pr.runtime.ClearInterrupt()

	// Clear explicitly tracked user globals (fast path)
	pr.userGlobalMu.Lock()
	userGlobals := pr.userGlobals
	pr.userGlobalMu.Unlock()

	cleared := make(map[string]struct{}, len(userGlobals))
	for _, name := range userGlobals {
		_ = pr.runtime.Set(name, goja.Undefined())
		cleared[name] = struct{}{}
	}

	// Scan for any globals created during execution (e.g., via globalThis.foo = ...)
	globalObj := pr.runtime.GlobalObject()
	for _, key := range globalObj.Keys() {
		if _, done := cleared[key]; done {
			continue
		}
		if _, isBase := pr.baseGlobals[key]; !isBase {
			_ = pr.runtime.Set(key, goja.Undefined())
		}
	}
}
