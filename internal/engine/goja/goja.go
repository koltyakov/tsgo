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
	runtime, release, err := e.pool.acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire runtime: %w", err)
	}
	defer release()

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

	// Inject globals directly - runtime is cleared on release anyway
	for name, value := range globals {
		if err := runtime.Set(name, value); err != nil {
			return nil, fmt.Errorf("failed to set global %s: %w", name, err)
		}
	}

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
	runtime *goja.Runtime
	busy    atomic.Bool
}

func newPool(size int) *pool {
	p := &pool{
		runtimes: make([]*pooledRuntime, size),
	}
	p.cond = sync.NewCond(&p.mu)

	for i := range size {
		p.runtimes[i] = &pooledRuntime{
			runtime: createRuntime(),
		}
	}

	return p
}

func (p *pool) acquire(ctx context.Context) (*goja.Runtime, func(), error) {
	if p.closed.Load() {
		return nil, nil, ErrPoolClosed
	}

	p.mu.Lock()
	defer p.mu.Unlock()

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
				return r.runtime, func() {
					clearRuntime(r.runtime)
					r.busy.Store(false)
					// Signal waiting goroutines
					p.cond.Signal()
				}, nil
			}
		}

		// All runtimes busy - wait with timeout using sync.Cond
		// Create a done channel that the goroutine will check
		done := make(chan struct{})

		// Spawn a goroutine to wake us up on context cancellation or timeout
		go func() {
			timer := time.NewTimer(50 * time.Millisecond)
			defer timer.Stop()

			select {
			case <-ctx.Done():
				p.cond.Broadcast()
			case <-timer.C:
				p.cond.Signal()
			case <-done:
				// Parent returned, exit cleanly
				return
			}
		}()

		p.cond.Wait()
		close(done) // Signal goroutine to exit
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

// clearRuntime resets a runtime for reuse.
func clearRuntime(runtime *goja.Runtime) {
	runtime.ClearInterrupt()
}
