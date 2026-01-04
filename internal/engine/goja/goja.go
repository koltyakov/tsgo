// Package goja provides a GOJA-based JavaScript execution engine.
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

// Sentinel errors for the GOJA engine.
var (
	// ErrPoolClosed is returned when trying to acquire from a closed pool.
	ErrPoolClosed = errors.New("goja: pool is closed")
)

// Config configures the GOJA engine.
type Config struct {
	// PoolSize sets the number of pre-warmed GOJA runtimes.
	PoolSize int
}

// Engine executes JavaScript using the pure-Go GOJA runtime.
type Engine struct {
	config Config
	pool   *pool
}

// New creates a new GOJA execution engine.
func New(cfg Config) *Engine {
	if cfg.PoolSize <= 0 {
		// Default to number of CPUs, capped at 16
		cfg.PoolSize = runtime.NumCPU()
		if cfg.PoolSize > 16 {
			cfg.PoolSize = 16
		}
		if cfg.PoolSize < 2 {
			cfg.PoolSize = 2
		}
	}

	return &Engine{
		config: cfg,
		pool:   newPool(cfg.PoolSize),
	}
}

// Execute runs JavaScript code in a GOJA runtime.
func (e *Engine) Execute(ctx context.Context, code string, globals map[string]any) (*types.Result, error) {
	// Check context before expensive operations
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

	runtime := pooledRT.runtime

	// Only spawn cancellation goroutine if context has a deadline
	// This avoids goroutine overhead for simple executions
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		done := make(chan struct{})
		defer close(done)

		go func() {
			select {
			case <-ctx.Done():
				runtime.Interrupt("execution timeout")
			case <-done:
			}
		}()
	}

	// Inject globals and track them for cleanup (context isolation)
	pooledRT.userGlobalMu.Lock()
	pooledRT.userGlobals = pooledRT.userGlobals[:0] // reset slice, keep capacity
	for name, value := range globals {
		if err := runtime.Set(name, value); err != nil {
			pooledRT.userGlobalMu.Unlock()
			return nil, fmt.Errorf("failed to set global %s: %w", name, err)
		}
		pooledRT.userGlobals = append(pooledRT.userGlobals, name)
	}
	pooledRT.userGlobalMu.Unlock()

	// Wrap code to extract default export
	wrappedCode := wrapCodeForExport(code)

	// Execute
	val, err := runtime.RunString(wrappedCode)

	result := &types.Result{
		Metrics: types.ExecutionMetrics{
			ExecutionTime: time.Since(start),
			TotalTime:     time.Since(start),
		},
		EngineUsed: types.EngineGOJA,
	}

	if err != nil {
		if jsErr, ok := err.(*goja.Exception); ok {
			return nil, &types.ExecutionError{
				Message: jsErr.Error(),
				Stack:   jsErr.String(),
			}
		}
		return nil, &types.ExecutionError{
			Message: err.Error(),
		}
	}

	// Export value to Go
	result.Value = val.Export()
	return result, nil
}

// Close releases engine resources.
func (e *Engine) Close() error {
	e.pool.close()
	return nil
}

