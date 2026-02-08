package transpiler

import (
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	tr := New()
	if tr == nil {
		t.Fatal("expected transpiler to be created")
	}
}

func TestTranspile_SimpleExpression(t *testing.T) {
	tr := New()

	js, _, _, _, err := tr.Transpile("const x: number = 42;")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if js == "" {
		t.Error("expected JavaScript output")
	}
}

func TestTranspile_TypeScriptSyntax(t *testing.T) {
	tr := New()

	code := `
		interface User {
			name: string;
			age: number;
		}

		const user: User = { name: "Alice", age: 30 };
	`

	js, _, _, _, err := tr.Transpile(code)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should not contain TypeScript-specific syntax
	if strings.Contains(js, "interface") {
		t.Error("expected interface to be stripped")
	}
	if strings.Contains(js, ": User") {
		t.Error("expected type annotation to be stripped")
	}
}

func TestTranspile_ArrowFunction(t *testing.T) {
	tr := New()

	code := `const add = (a: number, b: number): number => a + b;`
	js, _, _, _, err := tr.Transpile(code)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if js == "" {
		t.Error("expected JavaScript output")
	}
}

func TestTranspile_SyntaxError(t *testing.T) {
	tr := New()

	_, _, _, _, err := tr.Transpile("const x: = ")

	if err == nil {
		t.Error("expected syntax error")
	}
}

func TestTranspile_Caching(t *testing.T) {
	tr := New()

	code := "const x = 1;"

	// First call
	js1, _, _, _, err := tr.Transpile(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Second call should use cache
	js2, _, _, _, err := tr.Transpile(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if js1 != js2 {
		t.Error("expected cached result to match")
	}
}

func TestClearCache(t *testing.T) {
	tr := New()

	code := "const x = 1;"
	_, _, _, _, _ = tr.Transpile(code)

	tr.ClearCache()

	// Should still work after clearing cache
	_, _, _, _, err := tr.Transpile(code)
	if err != nil {
		t.Fatalf("unexpected error after cache clear: %v", err)
	}
}

func TestCacheStats(t *testing.T) {
	tr := NewWithCacheSize(2)

	size, capacity := tr.CacheStats()
	if capacity != 2 {
		t.Fatalf("expected cache capacity 2, got %d", capacity)
	}
	if size != 0 {
		t.Fatalf("expected empty cache initially, got %d", size)
	}

	_, _, _, _, err := tr.Transpile("const x = 1;")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	size, capacity = tr.CacheStats()
	if capacity != 2 {
		t.Fatalf("expected cache capacity 2, got %d", capacity)
	}
	if size == 0 {
		t.Fatal("expected cache to contain at least one entry")
	}
}

func TestExtractInlineSourceMap(t *testing.T) {
	code := `var x = 1;
//# sourceMappingURL=data:application/json;base64,eyJ2ZXJzaW9uIjozfQ==`

	sm := extractInlineSourceMap(code)
	if sm != "eyJ2ZXJzaW9uIjozfQ==" {
		t.Errorf("expected base64 source map, got: %s", sm)
	}
}

func TestExtractInlineSourceMap_NotFound(t *testing.T) {
	code := "var x = 1;"
	sm := extractInlineSourceMap(code)
	if sm != "" {
		t.Errorf("expected empty string, got: %s", sm)
	}
}

func TestHashCode(t *testing.T) {
	h1 := hashCode("hello")
	h2 := hashCode("hello")
	h3 := hashCode("world")

	if h1 != h2 {
		t.Error("same input should produce same hash")
	}
	if h1 == h3 {
		t.Error("different input should produce different hash")
	}
	// FNV-1a 64-bit produces base36 string of ~13 chars
	if len(h1) == 0 || len(h1) > 16 {
		t.Errorf("expected non-empty hash, got length %d", len(h1))
	}
}

func TestTranspileError(t *testing.T) {
	err := &TranspileError{
		Message: "test error",
		Line:    10,
		Column:  5,
	}

	if err.Error() != "test error" {
		t.Errorf("expected 'test error', got %q", err.Error())
	}
}

func TestLRUCache(t *testing.T) {
	cache := newLRUCache(2)

	cache.put("a", 1)
	cache.put("b", 2)

	if v, ok := cache.get("a"); !ok || v != 1 {
		t.Error("expected a=1")
	}
	if v, ok := cache.get("b"); !ok || v != 2 {
		t.Error("expected b=2")
	}

	// Add third item, should evict "a" since "b" was accessed more recently
	cache.put("c", 3)

	// "b" should still be present (accessed after "a")
	if v, ok := cache.get("b"); !ok || v != 2 {
		t.Error("expected b=2 to still be present")
	}
	// "c" should be present
	if v, ok := cache.get("c"); !ok || v != 3 {
		t.Error("expected c=3 to be present")
	}
}

func TestPreprocessTrailingExpression(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string // what the result should contain
	}{
		{
			name:     "simple comparison",
			input:    "1 === 2",
			contains: "export default (1 === 2)",
		},
		{
			name:     "comparison with variables",
			input:    "let a = '1';\nlet b = '2';\na === b",
			contains: "export default (a === b)",
		},
		{
			name:     "logical expression",
			input:    "true && false",
			contains: "export default (true && false)",
		},
		{
			name:     "already has export default",
			input:    "export default 42;",
			contains: "export default 42",
		},
		{
			name:     "trailing number",
			input:    "const x = 1;\n42",
			contains: "export default (42)",
		},
		{
			name:     "arithmetic expression",
			input:    "const x = 10;\nx * 2 + 5",
			contains: "export default (x * 2 + 5)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := preprocessTrailingExpression(tt.input)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("expected result to contain %q, got %q", tt.contains, result)
			}
		})
	}
}

func TestIsDeclaration(t *testing.T) {
	declarations := []string{
		"const x = 1",
		"let y = 2",
		"var z = 3",
		"function foo() {}",
		"async function bar() {}",
		"interface User {}",
		"type Status = string",
		"class Person {}",
		"import { x } from 'y'",
		"export const a = 1",
		"declare const b: number",
	}

	for _, stmt := range declarations {
		if !isDeclaration(stmt) {
			t.Errorf("expected %q to be a declaration", stmt)
		}
	}

	expressions := []string{
		"1 === 2",
		"a && b",
		"foo()",
		"x + y",
		"obj.method()",
		"true",
		"42",
	}

	for _, stmt := range expressions {
		if isDeclaration(stmt) {
			t.Errorf("expected %q to NOT be a declaration", stmt)
		}
	}
}
