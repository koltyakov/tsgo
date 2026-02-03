package bun

import (
	"context"
	"errors"
	"os/exec"
	"sync"
	"testing"
	"time"
)

func TestBunEngine(t *testing.T) {
	// Check if Bun is available
	if _, err := exec.LookPath("bun"); err != nil {
		t.Skip("Bun is not installed")
	}

	engine, err := New(Config{PoolSize: 2})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer func() { _ = engine.Close() }()

	if !engine.IsAvailable() {
		t.Skip("Bun engine is not available")
	}

	t.Run("simple execution", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := engine.Execute(ctx, `export default 42`, nil)
		if err != nil {
			t.Fatalf("execution failed: %v", err)
		}

		// Bun may return float64 from JSON
		switch v := result.Value.(type) {
		case float64:
			if v != 42 {
				t.Errorf("expected 42, got %v", v)
			}
		case int:
			if v != 42 {
				t.Errorf("expected 42, got %v", v)
			}
		default:
			t.Errorf("expected number, got %T: %v", v, v)
		}
	})

	t.Run("with globals", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := engine.Execute(ctx, `export default x + y`, map[string]any{
			"x": 10,
			"y": 32,
		})
		if err != nil {
			t.Fatalf("execution failed: %v", err)
		}

		switch v := result.Value.(type) {
		case float64:
			if v != 42 {
				t.Errorf("expected 42, got %v", v)
			}
		case int:
			if v != 42 {
				t.Errorf("expected 42, got %v", v)
			}
		}
	})

	t.Run("async execution", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := engine.Execute(ctx, `
			const delay = (ms: number) => new Promise(resolve => setTimeout(resolve, ms));
			export default async function() {
				await delay(10);
				return "async-complete";
			}
		`, nil)
		if err != nil {
			t.Fatalf("execution failed: %v", err)
		}

		if result.Value != "async-complete" {
			t.Errorf("expected 'async-complete', got %v", result.Value)
		}
	})

	t.Run("error handling", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := engine.Execute(ctx, `throw new Error("test error")`, nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("concurrent execution", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		const n = 5
		results := make(chan int, n)
		errors := make(chan error, n)

		for i := 0; i < n; i++ {
			i := i
			go func() {
				result, err := engine.Execute(ctx, `export default value * 2`, map[string]any{
					"value": i,
				})
				if err != nil {
					errors <- err
					return
				}
				switch v := result.Value.(type) {
				case float64:
					results <- int(v)
				case int:
					results <- v
				}
			}()
		}

		// Collect results
		sum := 0
		for i := 0; i < n; i++ {
			select {
			case r := <-results:
				sum += r
			case err := <-errors:
				t.Fatalf("concurrent execution failed: %v", err)
			case <-ctx.Done():
				t.Fatal("timeout waiting for results")
			}
		}

		// sum of (0*2 + 1*2 + 2*2 + 3*2 + 4*2) = 20
		expected := 20
		if sum != expected {
			t.Errorf("expected sum %d, got %d", expected, sum)
		}
	})
}

func TestBunEngineNotAvailable(t *testing.T) {
	engine, _ := New(Config{
		ExecutablePath: "/nonexistent/bun",
		PoolSize:       1,
	})

	if engine.IsAvailable() {
		t.Error("engine should not be available with nonexistent executable")
	}
}

func TestIsProcessCrash(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "broken pipe",
			err:      errors.New("write |1: broken pipe"),
			expected: true,
		},
		{
			name:     "connection reset",
			err:      errors.New("read tcp: connection reset by peer"),
			expected: true,
		},
		{
			name:     "unexpected EOF",
			err:      errors.New("unexpected EOF"),
			expected: true,
		},
		{
			name:     "process died",
			err:      errors.New("process died"),
			expected: true,
		},
		{
			name:     "normal script error",
			err:      errors.New("ReferenceError: x is not defined"),
			expected: false,
		},
		{
			name:     "timeout error",
			err:      context.DeadlineExceeded,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isProcessCrash(tt.err)
			if got != tt.expected {
				t.Errorf("isProcessCrash(%q) = %v, want %v", tt.err, got, tt.expected)
			}
		})
	}
}

