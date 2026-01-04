package goja

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	engine := New(Config{PoolSize: 2})
	if engine == nil {
		t.Fatal("expected engine to be created")
	}
	defer func() { _ = engine.Close() }()
}

func TestExecute_SimpleExpression(t *testing.T) {
	engine := New(Config{PoolSize: 2})
	defer func() { _ = engine.Close() }()

	ctx := context.Background()
	result, err := engine.Execute(ctx, "1 + 2", nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}

	// Check value is 3
	switch v := result.Value.(type) {
	case int64:
		if v != 3 {
			t.Errorf("expected 3, got %d", v)
		}
	case float64:
		if v != 3 {
			t.Errorf("expected 3, got %f", v)
		}
	default:
		t.Errorf("unexpected value type: %T", result.Value)
	}
}

func TestExecute_WithGlobals(t *testing.T) {
	engine := New(Config{PoolSize: 2})
	defer func() { _ = engine.Close() }()

	ctx := context.Background()
	globals := map[string]any{
		"multiplier": 10,
	}

	result, err := engine.Execute(ctx, "5 * multiplier", globals)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	switch v := result.Value.(type) {
	case int64:
		if v != 50 {
			t.Errorf("expected 50, got %d", v)
		}
	case float64:
		if v != 50 {
			t.Errorf("expected 50, got %f", v)
		}
	}
}

