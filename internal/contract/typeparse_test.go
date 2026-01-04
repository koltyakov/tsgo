package contract

import (
	"strings"
	"testing"
)

func TestInferKind(t *testing.T) {
	tests := []struct {
		name     string
		typeStr  string
		expected string
	}{
		// Primitives
		{"string", "string", "primitive"},
		{"number", "number", "primitive"},
		{"boolean", "boolean", "primitive"},
		{"bigint", "bigint", "primitive"},
		{"null", "null", "primitive"},
		{"undefined", "undefined", "primitive"},
		{"void", "void", "primitive"},
		{"never", "never", "primitive"},
		{"any", "any", "primitive"},
		{"unknown", "unknown", "primitive"},

		// Literals
		{"string literal", `"hello"`, "literal"},
		{"number literal", "42", "literal"},
		{"float literal", "3.14", "literal"},
		{"true literal", "true", "literal"},
		{"false literal", "false", "literal"},

		// Arrays
		{"simple array", "string[]", "array"},
		{"number array", "number[]", "array"},
		{"object array", "{ name: string; }[]", "array"},

		// Unions
		{"simple union", "string | number", "union"},
		{"triple union", "string | number | boolean", "union"},
		{"union with undefined", "string | undefined", "union"},

		// Functions
		{"arrow function", "(x: number) => string", "function"},
		{"void function", "() => void", "function"},

		// Objects
		{"simple object", "{ name: string; }", "object"},
		{"nested object", "{ user: { name: string; }; }", "object"},
		{"empty object", "{}", "object"},

		// Generic types
		{"Promise", "Promise<string>", "object"},
		{"Map", "Map<string, number>", "object"},
		{"Array generic", "Array<string>", "object"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := InferKind(tt.typeStr)
			if result != tt.expected {
				t.Errorf("InferKind(%q) = %q, want %q", tt.typeStr, result, tt.expected)
			}
		})
	}
}

func TestContainsTopLevelUnion(t *testing.T) {
	tests := []struct {
		name     string
		typeStr  string
		expected bool
	}{
		{"simple union", "string | number", true},
		{"no union", "string", false},
		{"union in object", "{ a: string | number; }", false},
		{"union in function", "(a: string | number) => void", false},
		{"union in generic", "Promise<string | number>", false},
		{"top level with nested", "{ a: string; } | { b: number; }", true},
		{"array element union wrapped", "(string | number)[]", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ContainsTopLevelUnion(tt.typeStr)
			if result != tt.expected {
				t.Errorf("ContainsTopLevelUnion(%q) = %v, want %v", tt.typeStr, result, tt.expected)
			}
		})
	}
}

func TestSplitUnion(t *testing.T) {
	tests := []struct {
		name     string
		typeStr  string
		expected []string
	}{
		{"simple union", "string | number", []string{"string ", " number"}},
		{"triple union", "string | number | boolean", []string{"string ", " number ", " boolean"}},
		{"no union", "string", []string{"string"}},
		{"union with objects", "{ a: string; } | { b: number; }", []string{"{ a: string; } ", " { b: number; }"}},
		{"nested union ignored", "{ a: string | number; }", []string{"{ a: string | number; }"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SplitUnion(tt.typeStr)
			if len(result) != len(tt.expected) {
				t.Errorf("SplitUnion(%q) got %d parts, want %d", tt.typeStr, len(result), len(tt.expected))
				return
			}
			for i, part := range result {
				if part != tt.expected[i] {
					t.Errorf("SplitUnion(%q)[%d] = %q, want %q", tt.typeStr, i, part, tt.expected[i])
				}
			}
		})
	}
}

func TestParseProperties(t *testing.T) {
	tests := []struct {
		name     string
		inner    string
		expected []string
	}{
		{"single property", "name: string", []string{"name: string"}},
		{"two properties", "name: string; age: number", []string{"name: string", " age: number"}},
		{"trailing semicolon", "name: string; age: number;", []string{"name: string", " age: number"}},
		{"nested object", "user: { name: string; age: number; }; active: boolean", []string{"user: { name: string; age: number; }", " active: boolean"}},
		{"array property", "items: string[]; count: number", []string{"items: string[]", " count: number"}},
		{"empty", "", []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseProperties(tt.inner)
			if len(result) != len(tt.expected) {
				t.Errorf("ParseProperties(%q) got %d parts, want %d: %v", tt.inner, len(result), len(tt.expected), result)
				return
			}
			for i, part := range result {
				if part != tt.expected[i] {
					t.Errorf("ParseProperties(%q)[%d] = %q, want %q", tt.inner, i, part, tt.expected[i])
				}
			}
		})
	}
}

func TestParseTypeString(t *testing.T) {
	tests := []struct {
		name         string
		typeStr      string
		kind         string
		expectedKind string
		checkProps   bool
		propCount    int
	}{
		{"primitive string", "string", "", "primitive", false, 0},
		{"primitive number", "number", "", "primitive", false, 0},
		{"literal string", `"hello"`, "", "literal", false, 0},
		{"literal number", "42", "", "literal", false, 0},
		{"array", "string[]", "", "array", false, 0},
		{"union", "string | number", "", "union", false, 0},
		{"function", "() => void", "", "function", false, 0},
		{"simple object", "{ name: string; }", "", "object", true, 1},
		{"two prop object", "{ name: string; age: number; }", "", "object", true, 2},
		{"nested object", "{ user: { name: string; }; }", "", "object", true, 1},
		{"forced kind", "anything", "primitive", "primitive", false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseTypeString(tt.typeStr, tt.kind)
			if result.Kind != tt.expectedKind {
				t.Errorf("ParseTypeString(%q, %q).Kind = %q, want %q", tt.typeStr, tt.kind, result.Kind, tt.expectedKind)
			}
			if tt.checkProps && len(result.Properties) != tt.propCount {
				t.Errorf("ParseTypeString(%q, %q) has %d properties, want %d", tt.typeStr, tt.kind, len(result.Properties), tt.propCount)
			}
		})
	}
}