func TestContextIsolation(t *testing.T) {
	// Check if Bun is available
	if _, err := exec.LookPath("bun"); err != nil {
		t.Skip("Bun is not installed")
	}

	// This test verifies that context does not leak between executions.
	// Critical for BPMN engines where each process must be isolated.
	engine, err := New(Config{PoolSize: 1}) // Single process to guarantee reuse
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer func() { _ = engine.Close() }()

	if !engine.IsAvailable() {
		t.Skip("Bun engine is not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("globalThis pollution", func(t *testing.T) {
		// First execution: set a secret on globalThis
		_, err := engine.Execute(ctx, "globalThis.bunSecret = 'LEAKED'; export default 'done'", nil)
		if err != nil {
			t.Fatalf("first execution failed: %v", err)
		}

		// Second execution: verify the secret is NOT accessible
		result, err := engine.Execute(ctx, "export default typeof globalThis.bunSecret !== 'undefined' ? 'LEAK' : 'CLEAN'", nil)
		if err != nil {
			t.Fatalf("second execution failed: %v", err)
		}
		if result.Value != "CLEAN" {
			t.Errorf("context leak detected via globalThis: %v", result.Value)
		}
	})

	t.Run("injected globals cleaned", func(t *testing.T) {
		// Execute with an injected global
		globals := map[string]any{"injectedSecret": "SECRET123"}
		_, err := engine.Execute(ctx, "export default injectedSecret", globals)
		if err != nil {
			t.Fatalf("first execution failed: %v", err)
		}

		// Execute without the global - should not be accessible
		result, err := engine.Execute(ctx, "export default typeof injectedSecret !== 'undefined' ? 'LEAK' : 'CLEAN'", nil)
		if err != nil {
			t.Fatalf("second execution failed: %v", err)
		}
		if result.Value != "CLEAN" {
			t.Errorf("injected global leaked between executions: %v", result.Value)
		}
	})

	t.Run("function pollution", func(t *testing.T) {
		// Define a function on globalThis
		_, err := engine.Execute(ctx, "globalThis.leakedFunc = function() { return 'SECRET' }; export default 'done'", nil)
		if err != nil {
			t.Fatalf("first execution failed: %v", err)
		}

		// Verify function is not accessible
		result, err := engine.Execute(ctx, "export default typeof leakedFunc !== 'undefined' ? 'LEAK' : 'CLEAN'", nil)
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
			code := "globalThis.seqTest" + string(rune('0'+i)) + " = " + string(rune('0'+i)) + "; export default 'done'"
			_, err := engine.Execute(ctx, code, nil)
			if err != nil {
				t.Fatalf("execution %d failed: %v", i, err)
			}
		}

		// None should be accessible
		result, err := engine.Execute(ctx, "export default typeof seqTest0 !== 'undefined' || typeof seqTest4 !== 'undefined' ? 'LEAK' : 'CLEAN'", nil)
		if err != nil {
			t.Fatalf("check execution failed: %v", err)
		}
		if result.Value != "CLEAN" {
			t.Errorf("sequential isolation failed: %v", result.Value)
		}
	})
}

func TestContextIsolation_Concurrent(t *testing.T) {
	// Check if Bun is available
	if _, err := exec.LookPath("bun"); err != nil {
		t.Skip("Bun is not installed")
	}

	// Test concurrent isolation with multiple processes
	engine, err := New(Config{PoolSize: 4})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer func() { _ = engine.Close() }()

	if !engine.IsAvailable() {
		t.Skip("Bun engine is not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	const numGoroutines = 10
	const iterations = 5

	var wg sync.WaitGroup
	leaked := make(chan string, numGoroutines*iterations)

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for i := 0; i < iterations; i++ {
				// Set a unique global for this goroutine/iteration
				setCode := "globalThis.concurrentTest" + string(rune('A'+goroutineID)) + string(rune('0'+i)) + " = 'secret'; export default 'done'"
				_, err := engine.Execute(ctx, setCode, nil)
				if err != nil {
					continue
				}

				// Try to read a different goroutine's global
				otherG := (goroutineID + 1) % numGoroutines
				checkCode := "export default typeof globalThis.concurrentTest" + string(rune('A'+otherG)) + string(rune('0'+i)) + " !== 'undefined' ? 'LEAK' : 'CLEAN'"
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