func TestExecute_StringResult(t *testing.T) {
	engine := New(Config{PoolSize: 2})
	defer func() { _ = engine.Close() }()

	ctx := context.Background()
	result, err := engine.Execute(ctx, `"hello" + " " + "world"`, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Value != "hello world" {
		t.Errorf("expected 'hello world', got %v", result.Value)
	}
}

func TestExecute_FunctionExecution(t *testing.T) {
	engine := New(Config{PoolSize: 2})
	defer func() { _ = engine.Close() }()

	ctx := context.Background()
	code := `
		var add = function(a, b) { return a + b; };
		add(3, 4);
	`
	result, err := engine.Execute(ctx, code, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	switch v := result.Value.(type) {
	case int64:
		if v != 7 {
			t.Errorf("expected 7, got %d", v)
		}
	case float64:
		if v != 7 {
			t.Errorf("expected 7, got %f", v)
		}
	}
}

func TestExecute_ContextCancellation(t *testing.T) {
	engine := New(Config{PoolSize: 2})
	defer func() { _ = engine.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Infinite loop should be interrupted
	code := `
		var i = 0;
		while(true) { i++; }
		i;
	`
	_, err := engine.Execute(ctx, code, nil)

	if err == nil {
		t.Error("expected error from infinite loop")
	}
}

func TestExecute_SyntaxError(t *testing.T) {
	engine := New(Config{PoolSize: 2})
	defer func() { _ = engine.Close() }()

	ctx := context.Background()
	_, err := engine.Execute(ctx, "function {", nil)

	if err == nil {
		t.Error("expected syntax error")
	}
}

func TestWrapCodeForExport(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		contains string
	}{
		{
			name:     "simple expression",
			code:     "1 + 2",
			contains: "(1 + 2)",
		},
		{
			name:     "tsgo exports wrapper",
			code:     "var __tsgo_exports__ = { default: 42 };",
			contains: "__tsgo_exports__",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped := wrapCodeForExport(tt.code)
			if !strings.Contains(wrapped, tt.contains) {
				t.Errorf("expected wrapped code to contain %q, got: %s", tt.contains, wrapped)
			}
		})
	}
}

func TestPoolAcquireRelease(t *testing.T) {
	p := newPool(2)
	defer p.close()

	ctx := context.Background()

	// Acquire all runtimes
	r1, release1, err := p.acquire(ctx)
	if err != nil {
		t.Fatalf("failed to acquire: %v", err)
	}
	if r1 == nil {
		t.Fatal("expected runtime")
	}

	r2, release2, err := p.acquire(ctx)
	if err != nil {
		t.Fatalf("failed to acquire: %v", err)
	}
	if r2 == nil {
		t.Fatal("expected runtime")
	}

	// Release all (no temporary runtime creation in new pool design)
	release2()
	release1()
}

func TestPoolClosed(t *testing.T) {
	p := newPool(2)
	p.close()

	ctx := context.Background()
	_, _, err := p.acquire(ctx)
	if err == nil {
		t.Error("expected error from closed pool")
	}
}

func TestExecute_ComparisonExpression(t *testing.T) {
	engine := New(Config{PoolSize: 2})
	defer func() { _ = engine.Close() }()

	// Note: These tests test the raw GOJA engine without transpilation.
	// Multi-statement code with trailing expressions requires preprocessing
	// by the transpiler, so we only test simple expressions here.
	tests := []struct {
		name     string
		code     string
		expected bool
	}{
		{"strict equality false", "1 === 2", false},
		{"strict equality true", "2 === 2", true},
		{"strict inequality", "1 !== 2", true},
		{"less than", "1 < 2", true},
		{"greater than", "1 > 2", false},
		{"logical and", "true && false", false},
		{"logical or", "true || false", true},
	}

	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.Execute(ctx, tt.code, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			got, ok := result.Value.(bool)
			if !ok {
				t.Fatalf("expected boolean result, got %T: %v", result.Value, result.Value)
			}

			if got != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestAsyncFunctionExport(t *testing.T) {
	engine := New(Config{PoolSize: 2})
	defer func() { _ = engine.Close() }()

	// Test that async functions return a clear error
	// GOJA can't resolve promises (no event loop), so we return an error
	// instead of silently returning undefined/empty object

	ctx := context.Background()

	// This is what esbuild produces for async functions (simplified)
	// The actual transpiled code uses __async helper function
	transpilerCode := `
var __tsgo_exports__ = (() => {
  var __defProp = Object.defineProperty;
  var __export = (target, all) => {
    for (var name in all)
      __defProp(target, name, { get: all[name], enumerable: true });
  };
  var stdin_exports = {};
  __export(stdin_exports, {
    default: () => stdin_default
  });
  var stdin_default = async () => {
    return "abc";
  };
  return stdin_exports;
})();
`

	_, err := engine.Execute(ctx, transpilerCode, nil)
	if err == nil {
		t.Fatal("expected error for async function, got nil")
	}

	// Should return a clear error message about async not being supported
	if !strings.Contains(err.Error(), "Async functions are not supported") {
		t.Errorf("expected async not supported error, got: %v", err)
	}
}

func TestContextIsolation(t *testing.T) {
	// This test verifies that context does not leak between executions.
	// Critical for BPMN engines where each process must be isolated.
	engine := New(Config{PoolSize: 1}) // Single runtime to guarantee reuse
	defer func() { _ = engine.Close() }()

	ctx := context.Background()

	t.Run("globalThis pollution", func(t *testing.T) {
		// First execution: set a secret on globalThis
		_, err := engine.Execute(ctx, "globalThis.secretValue = 'LEAKED'", nil)
		if err != nil {
			t.Fatalf("first execution failed: %v", err)
		}

		// Second execution: verify the secret is NOT accessible
		result, err := engine.Execute(ctx, "typeof globalThis.secretValue !== 'undefined' ? 'LEAK' : 'CLEAN'", nil)
		if err != nil {
			t.Fatalf("second execution failed: %v", err)
		}
		if result.Value != "CLEAN" {
			t.Errorf("context leak detected via globalThis: %v", result.Value)
		}
	})

	t.Run("var declaration", func(t *testing.T) {
		// var declarations in the wrapped code should be cleaned up
		_, err := engine.Execute(ctx, "globalThis.leakedVar = 'SECRET'; 'done'", nil)
		if err != nil {
			t.Fatalf("first execution failed: %v", err)
		}

		result, err := engine.Execute(ctx, "typeof leakedVar !== 'undefined' ? 'LEAK' : 'CLEAN'", nil)
		if err != nil {
			t.Fatalf("second execution failed: %v", err)
		}
		if result.Value != "CLEAN" {
			t.Errorf("context leak detected via var: %v", result.Value)
		}
	})

	t.Run("injected globals cleaned", func(t *testing.T) {
		// Execute with an injected global
		globals := map[string]any{"injectedSecret": "SECRET123"}
		_, err := engine.Execute(ctx, "injectedSecret", globals)
		if err != nil {
			t.Fatalf("first execution failed: %v", err)
		}

		// Execute without the global - should not be accessible
		result, err := engine.Execute(ctx, "typeof injectedSecret !== 'undefined' ? 'LEAK' : 'CLEAN'", nil)
		if err != nil {
			t.Fatalf("second execution failed: %v", err)
		}
		if result.Value != "CLEAN" {
			t.Errorf("injected global leaked between executions: %v", result.Value)
		}
	})

	t.Run("function pollution", func(t *testing.T) {
		// Define a function on globalThis
		_, err := engine.Execute(ctx, "globalThis.leakedFunc = function() { return 'SECRET' }", nil)
		if err != nil {
			t.Fatalf("first execution failed: %v", err)
		}

		// Verify function is not accessible
		result, err := engine.Execute(ctx, "typeof leakedFunc !== 'undefined' ? 'LEAK' : 'CLEAN'", nil)
		if err != nil {
			t.Fatalf("second execution failed: %v", err)
		}
		if result.Value != "CLEAN" {
			t.Errorf("function leaked between executions: %v", result.Value)
		}
	})

	t.Run("sequential isolation", func(t *testing.T) {
		// Multiple sequential executions should all be isolated
		for i := 0; i < 5; i++ {
			code := "globalThis.seqTest" + string(rune('0'+i)) + " = " + string(rune('0'+i))
			_, err := engine.Execute(ctx, code, nil)
			if err != nil {
				t.Fatalf("execution %d failed: %v", i, err)
			}
		}

		// None should be accessible
		result, err := engine.Execute(ctx, "typeof seqTest0 !== 'undefined' || typeof seqTest4 !== 'undefined' ? 'LEAK' : 'CLEAN'", nil)
		if err != nil {
			t.Fatalf("check execution failed: %v", err)
		}
		if result.Value != "CLEAN" {
			t.Errorf("sequential isolation failed: %v", result.Value)
		}
	})
}

func TestContextIsolation_Concurrent(t *testing.T) {
	// Test concurrent isolation with multiple runtimes
	engine := New(Config{PoolSize: 4})
	defer func() { _ = engine.Close() }()

	ctx := context.Background()
	const numGoroutines = 20
	const iterations = 10

	var wg sync.WaitGroup
	leaked := make(chan string, numGoroutines*iterations)

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for i := 0; i < iterations; i++ {
				// Set a unique global for this goroutine/iteration
				setCode := "globalThis.concurrentTest" + string(rune('A'+goroutineID)) + string(rune('0'+i)) + " = 'secret'"
				_, err := engine.Execute(ctx, setCode, nil)
				if err != nil {
					continue
				}

				// Try to read a different goroutine's global
				otherG := (goroutineID + 1) % numGoroutines
				checkCode := "typeof globalThis.concurrentTest" + string(rune('A'+otherG)) + string(rune('0'+i)) + " !== 'undefined' ? 'LEAK' : 'CLEAN'"
				result, err := engine.Execute(ctx, checkCode, nil)
				if err != nil {
					continue
				}

				if result.Value == "LEAK" {
					leaked <- "goroutine " + string(rune('0'+goroutineID)) + " saw goroutine " + string(rune('0'+otherG)) + "'s data"
				}
			}
		}(g)
	}

	wg.Wait()
	close(leaked)

	var leaks []string
	for l := range leaked {
		leaks = append(leaks, l)
	}

	if len(leaks) > 0 {
		t.Errorf("concurrent isolation failed with %d leaks: %v", len(leaks), leaks[:min(5, len(leaks))])
	}
}
