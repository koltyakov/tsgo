package tsgo

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"os"
	"strings"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	executor := New()
	if executor == nil {
		t.Fatal("expected executor")
	}
	defer func() { _ = executor.Close() }()
}

func TestNewWithOptions(t *testing.T) {
	executor := New(
		WithEngine(EngineGOJA),
		WithTimeout(5*time.Second),
		WithMemoryLimit(100*1024*1024),
		WithGlobals(map[string]any{"test": 123}),
		WithSourceMaps(true),
		WithPoolSize(4),
	)
	defer func() { _ = executor.Close() }()

	if executor.config.Engine != EngineGOJA {
		t.Errorf("expected engine GOJA, got %v", executor.config.Engine)
	}
	if executor.config.Timeout.Duration() != 5*time.Second {
		t.Errorf("expected timeout 5s, got %v", executor.config.Timeout)
	}
}

func TestWithDebugLogger(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	executor := New(
		WithEngine(EngineGOJA),
		WithDebugLogger(logger),
	)
	defer func() { _ = executor.Close() }()

	if executor.config.Logger != logger {
		t.Error("expected logger to be set")
	}
}

func TestWithBackgroundWarmup(t *testing.T) {
	executor := New(
		WithBackgroundWarmup(true),
	)
	defer func() { _ = executor.Close() }()

	if !executor.config.BackgroundWarmup {
		t.Error("expected BackgroundWarmup to be true")
	}
}

