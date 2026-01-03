package goja

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	engine := New(Config{PoolSize: 2})
	if engine == nil {
		t.Fatal("expected engine to be created")
	}
	defer engine.Close()
}

func TestExecute_SimpleExpression(t *testing.T) {
	engine := New(Config{PoolSize: 2})
	defer engine.Close()

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
	defer engine.Close()

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
	defer engine.Close()

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
	defer engine.Close()

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
	defer engine.Close()

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
	defer engine.Close()

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

	// Acquire all runtimes
	r1, release1, err := p.acquire()
	if err != nil {
		t.Fatalf("failed to acquire: %v", err)
	}
	if r1 == nil {
		t.Fatal("expected runtime")
	}

	r2, release2, err := p.acquire()
	if err != nil {
		t.Fatalf("failed to acquire: %v", err)
	}
	if r2 == nil {
		t.Fatal("expected runtime")
	}

	// Third should create temporary
	r3, release3, err := p.acquire()
	if err != nil {
		t.Fatalf("failed to acquire: %v", err)
	}
	if r3 == nil {
		t.Fatal("expected runtime")
	}

	// Release all
	release3()
	release2()
	release1()
}

func TestPoolClosed(t *testing.T) {
	p := newPool(2)
	p.close()

	_, _, err := p.acquire()
	if err == nil {
		t.Error("expected error from closed pool")
	}
}
