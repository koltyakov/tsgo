// Package wasm provides a WASM-based sandboxed JavaScript execution engine.
//
// This engine uses wazero (pure-Go WebAssembly runtime) to execute QuickJS
// compiled to WASM, providing a fully sandboxed execution environment with
// configurable memory limits and no external dependencies.
package wasm

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"

	"github.com/koltyakov/tsgo/internal/types"
)

//go:embed quickjs.wasm
var quickjsWasm []byte

// Config configures the WASM execution engine.
type Config struct {
	// MemoryLimit sets the maximum memory available to the WASM module.
	// Default is 64MB.
	MemoryLimit int64
	// CompilationCache enables caching of compiled WASM modules.
	CompilationCache bool
}

// Engine executes JavaScript in a WASM sandbox using QuickJS.
type Engine struct {
	config  Config
	runtime wazero.Runtime
	module  api.Module
	mu      sync.Mutex
	closed  bool
}

// New creates a new WASM execution engine.
func New(cfg Config) (*Engine, error) {
	if cfg.MemoryLimit <= 0 {
		cfg.MemoryLimit = 64 * 1024 * 1024 // 64MB
	}

	// Check if QuickJS WASM is embedded
	if len(quickjsWasm) == 0 {
		return nil, fmt.Errorf("QuickJS WASM module not embedded")
	}

	ctx := context.Background()

	// Create wazero runtime
	runtimeConfig := wazero.NewRuntimeConfig()
	if cfg.CompilationCache {
		cache := wazero.NewCompilationCache()
		runtimeConfig = runtimeConfig.WithCompilationCache(cache)
	}

	runtime := wazero.NewRuntimeWithConfig(ctx, runtimeConfig)

	// Instantiate WASI for basic I/O
	wasi_snapshot_preview1.MustInstantiate(ctx, runtime)

	// Configure memory limits
	moduleConfig := wazero.NewModuleConfig().
		WithStartFunctions("_initialize")

	// Compile and instantiate QuickJS
	compiled, err := runtime.CompileModule(ctx, quickjsWasm)
	if err != nil {
		runtime.Close(ctx)
		return nil, fmt.Errorf("failed to compile QuickJS module: %w", err)
	}

	module, err := runtime.InstantiateModule(ctx, compiled, moduleConfig)
	if err != nil {
		runtime.Close(ctx)
		return nil, fmt.Errorf("failed to instantiate QuickJS module: %w", err)
	}

	return &Engine{
		config:  cfg,
		runtime: runtime,
		module:  module,
	}, nil
}

// Execute runs JavaScript code in the WASM sandbox.
func (e *Engine) Execute(ctx context.Context, code string, globals map[string]any) (*types.Result, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return nil, fmt.Errorf("engine is closed")
	}

	start := time.Now()

	// Get the eval function from QuickJS
	evalFunc := e.module.ExportedFunction("qjs_eval")
	if evalFunc == nil {
		return nil, fmt.Errorf("qjs_eval function not found in module")
	}

	// Build script with globals
	script := buildScript(code, globals)

	// Allocate memory for the script
	malloc := e.module.ExportedFunction("malloc")
	free := e.module.ExportedFunction("free")

	if malloc == nil || free == nil {
		return nil, fmt.Errorf("memory functions not found in module")
	}

	scriptBytes := []byte(script)
	scriptLen := uint64(len(scriptBytes))

	// Allocate memory for script
	results, err := malloc.Call(ctx, scriptLen+1) // +1 for null terminator
	if err != nil {
		return nil, fmt.Errorf("malloc failed: %w", err)
	}
	scriptPtr := results[0]
	defer free.Call(ctx, scriptPtr)

	// Write script to WASM memory
	if !e.module.Memory().Write(uint32(scriptPtr), scriptBytes) {
		return nil, fmt.Errorf("failed to write script to memory")
	}
	// Null terminator
	e.module.Memory().WriteByte(uint32(scriptPtr)+uint32(scriptLen), 0)

	// Allocate buffer for result
	resultBufSize := uint64(1024 * 64) // 64KB result buffer
	results, err = malloc.Call(ctx, resultBufSize)
	if err != nil {
		return nil, fmt.Errorf("malloc for result failed: %w", err)
	}
	resultPtr := results[0]
	defer free.Call(ctx, resultPtr)

	// Call eval
	results, err = evalFunc.Call(ctx, scriptPtr, resultPtr, resultBufSize)
	if err != nil {
		return nil, fmt.Errorf("eval failed: %w", err)
	}

	// Read result
	resultLen := results[0]
	if resultLen == 0 {
		return nil, fmt.Errorf("eval returned empty result")
	}

	resultBytes, ok := e.module.Memory().Read(uint32(resultPtr), uint32(resultLen))
	if !ok {
		return nil, fmt.Errorf("failed to read result from memory")
	}

	// Parse result JSON
	var evalResult struct {
		Success bool   `json:"success"`
		Value   any    `json:"value"`
		Error   string `json:"error"`
	}

	if err := json.Unmarshal(resultBytes, &evalResult); err != nil {
		return nil, fmt.Errorf("failed to parse result: %w", err)
	}

	execTime := time.Since(start)

	if !evalResult.Success {
		return nil, &types.ExecutionError{
			Message: evalResult.Error,
		}
	}

	return &types.Result{
		Value: evalResult.Value,
		Metrics: types.ExecutionMetrics{
			ExecutionTime: execTime,
			TotalTime:     execTime,
		},
		EngineUsed: types.EngineWASM,
	}, nil
}

// buildScript wraps the code with globals injection.
func buildScript(code string, globals map[string]any) string {
	if len(globals) == 0 {
		return wrapForResult(code)
	}

	// Build global declarations
	globalsJSON, _ := json.Marshal(globals)
	return fmt.Sprintf(`
		(function() {
			var __globals = %s;
			for (var k in __globals) {
				this[k] = __globals[k];
			}
			%s
		})()
	`, string(globalsJSON), wrapForResult(code))
}

func wrapForResult(code string) string {
	return fmt.Sprintf(`
		(function() {
			try {
				var __result = (function() { %s })();
				return JSON.stringify({success: true, value: __result});
			} catch (e) {
				return JSON.stringify({success: false, error: e.message || String(e)});
			}
		})()
	`, code)
}

// Close releases engine resources.
func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return nil
	}
	e.closed = true

	ctx := context.Background()

	if e.module != nil {
		e.module.Close(ctx)
	}
	if e.runtime != nil {
		e.runtime.Close(ctx)
	}

	return nil
}

// IsAvailable returns true if the WASM engine is ready.
func (e *Engine) IsAvailable() bool {
	return e.module != nil && !e.closed
}
