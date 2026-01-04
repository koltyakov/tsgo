package typeinfer

import (
	"context"
	"testing"
	"time"
)

func TestIsBunAvailable(t *testing.T) {
	available := IsBunAvailable()
	t.Logf("Bun available: %v", available)
}

func TestInferDefaultExport(t *testing.T) {
	if !IsBunAvailable() {
		t.Skip("Bun not available, skipping TypeScript inference tests")
	}

	inferrer := NewInferrer().WithTimeout(10 * time.Second)
	ctx := context.Background()

	tests := []struct {
		name     string
		code     string
		wantType string
		wantKind string
	}{
		{
			name:     "string literal",
			code:     `export default "hello" as const`,
			wantType: `"hello"`,
			wantKind: "literal",
		},
		{
			name:     "number literal",
			code:     `export default 42 as const`,
			wantType: "42",
			wantKind: "literal",
		},
		{
			name:     "boolean",
			code:     `export default true`,
			wantType: "true",
			wantKind: "literal",
		},
		{
			name:     "string variable",
			code:     `const x: string = "hello"; export default x`,
			wantType: "string",
			wantKind: "primitive",
		},
		{
			name:     "number variable",
			code:     `const x: number = 42; export default x`,
			wantType: "number",
			wantKind: "primitive",
		},
		{
			name:     "bigint",
			code:     `const x: bigint = 42n; export default x`,
			wantType: "bigint",
			wantKind: "primitive",
		},
		{
			name:     "simple object",
			code:     `export default { name: "test", count: 42 }`,
			wantType: "{ name: string; count: number; }",
			wantKind: "object",
		},
		{
			name:     "array of numbers",
			code:     `export default [1, 2, 3]`,
			wantType: "number[]",
			wantKind: "array",
		},
		{
			name:     "array of strings",
			code:     `export default ["a", "b", "c"]`,
			wantType: "string[]",
			wantKind: "array",
		},
		{
			name:     "function via const",
			code:     `const fn = function(x: number): string { return x.toString(); }; export default fn`,
			wantType: "(x: number) => string",
			wantKind: "function",
		},
		{
			name:     "arrow function",
			code:     `export default (x: number): string => x.toString()`,
			wantType: "(x: number) => string",
			wantKind: "function",
		},
		{
			name:     "promise",
			code:     `export default Promise.resolve(42)`,
			wantType: "Promise<number>",
			wantKind: "object",
		},
		{
			name:     "async function return",
			code:     `async function foo(): Promise<string> { return "hello"; } export default foo()`,
			wantType: "Promise<string>",
			wantKind: "object",
		},
		{
			name:     "union type",
			code:     `const x: string | number = "hello"; export default x`,
			wantType: "string | number",
			wantKind: "union",
		},
		{
			name:     "null",
			code:     `export default null`,
			wantType: "null",
			wantKind: "primitive",
		},
		{
			name:     "undefined",
			code:     `export default undefined`,
			wantType: "undefined",
			wantKind: "primitive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := inferrer.InferDefaultExport(ctx, tt.code)
			if err != nil {
				t.Fatalf("InferDefaultExport failed: %v", err)
			}
			if result.Error != "" {
				t.Fatalf("Inference returned error: %s", result.Error)
			}
			if result.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", result.Type, tt.wantType)
			}
			if result.Kind != tt.wantKind {
				t.Errorf("Kind = %q, want %q", result.Kind, tt.wantKind)
			}
		})
	}
}

func TestInferBunNanoseconds(t *testing.T) {
	if !IsBunAvailable() {
		t.Skip("Bun not available, skipping TypeScript inference tests")
	}

	inferrer := NewInferrer().WithTimeout(10 * time.Second)
	ctx := context.Background()

	code := `declare const Bun: {
  nanoseconds(): bigint;
};

export default Bun.nanoseconds()`

	result, err := inferrer.InferDefaultExport(ctx, code)
	if err != nil {
		t.Fatalf("InferDefaultExport failed: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("Inference returned error: %s", result.Error)
	}

	if result.Type != "bigint" {
		t.Errorf("Type = %q, want %q", result.Type, "bigint")
	}
	if result.Kind != "primitive" {
		t.Errorf("Kind = %q, want %q", result.Kind, "primitive")
	}
}

func TestInferObjectWithMethods(t *testing.T) {
	if !IsBunAvailable() {
		t.Skip("Bun not available, skipping TypeScript inference tests")
	}

	inferrer := NewInferrer().WithTimeout(10 * time.Second)
	ctx := context.Background()

	code := `interface API {
  get(url: string): Promise<string>;
  post(url: string, body: object): Promise<object>;
}
declare const api: API;
export default api.get("https://example.com")`

	result, err := inferrer.InferDefaultExport(ctx, code)
	if err != nil {
		t.Fatalf("InferDefaultExport failed: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("Inference returned error: %s", result.Error)
	}

	if result.Type != "Promise<string>" {
		t.Errorf("Type = %q, want %q", result.Type, "Promise<string>")
	}
}

func TestNoDefaultExport(t *testing.T) {
	if !IsBunAvailable() {
		t.Skip("Bun not available, skipping TypeScript inference tests")
	}

	inferrer := NewInferrer().WithTimeout(10 * time.Second)
	ctx := context.Background()

	// Code with only statements (no trailing expression) should return void or any
	result, err := inferrer.InferDefaultExport(ctx, `const x = 42`)
	if err != nil {
		t.Fatalf("InferDefaultExport failed: %v", err)
	}

	// Should return void or any since there's no expression to infer
	if result.Type != "void" && result.Type != "any" {
		t.Errorf("Type = %q, want %q or %q", result.Type, "void", "any")
	}
}

func TestReplExpressionInference(t *testing.T) {
	if !IsBunAvailable() {
		t.Skip("Bun not available, skipping TypeScript inference tests")
	}

	inferrer := NewInferrer().WithTimeout(10 * time.Second)
	ctx := context.Background()

	tests := []struct {
		name     string
		code     string
		wantType string
		wantKind string
	}{
		{
			name:     "trailing variable",
			code:     "const x = 42\nx",
			wantType: "42",
			wantKind: "literal",
		},
		{
			name:     "trailing expression",
			code:     "const x = 10\nconst y = 20\nx + y",
			wantType: "number",
			wantKind: "primitive",
		},
		{
			name:     "trailing function call",
			code:     "declare const Bun: { nanoseconds(): bigint };\nBun.nanoseconds()",
			wantType: "bigint",
			wantKind: "primitive",
		},
		{
			name:     "trailing object literal",
			code:     "const name = 'test'\n{ name, count: 42 }",
			wantType: "{ name: string; count: number; }",
			wantKind: "object",
		},
		{
			name:     "trailing array",
			code:     "const x = 1\n[x, 2, 3]",
			wantType: "number[]",
			wantKind: "array",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := inferrer.InferDefaultExport(ctx, tt.code)
			if err != nil {
				t.Fatalf("InferDefaultExport failed: %v", err)
			}
			if result.Error != "" {
				t.Fatalf("Inference returned error: %s", result.Error)
			}
			if result.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", result.Type, tt.wantType)
			}
			if result.Kind != tt.wantKind {
				t.Errorf("Kind = %q, want %q", result.Kind, tt.wantKind)
			}
		})
	}
}