func TestParseObjectType(t *testing.T) {
	tests := []struct {
		name      string
		typeStr   string
		propNames []string
		propTypes []string
		optional  []bool
	}{
		{
			name:      "single property",
			typeStr:   "{ name: string; }",
			propNames: []string{"name"},
			propTypes: []string{"primitive"},
			optional:  []bool{false},
		},
		{
			name:      "two properties",
			typeStr:   "{ name: string; age: number; }",
			propNames: []string{"name", "age"},
			propTypes: []string{"primitive", "primitive"},
			optional:  []bool{false, false},
		},
		{
			name:      "optional property",
			typeStr:   "{ name: string; age?: number; }",
			propNames: []string{"name", "age"},
			propTypes: []string{"primitive", "primitive"},
			optional:  []bool{false, true},
		},
		{
			name:      "nested object",
			typeStr:   "{ user: { name: string; }; }",
			propNames: []string{"user"},
			propTypes: []string{"object"},
			optional:  []bool{false},
		},
		{
			name:      "array property",
			typeStr:   "{ items: string[]; }",
			propNames: []string{"items"},
			propTypes: []string{"array"},
			optional:  []bool{false},
		},
		{
			name:      "complex nested",
			typeStr:   "{ data: { items: { id: number; name: string; }[]; }; }",
			propNames: []string{"data"},
			propTypes: []string{"object"},
			optional:  []bool{false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseObjectType(tt.typeStr)

			if result.Kind != "object" {
				t.Errorf("ParseObjectType(%q).Kind = %q, want \"object\"", tt.typeStr, result.Kind)
			}

			if len(result.Properties) != len(tt.propNames) {
				t.Errorf("ParseObjectType(%q) has %d properties, want %d", tt.typeStr, len(result.Properties), len(tt.propNames))
				return
			}

			for i, prop := range result.Properties {
				if prop.Name != tt.propNames[i] {
					t.Errorf("property[%d].Name = %q, want %q", i, prop.Name, tt.propNames[i])
				}
				if prop.Type.Kind != tt.propTypes[i] {
					t.Errorf("property[%d].Type.Kind = %q, want %q", i, prop.Type.Kind, tt.propTypes[i])
				}
				if prop.Required == tt.optional[i] {
					t.Errorf("property[%d].Required = %v, want %v", i, prop.Required, !tt.optional[i])
				}
			}
		})
	}
}

func TestParseTypeStringNestedObjectFormatting(t *testing.T) {
	// Test that deeply nested objects are properly parsed
	typeStr := "{ complianceSettings: { minPasswordLength: number; minAge: number; requireMFA: boolean; }; testResults: { validUser: { input: string; result: string; errors: string[]; }; }; }"

	result := ParseTypeString(typeStr, "")

	if result.Kind != "object" {
		t.Fatalf("expected object, got %s", result.Kind)
	}

	if len(result.Properties) != 2 {
		t.Fatalf("expected 2 properties, got %d", len(result.Properties))
	}

	// Check complianceSettings
	compliance := result.Properties[0]
	if compliance.Name != "complianceSettings" {
		t.Errorf("expected complianceSettings, got %s", compliance.Name)
	}
	if compliance.Type.Kind != "object" {
		t.Errorf("complianceSettings should be object, got %s", compliance.Type.Kind)
	}
	if len(compliance.Type.Properties) != 3 {
		t.Errorf("complianceSettings should have 3 properties, got %d", len(compliance.Type.Properties))
	}

	// Check testResults
	testResults := result.Properties[1]
	if testResults.Name != "testResults" {
		t.Errorf("expected testResults, got %s", testResults.Name)
	}
	if testResults.Type.Kind != "object" {
		t.Errorf("testResults should be object, got %s", testResults.Type.Kind)
	}

	// Check nested validUser
	if len(testResults.Type.Properties) > 0 {
		validUser := testResults.Type.Properties[0]
		if validUser.Name != "validUser" {
			t.Errorf("expected validUser, got %s", validUser.Name)
		}
		if validUser.Type.Kind != "object" {
			t.Errorf("validUser should be object, got %s", validUser.Type.Kind)
		}
		if len(validUser.Type.Properties) != 3 {
			t.Errorf("validUser should have 3 properties, got %d", len(validUser.Type.Properties))
		}
	}
}

func TestNestedObjectTSFormatting(t *testing.T) {
	typeStr := "{ complianceSettings: { minPasswordLength: number; minAge: number; requireMFA: boolean; }; testResults: { validUser: { input: string; result: string; errors: string[]; }; }; }"

	result := ParseTypeString(typeStr, "")

	c := &Contract{
		Name: "Result",
		Type: result,
	}

	ts := c.ToTypeScript()
	t.Logf("TypeScript output:\n%s", ts)

	// Verify multiline formatting
	if !strings.Contains(ts, "{\n") {
		t.Error("expected multiline object formatting with newlines")
	}

	// Check indentation exists
	if !strings.Contains(ts, "  complianceSettings:") {
		t.Error("expected indented complianceSettings property")
	}

	// Check nested object is also multiline
	if !strings.Contains(ts, "    minPasswordLength:") {
		t.Error("expected double-indented nested property")
	}
}
