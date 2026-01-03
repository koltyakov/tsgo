package bun

import (
	"context"
	"os/exec"
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
	defer engine.Close()

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
