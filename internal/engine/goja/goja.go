// Package goja provides a GOJA-based JavaScript execution engine.
package goja

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/koltyakov/tsgo/internal/types"
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
		cfg.PoolSize = 8
	}

	return &Engine{
		config: cfg,
		pool:   newPool(cfg.PoolSize),
	}
}

// Execute runs JavaScript code in a GOJA runtime.
func (e *Engine) Execute(ctx context.Context, code string, globals map[string]any) (*types.Result, error) {
	start := time.Now()

	// Get runtime from pool
	runtime, release, err := e.pool.acquire()
	if err != nil {
		return nil, fmt.Errorf("failed to acquire runtime: %w", err)
	}
	defer release()

	// Set up context cancellation
	done := make(chan struct{})
	defer close(done)

	go func() {
		select {
		case <-ctx.Done():
			runtime.Interrupt("execution timeout")
		case <-done:
		}
	}()

	// Inject globals
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

// wrapCodeForExport wraps code to extract the default export.
func wrapCodeForExport(code string) string {
	trimmed := strings.TrimSpace(code)

	// If code is already an IIFE or simple expression, return as-is
	if strings.HasPrefix(trimmed, "(function") && strings.HasSuffix(trimmed, ")()") {
		return code
	}
	if strings.HasPrefix(trimmed, "(") && strings.HasSuffix(trimmed, ")") {
		return code
	}

	// Check if it's a simple expression (no statements)
	if !strings.Contains(trimmed, ";") && !strings.Contains(trimmed, "\n") {
		return fmt.Sprintf("(%s)", trimmed)
	}

	// For esbuild IIFE output with GlobalName="__tsgo_exports__"
	// The IIFE assigns the result of the inner function to __tsgo_exports__
	// If the inner function has exports, they'll be in __tsgo_exports__
	// If not, __tsgo_exports__ will be undefined
	if strings.Contains(trimmed, "var __tsgo_exports__") || strings.Contains(trimmed, "__tsgo_exports__=") {
		// esbuild wraps code as: var __tsgo_exports__ = (() => { ...code... })();
		// For code without exports, this returns undefined
		// We need to extract the last expression value
		return fmt.Sprintf(`
(function() {
	var __last_result__;
	%s
	if (typeof __tsgo_exports__ !== 'undefined' && __tsgo_exports__ !== null) {
		if (__tsgo_exports__ && typeof __tsgo_exports__.default !== 'undefined') {
			var __default__ = __tsgo_exports__.default;
			if (typeof __default__ === 'function') {
				return __default__();
			}
			return __default__;
		}
		// Check if __tsgo_exports__ has any own properties
		if (Object.keys(__tsgo_exports__).length > 0) {
			return __tsgo_exports__;
		}
	}
	return __last_result__;
})()
`, code)
	}

	// For code that goes through transpiler but doesn't use exports
	// Try to capture the last expression
	return fmt.Sprintf(`
(function() {
	var exports = {};
	var module = { exports: exports };
	var __default__;
	var __last_result__;
	
	%s
	
	function __invoke_if_fn__(val) {
		if (typeof val === 'function') {
			return val();
		}
		return val;
	}
	
	if (typeof __default__ !== 'undefined') {
		return __invoke_if_fn__(__default__);
	}
	if (typeof module.exports.default !== 'undefined') {
		return __invoke_if_fn__(module.exports.default);
	}
	if (typeof exports.default !== 'undefined') {
		return __invoke_if_fn__(exports.default);
	}
	if (Object.keys(module.exports).length > 0) {
		return module.exports;
	}
	return __last_result__;
})()
`, code)
}

// pool manages pre-warmed GOJA runtimes.
type pool struct {
	runtimes []*pooledRuntime
	mu       sync.Mutex
	closed   bool
}

type pooledRuntime struct {
	runtime *goja.Runtime
	busy    bool
	mu      sync.Mutex
}

func newPool(size int) *pool {
	p := &pool{
		runtimes: make([]*pooledRuntime, size),
	}

	for i := 0; i < size; i++ {
		p.runtimes[i] = &pooledRuntime{
			runtime: createRuntime(),
			busy:    false,
		}
	}

	return p
}

func (p *pool) acquire() (*goja.Runtime, func(), error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, nil, fmt.Errorf("pool is closed")
	}

	// Find a free runtime
	for _, r := range p.runtimes {
		r.mu.Lock()
		if !r.busy {
			r.busy = true
			r.mu.Unlock()
			p.mu.Unlock()

			return r.runtime, func() {
				r.mu.Lock()
				clearRuntime(r.runtime)
				r.busy = false
				r.mu.Unlock()
			}, nil
		}
		r.mu.Unlock()
	}
	p.mu.Unlock()

	// All runtimes busy, create a temporary one
	runtime := createRuntime()
	return runtime, func() {}, nil
}

func (p *pool) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
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