func TestWithFunctions(t *testing.T) {
	executor := New(
		WithEngine(EngineGOJA),
		WithFunctions(map[string]FunctionDef{
			"add": {
				TSCode: `function add(a: number, b: number): number { return a + b; }`,
			},
			"sqrt": {
				TSCode: `function sqrt(x: number): number { return Math.sqrt(x); }`,
				GoFunc: math.Sqrt,
			},
		}),
	)
	defer func() { _ = executor.Close() }()

	ctx := context.Background()

	// Test TSCode-only function
	result, err := executor.Execute(ctx, `export default add(10, 20);`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, ok := result.Value.(int64); !ok || v != 30 {
		t.Errorf("expected 30, got %v", result.Value)
	}

	// Test TSCode+GoFunc function (GOJA uses GoFunc)
	result, err = executor.Execute(ctx, `export default sqrt(16);`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// GoFunc returns float64, check both possible types
	switch v := result.Value.(type) {
	case float64:
		if v != 4 {
			t.Errorf("expected 4, got %v", v)
		}
	case int64:
		if v != 4 {
			t.Errorf("expected 4, got %v", v)
		}
	default:
		t.Errorf("expected numeric type, got %T", result.Value)
	}
}

func TestWithFunctions_Bun(t *testing.T) {
	executor := New(
		WithEngine(EngineBun),
		WithTimeout(5*time.Second),
		WithFunctions(map[string]FunctionDef{
			"multiply": {
				TSCode: `function multiply(a: number, b: number): number { return a * b; }`,
			},
		}),
	)
	defer func() { _ = executor.Close() }()

	ctx := context.Background()
	result, err := executor.Execute(ctx, `export default multiply(6, 7);`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, ok := result.Value.(float64); !ok || v != 42 {
		t.Errorf("expected 42, got %v (%T)", result.Value, result.Value)
	}
}

func TestExecute_SimpleExpression(t *testing.T) {
	executor := New(WithEngine(EngineGOJA))
	defer func() { _ = executor.Close() }()

	ctx := context.Background()
	result, err := executor.Execute(ctx, `1 + 2`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
}

func TestExecute_WithGlobals(t *testing.T) {
	executor := New(
		WithEngine(EngineGOJA),
		WithGlobals(map[string]any{"multiplier": 10}),
	)
	defer func() { _ = executor.Close() }()

	ctx := context.Background()
	result, err := executor.Execute(ctx, `5 * multiplier`)

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

func TestExecute_TypeScript(t *testing.T) {
	executor := New(WithEngine(EngineGOJA))
	defer func() { _ = executor.Close() }()

	ctx := context.Background()
	result, err := executor.Execute(ctx, `
		const greet = (name: string): string => {
			return "Hello, " + name + "!";
		};
		export default greet("TypeScript");
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Value != "Hello, TypeScript!" {
		t.Errorf("expected 'Hello, TypeScript!', got %v", result.Value)
	}
}

func TestExecute_SecurityRestriction(t *testing.T) {
	executor := New(
		WithEngine(EngineGOJA),
		WithSecurity(SecurityPolicy{
			RestrictedGlobals: []string{"eval"},
		}),
	)
	defer func() { _ = executor.Close() }()

	ctx := context.Background()
	_, err := executor.Execute(ctx, `eval("1 + 1")`)

	if err == nil {
		t.Error("expected error for restricted global")
	}
}

func TestEngineConstants(t *testing.T) {
	if EngineAuto != 0 {
		t.Error("expected EngineAuto to be 0")
	}
	if EngineGOJA != 1 {
		t.Error("expected EngineGOJA to be 1")
	}
}

func TestNewTypeBuilder(t *testing.T) {
	builder := NewTypeBuilder()
	if builder == nil {
		t.Fatal("expected builder")
	}

	builder.AddGlobal("testVar", "string")
	dts := builder.Build()

	if !strings.Contains(dts, "testVar") {
		t.Error("expected testVar in output")
	}
}

func TestGenerateContextTypes(t *testing.T) {
	dts := GenerateContextTypes(map[string]any{"userId": 123})
	if !strings.Contains(dts, "userId") {
		t.Error("expected userId in output")
	}
}

func TestAnalyzeContract(t *testing.T) {
	code := `
		interface User {
			id: number;
			name: string;
		}
		const user: User = { id: 1, name: "Alice" };
		export default user;
	`

	contract, err := AnalyzeContract(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if contract == nil {
		t.Fatal("expected contract")
	}

	if contract.Type == nil {
		t.Fatal("expected type in contract")
	}

	// Type should be expanded to structural form
	if contract.Type.Name != "{ id: number; name: string; }" {
		t.Errorf("expected expanded type, got %s", contract.Type.Name)
	}

	// Test TypeScript generation
	ts := contract.ToTypeScript()
	if !strings.Contains(ts, "export type Result") {
		t.Error("expected TypeScript output")
	}

	// Test JSON Schema generation
	schema := contract.ToJSONSchema()
	if schema.Schema == "" {
		t.Error("expected JSON Schema")
	}
}

func TestNewContractAnalyzer(t *testing.T) {
	analyzer := NewContractAnalyzer()
	if analyzer == nil {
		t.Fatal("expected analyzer")
	}

	code := `export default { value: 42 };`
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if contract.Type.Kind != "object" {
		t.Errorf("expected object kind, got %s", contract.Type.Kind)
	}
}

func TestDefaultMonacoConfig(t *testing.T) {
	cfg := DefaultMonacoConfig()
	if cfg.Host != "localhost" {
		t.Errorf("expected localhost, got %s", cfg.Host)
	}
}

func TestNewMonacoHandler(t *testing.T) {
	handler := NewMonacoHandler()
	if handler == nil {
		t.Fatal("expected handler")
	}
}

func TestMonacoClientScript(t *testing.T) {
	script := MonacoClientScript()
	if !strings.Contains(script, "tsgoMonaco") {
		t.Error("expected tsgoMonaco in script")
	}
}

func TestExecute_ClosedExecutor(t *testing.T) {
	executor := New(WithEngine(EngineGOJA))
	_ = executor.Close()

	ctx := context.Background()
	_, err := executor.Execute(ctx, `1 + 1`)
	if err != ErrExecutorClosed {
		t.Errorf("expected ErrExecutorClosed, got %v", err)
	}
}

func TestExecute_EmptyCode(t *testing.T) {
	executor := New(WithEngine(EngineGOJA))
	defer func() { _ = executor.Close() }()

	ctx := context.Background()
	_, err := executor.Execute(ctx, ``)
	if err != ErrEmptyCode {
		t.Errorf("expected ErrEmptyCode, got %v", err)
	}
}

func TestExecute_CancelledContext(t *testing.T) {
	executor := New(WithEngine(EngineGOJA))
	defer func() { _ = executor.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := executor.Execute(ctx, `1 + 1`)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestExecute_BunEngine(t *testing.T) {
	executor := New(
		WithEngine(EngineBun),
		WithTimeout(5*time.Second),
	)
	defer func() { _ = executor.Close() }()

	ctx := context.Background()
	result, err := executor.Execute(ctx, `export default 2 + 3;`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, ok := result.Value.(float64); !ok || v != 5 {
		t.Errorf("expected 5, got %v", result.Value)
	}
}

func TestExecute_BunWithGlobals(t *testing.T) {
	executor := New(
		WithEngine(EngineBun),
		WithTimeout(5*time.Second),
		WithGlobals(map[string]any{"factor": 10}),
	)
	defer func() { _ = executor.Close() }()

	ctx := context.Background()
	result, err := executor.Execute(ctx, `export default 5 * factor;`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, ok := result.Value.(float64); !ok || v != 50 {
		t.Errorf("expected 50, got %v", result.Value)
	}
}

func TestExecute_TopLevelAwait(t *testing.T) {
	executor := New(
		WithEngine(EngineBun),
		WithTimeout(5*time.Second),
	)
	defer func() { _ = executor.Close() }()

	ctx := context.Background()
	result, err := executor.Execute(ctx, `
		const value = await Promise.resolve(42);
		export default value;
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, ok := result.Value.(float64); !ok || v != 42 {
		t.Errorf("expected 42, got %v", result.Value)
	}
}

func TestExecute_GOJAUnsupportedFeatures(t *testing.T) {
	executor := New(WithEngine(EngineGOJA))
	defer func() { _ = executor.Close() }()

	ctx := context.Background()
	_, err := executor.Execute(ctx, `
		async function test() { return 1; }
		export default await test();
	`)
	if err == nil {
		t.Error("expected error for async in GOJA")
	}
	// Check for any error message related to async/await not being supported
	errMsg := err.Error()
	if !strings.Contains(errMsg, "async") && !strings.Contains(errMsg, "GOJA") {
		t.Errorf("expected async/await or GOJA related error, got %v", err)
	}
}

func TestExecute_TranspilationError(t *testing.T) {
	executor := New(WithEngine(EngineGOJA))
	defer func() { _ = executor.Close() }()

	ctx := context.Background()
	_, err := executor.Execute(ctx, `const x: = invalid syntax here {{{`)
	if err == nil {
		t.Error("expected transpilation error")
	}
}

func TestExecute_AutoEngineSelection(t *testing.T) {
	executor := New(WithEngine(EngineAuto))
	defer func() { _ = executor.Close() }()

	ctx := context.Background()

	// Simple code should work with auto selection
	result, err := executor.Execute(ctx, `export default 1 + 1;`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Value == nil {
		t.Error("expected result value")
	}
}

func TestExecute_SourceMapError(t *testing.T) {
	executor := New(
		WithEngine(EngineGOJA),
		WithSourceMaps(true),
	)
	defer func() { _ = executor.Close() }()

	ctx := context.Background()
	_, err := executor.Execute(ctx, `
		function test() {
			throw new Error("test error");
		}
		test();
	`)
	if err == nil {
		t.Error("expected error")
	}
}

func TestClose_Idempotent(t *testing.T) {
	executor := New(WithEngine(EngineGOJA))

	// Close multiple times should not panic
	err1 := executor.Close()
	err2 := executor.Close()
	err3 := executor.Close()

	if err1 != nil {
		t.Errorf("first close should succeed: %v", err1)
	}
	if err2 != nil {
		t.Errorf("second close should succeed: %v", err2)
	}
	if err3 != nil {
		t.Errorf("third close should succeed: %v", err3)
	}
}

func TestClose_WithBunEngine(t *testing.T) {
	executor := New(
		WithEngine(EngineBun),
		WithTimeout(5*time.Second),
	)

	// Execute something to initialize the engine
	ctx := context.Background()
	_, _ = executor.Execute(ctx, `export default 1;`)

	err := executor.Close()
	if err != nil {
		t.Errorf("close should succeed: %v", err)
	}
}

func TestExecutionError_Error(t *testing.T) {
	err := &ExecutionError{
		Message: "test error",
		Code:    "some code",
	}
	if err.Error() != "test error" {
		t.Errorf("expected 'test error', got %q", err.Error())
	}
}

func TestExecutionError_Unwrap(t *testing.T) {
	cause := errors.New("root cause")
	err := &ExecutionError{
		Message: "wrapper",
		Cause:   cause,
	}
	if err.Unwrap() != cause {
		t.Error("expected unwrap to return cause")
	}
}

func TestExecute_ComparisonExpression(t *testing.T) {
	executor := New(WithEngine(EngineGOJA))
	defer func() { _ = executor.Close() }()

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
		{"with variables", "let a = '1';\nlet b = '2';\na === b", false},
		{"logical and", "true && false", false},
		{"logical or", "true || false", true},
		{"complex comparison", "const x = 5;\nx > 3 && x < 10", true},
	}

	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := executor.Execute(ctx, tt.code)
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

func TestExecutorStats(t *testing.T) {
	executor := New(
		WithEngine(EngineGOJA),
		WithPoolSize(2),
	)
	defer func() { _ = executor.Close() }()

	// Get initial stats (engine not yet initialized - lazy loading)
	stats := executor.Stats()

	if stats.EngineConfigured != EngineGOJA {
		t.Errorf("expected engine GOJA, got %v", stats.EngineConfigured)
	}

	// Engine is lazily initialized, so it shouldn't be active yet
	if stats.GOJAActive {
		t.Error("expected GOJA to not be active before first execution")
	}

	if stats.BunActive {
		t.Error("expected Bun to not be active")
	}
	if stats.TranspilerCacheCapacity <= 0 {
		t.Errorf("expected transpiler cache capacity > 0, got %d", stats.TranspilerCacheCapacity)
	}
	if stats.TranspilerCacheSize != 0 {
		t.Errorf("expected empty transpiler cache at start, got %d", stats.TranspilerCacheSize)
	}

	// Execute something to initialize the engine
	ctx := context.Background()
	_, _ = executor.Execute(ctx, `export default 42;`)

	// Get stats again after execution
	stats = executor.Stats()
	if !stats.GOJAActive {
		t.Error("expected GOJA to be active after execution")
	}
	if stats.TranspilerCacheSize <= 0 {
		t.Errorf("expected transpiler cache to have entries after execution, got %d", stats.TranspilerCacheSize)
	}
}

func TestExecutorStatsWithBun(t *testing.T) {
	executor := New(
		WithEngine(EngineBun),
		WithPoolSize(1),
	)
	defer func() { _ = executor.Close() }()

	// Execute to initialize Bun engine
	ctx := context.Background()
	_, _ = executor.Execute(ctx, `export default 1;`)

	stats := executor.Stats()
	if stats.EngineConfigured != EngineBun {
		t.Errorf("expected engine Bun, got %v", stats.EngineConfigured)
	}
}

func TestExecutorStatsWithAuto(t *testing.T) {
	executor := New(WithEngine(EngineAuto))
	defer func() { _ = executor.Close() }()

	stats := executor.Stats()
	if stats.EngineConfigured != EngineAuto {
		t.Errorf("expected engine Auto, got %v", stats.EngineConfigured)
	}
}

func TestClose_WithInFlightExecution(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})

	executor := New(
		WithEngine(EngineGOJA),
		WithTimeout(2*time.Second),
		WithFunctions(map[string]FunctionDef{
			"blocker": {
				GoFunc: func() int {
					close(started)
					<-release
					return 1
				},
			},
		}),
	)

	done := make(chan error, 1)
	go func() {
		_, err := executor.Execute(context.Background(), `export default blocker();`)
		done <- err
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for execution to start")
	}

	closeDone := make(chan error, 1)
	go func() {
		closeDone <- executor.Close()
	}()

	select {
	case err := <-closeDone:
		t.Fatalf("close returned before in-flight execution completed: %v", err)
	case <-time.After(100 * time.Millisecond):
		// Expected: close should wait until execution finishes.
	}

	close(release)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("execution failed while close was pending: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for in-flight execution")
	}

	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("close failed: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for close")
	}

	_, err := executor.Execute(context.Background(), `export default 1;`)
	if err != ErrExecutorClosed {
		t.Fatalf("expected ErrExecutorClosed after close, got %v", err)
	}
}