// Pre-defined wrapper templates to avoid repeated string allocations
const (
	// tsgoExportsWrapper handles esbuild IIFE output with GlobalName="__tsgo_exports__"
	// Detects async function results and throws an error since GOJA can't resolve promises
	tsgoExportsWrapper = `(function(){var __last_result__;%s
function __checkAsync__(v){if(v&&typeof v.then==='function'){throw new Error('Async functions are not supported by GOJA engine. Use Bun engine instead.')}return v}
if(typeof __tsgo_exports__!=='undefined'&&__tsgo_exports__!==null){if(__tsgo_exports__&&typeof __tsgo_exports__.default!=='undefined'){var __d__=__tsgo_exports__.default;if(typeof __d__==='function')return __checkAsync__(__d__());return __d__}if(Object.keys(__tsgo_exports__).length>0)return __tsgo_exports__}return __last_result__})()`

	// moduleExportsWrapper handles CommonJS-style exports
	// Detects async function results and throws an error since GOJA can't resolve promises
	moduleExportsWrapper = `(function(){var exports={},module={exports:exports},__default__,__last_result__;%s
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
		return fmt.Sprintf(tsgoExportsWrapper, code)
	}

	// For code that goes through transpiler but doesn't use exports
	return fmt.Sprintf(moduleExportsWrapper, code)
}

// pool manages pre-warmed GOJA runtimes.
type pool struct {
	runtimes []*pooledRuntime
	mu       sync.Mutex
	cond     *sync.Cond
	closed   atomic.Bool
}

type pooledRuntime struct {
	runtime      *goja.Runtime
	busy         atomic.Bool
	baseGlobals  map[string]struct{} // globals set during createRuntime (never cleared)
	userGlobals  []string            // globals set during execution (cleared on release)
	userGlobalMu sync.Mutex          // protects userGlobals
}

func newPool(size int) *pool {
	p := &pool{
		runtimes: make([]*pooledRuntime, size),
	}
	p.cond = sync.NewCond(&p.mu)

	for i := range size {
		rt := createRuntime()
		// Capture the base globals that should never be cleared
		baseGlobals := captureGlobalNames(rt)
		p.runtimes[i] = &pooledRuntime{
			runtime:     rt,
			baseGlobals: baseGlobals,
			userGlobals: make([]string, 0, 16), // pre-allocate for typical usage
		}
	}

	return p
}

func (p *pool) acquire(ctx context.Context) (*pooledRuntime, func(), error) {
	if p.closed.Load() {
		return nil, nil, ErrPoolClosed
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Set up a timer for periodic wake-ups (reused across iterations)
	timer := time.NewTimer(50 * time.Millisecond)
	defer timer.Stop()

	// Channel for context cancellation signaling
	ctxDone := ctx.Done()

	// Try to find a free runtime with timeout-based retry
	for {
		// Check context first
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}

		// Check if pool was closed while waiting
		if p.closed.Load() {
			return nil, nil, ErrPoolClosed
		}

		// Try to find a free runtime
		for _, r := range p.runtimes {
			if r.busy.CompareAndSwap(false, true) {
				return r, func() {
					clearRuntime(r)
					r.busy.Store(false)
					// Signal waiting goroutines
					p.cond.Signal()
				}, nil
			}
		}

		// All runtimes busy - wait with timeout using sync.Cond
		// Use a single goroutine to handle both context cancellation and timeout
		done := make(chan struct{})

		go func() {
			select {
			case <-ctxDone:
				p.cond.Broadcast()
			case <-timer.C:
				p.cond.Signal()
			case <-done:
				return
			}
		}()

		p.cond.Wait()
		close(done)

		// Reset timer for next iteration
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(50 * time.Millisecond)
	}
}

func (p *pool) close() {
	if !p.closed.CompareAndSwap(false, true) {
		return // Already closed
	}

	// Wake up any waiting goroutines
	p.cond.Broadcast()
}

// createRuntime creates a GOJA runtime with console support.
func createRuntime() *goja.Runtime {
	runtime := goja.New()

	// Set up field name mapper
	runtime.SetFieldNameMapper(goja.TagFieldNameMapper("json", true))

	// Set up console
	console := runtime.NewObject()
	console.Set("log", func(call goja.FunctionCall) goja.Value {
		return goja.Undefined()
	})
	console.Set("error", func(call goja.FunctionCall) goja.Value {
		return goja.Undefined()
	})
	console.Set("warn", func(call goja.FunctionCall) goja.Value {
		return goja.Undefined()
	})
	console.Set("info", func(call goja.FunctionCall) goja.Value {
		return goja.Undefined()
	})
	runtime.Set("console", console)

	// Set undefined for globals that don't exist in GOJA
	undefinedGlobals := []string{
		"fetch", "XMLHttpRequest", "WebSocket",
		"Worker", "SharedWorker", "ServiceWorker",
		"importScripts", "Deno", "Bun", "process", "require",
		"__dirname", "__filename",
	}
	for _, name := range undefinedGlobals {
		runtime.Set(name, goja.Undefined())
	}

	return runtime
}

// captureGlobalNames returns a set of all global property names in the runtime.
// This is used to identify base globals that should never be cleared.
func captureGlobalNames(runtime *goja.Runtime) map[string]struct{} {
	globals := make(map[string]struct{})
	globalObj := runtime.GlobalObject()
	for _, key := range globalObj.Keys() {
		globals[key] = struct{}{}
	}
	return globals
}

// clearRuntime resets a pooled runtime for reuse, ensuring context isolation.
// It removes all user-injected globals and any globals created during execution.
func clearRuntime(pr *pooledRuntime) {
	pr.runtime.ClearInterrupt()

	// Clear explicitly tracked user globals first (fast path)
	pr.userGlobalMu.Lock()
	userGlobals := pr.userGlobals
	pr.userGlobalMu.Unlock()

	// Use a set to track what we've already cleared
	cleared := make(map[string]struct{}, len(userGlobals))
	for _, name := range userGlobals {
		pr.runtime.Set(name, goja.Undefined())
		cleared[name] = struct{}{}
	}

	// Scan for any globals created during execution (e.g., via globalThis.foo = ...)
	// and remove them if they weren't part of the base runtime
	globalObj := pr.runtime.GlobalObject()
	for _, key := range globalObj.Keys() {
		// Skip if already cleared or is a base global
		if _, done := cleared[key]; done {
			continue
		}
		if _, isBase := pr.baseGlobals[key]; !isBase {
			pr.runtime.Set(key, goja.Undefined())
		}
	}
}
