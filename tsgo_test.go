package tsgo

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	executor := New()
	if executor == nil {
		t.Fatal("expected executor")
	}
	defer executor.Close()
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
	defer executor.Close()

	if executor.config.Engine != EngineGOJA {
		t.Errorf("expected engine GOJA, got %v", executor.config.Engine)
	}
	if executor.config.Timeout.Duration() != 5*time.Second {
		t.Errorf("expected timeout 5s, got %v", executor.config.Timeout)
	}
}

func TestExecute_SimpleExpression(t *testing.T) {
	executor := New(WithEngine(EngineGOJA))
	defer executor.Close()

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
	defer executor.Close()

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
	defer executor.Close()

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
	defer executor.Close()

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

	if contract.Type.Name != "User" {
		t.Errorf("expected User type, got %s", contract.Type.Name)
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

func TestExecute_ComparisonExpression(t *testing.T) {
	executor := New(WithEngine(EngineGOJA))
	defer executor.Close()

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
