package contract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAnalyzeSimpleInterface(t *testing.T) {
	code := `
		interface User {
			id: number;
			name: string;
			active: boolean;
		}
		const user: User = { id: 1, name: "Alice", active: true };
		export default user;
	`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if contract.Type == nil {
		t.Fatal("expected type definition")
	}

	if contract.Type.Kind != "object" {
		t.Errorf("expected object kind, got %s", contract.Type.Kind)
	}

	if contract.Type.Name != "User" {
		t.Errorf("expected User type, got %s", contract.Type.Name)
	}

	if len(contract.Type.Properties) != 3 {
		t.Errorf("expected 3 properties, got %d", len(contract.Type.Properties))
	}
}

func TestAnalyzeObjectLiteral(t *testing.T) {
	code := `
		export default {
			count: 42,
			message: "hello",
			enabled: true
		};
	`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if contract.Type.Kind != "object" {
		t.Errorf("expected object kind, got %s", contract.Type.Kind)
	}
}

func TestAnalyzeArray(t *testing.T) {
	code := `
		interface Item {
			id: number;
			name: string;
		}
		const items: Item[] = [{ id: 1, name: "One" }];
		export default items;
	`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if contract.Type.Kind != "array" {
		t.Errorf("expected array kind, got %s", contract.Type.Kind)
	}

	if contract.Type.ElementType == nil {
		t.Fatal("expected element type")
	}

	if contract.Type.ElementType.Name != "Item" {
		t.Errorf("expected Item element type, got %s", contract.Type.ElementType.Name)
	}
}

func TestAnalyzeUnionType(t *testing.T) {
	code := `
		type Status = "pending" | "active" | "completed";
		const status: Status = "active";
		export default status;
	`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if contract.Type.Kind != "union" {
		t.Errorf("expected union kind, got %s", contract.Type.Kind)
	}

	if len(contract.Type.UnionTypes) != 3 {
		t.Errorf("expected 3 union types, got %d", len(contract.Type.UnionTypes))
	}
}

func TestAnalyzeNullableType(t *testing.T) {
	code := `
		const value: string | null = "hello";
		export default value;
	`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !contract.Type.Nullable {
		t.Error("expected nullable type")
	}
}

func TestAnalyzeOptionalProperties(t *testing.T) {
	code := `
		interface Config {
			name: string;
			debug?: boolean;
			timeout?: number;
		}
		const config: Config = { name: "app" };
		export default config;
	`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var requiredCount, optionalCount int
	for _, prop := range contract.Type.Properties {
		if prop.Required {
			requiredCount++
		} else {
			optionalCount++
		}
	}

	if requiredCount != 1 {
		t.Errorf("expected 1 required property, got %d", requiredCount)
	}
	if optionalCount != 2 {
		t.Errorf("expected 2 optional properties, got %d", optionalCount)
	}
}

func TestAnalyzeWithDeclaredInputs(t *testing.T) {
	code := `
		declare const userId: number;
		declare const userName: string;
		
		export default { id: userId, name: userName };
	`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(contract.Inputs) != 2 {
		t.Errorf("expected 2 inputs, got %d", len(contract.Inputs))
	}

	if len(contract.Inputs) >= 2 {
		if contract.Inputs[0].Name != "userId" {
			t.Errorf("expected userId first, got %s", contract.Inputs[0].Name)
		}
		if contract.Inputs[1].Name != "userName" {
			t.Errorf("expected userName second, got %s", contract.Inputs[1].Name)
		}
	}
}

func TestToTypeScript(t *testing.T) {
	code := `
		interface User {
			id: number;
			name: string;
		}
		const user: User = { id: 1, name: "Alice" };
		export default user;
	`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ts := contract.ToTypeScript()

	if !strings.Contains(ts, "export type Result") {
		t.Error("expected export type Result in TypeScript output")
	}
}

func TestToJSONSchema(t *testing.T) {
	code := `
		interface User {
			id: number;
			name: string;
			active: boolean;
		}
		const user: User = { id: 1, name: "Alice", active: true };
		export default user;
	`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	schema := contract.ToJSONSchema()

	if schema.Schema != "https://json-schema.org/draft/2020-12/schema" {
		t.Errorf("expected JSON Schema 2020-12, got %s", schema.Schema)
	}

	if schema.Title != "Result" {
		t.Errorf("expected title Result, got %s", schema.Title)
	}

	if schema.Properties == nil || schema.Properties["result"] == nil {
		t.Fatal("expected result property in schema")
	}
}

func TestToJSON(t *testing.T) {
	code := `
		interface Config {
			enabled: boolean;
			count: number;
		}
		const config: Config = { enabled: true, count: 5 };
		export default config;
	`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	jsonBytes, err := contract.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if parsed["name"] != "Result" {
		t.Errorf("expected name Result, got %v", parsed["name"])
	}
}

func TestToJSONSchemaJSON(t *testing.T) {
	code := `
		export default { value: 42 };
	`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	jsonBytes, err := contract.ToJSONSchemaJSON()
	if err != nil {
		t.Fatalf("ToJSONSchemaJSON error: %v", err)
	}

	var schema JSONSchema
	if err := json.Unmarshal(jsonBytes, &schema); err != nil {
		t.Fatalf("invalid JSON Schema: %v", err)
	}

	if schema.Schema == "" {
		t.Error("expected $schema in output")
	}
}

func TestAnalyzePrimitiveExport(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected string
	}{
		{
			name:     "string literal",
			code:     `export default "hello";`,
			expected: "string",
		},
		{
			name:     "number literal",
			code:     `export default 42;`,
			expected: "number",
		},
		{
			name:     "boolean literal",
			code:     `export default true;`,
			expected: "boolean",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := NewAnalyzer()
			contract, err := analyzer.Analyze(tt.code)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if contract.Type.Name != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, contract.Type.Name)
			}
		})
	}
}

func TestAnalyzeGenericArray(t *testing.T) {
	code := `
		const numbers: Array<number> = [1, 2, 3];
		export default numbers;
	`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if contract.Type.Kind != "array" {
		t.Errorf("expected array kind, got %s", contract.Type.Kind)
	}

	if contract.Type.ElementType.Name != "number" {
		t.Errorf("expected number element type, got %s", contract.Type.ElementType.Name)
	}
}

func TestTypeDefToTSComplex(t *testing.T) {
	typeDef := &TypeDef{
		Kind: "object",
		Properties: []Property{
			{Name: "id", Type: &TypeDef{Kind: "primitive", Name: "number"}, Required: true},
			{Name: "tags", Type: &TypeDef{Kind: "array", ElementType: &TypeDef{Kind: "primitive", Name: "string"}}, Required: false},
		},
	}

	ts := typeDefToTS(typeDef)

	if !strings.Contains(ts, "id: number") {
		t.Error("expected id: number in output")
	}
	if !strings.Contains(ts, "tags?: string[]") {
		t.Error("expected tags?: string[] in output")
	}
}

func TestJSONSchemaArrayType(t *testing.T) {
	code := `
		const items: string[] = ["a", "b", "c"];
		export default items;
	`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	schema := contract.ToJSONSchema()
	result := schema.Properties["result"]

	if result.Type != "array" {
		t.Errorf("expected array type, got %s", result.Type)
	}

	if result.Items == nil || result.Items.Type != "string" {
		t.Error("expected items with string type")
	}
}

func TestJSONSchemaNullableType(t *testing.T) {
	code := `
		const value: number | null = null;
		export default value;
	`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	schema := contract.ToJSONSchema()
	result := schema.Properties["result"]

	if result.AnyOf == nil {
		t.Fatal("expected anyOf for nullable type")
	}

	hasNull := false
	for _, s := range result.AnyOf {
		if s.Type == "null" {
			hasNull = true
			break
		}
	}
	if !hasNull {
		t.Error("expected null type in anyOf")
	}
}

func TestAnalyzeComparisonExpression(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{"strict equality", "1 === 2"},
		{"strict inequality", "1 !== 2"},
		{"loose equality", "1 == 2"},
		{"loose inequality", "1 != 2"},
		{"less than", "1 < 2"},
		{"greater than", "1 > 2"},
		{"less or equal", "1 <= 2"},
		{"greater or equal", "1 >= 2"},
		{"with variables", "let a = '1';\nlet b = '2';\na === b"},
		{"exported comparison", "export default 1 === 2;"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := NewAnalyzer()
			contract, err := analyzer.Analyze(tt.code)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if contract.Type == nil {
				t.Fatal("expected type definition")
			}

			if contract.Type.Kind != "primitive" {
				t.Errorf("expected primitive kind, got %s", contract.Type.Kind)
			}

			if contract.Type.Name != "boolean" {
				t.Errorf("expected boolean type, got %s", contract.Type.Name)
			}
		})
	}
}

func TestAnalyzeLogicalExpression(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{"and operator", "true && false"},
		{"or operator", "true || false"},
		{"not operator", "!true"},
		{"complex logical", "a && b || c"},
		{"with comparison", "1 < 2 && 3 > 1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := NewAnalyzer()
			contract, err := analyzer.Analyze(tt.code)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if contract.Type == nil {
				t.Fatal("expected type definition")
			}

			if contract.Type.Kind != "primitive" {
				t.Errorf("expected primitive kind, got %s", contract.Type.Kind)
			}

			if contract.Type.Name != "boolean" {
				t.Errorf("expected boolean type, got %s", contract.Type.Name)
			}
		})
	}
}

func TestAnalyzeTrailingExpression(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		wantKind string
		wantName string
	}{
		{
			name:     "trailing number",
			code:     "const x = 1;\n42",
			wantKind: "primitive",
			wantName: "number",
		},
		{
			name:     "trailing string",
			code:     "const x = 1;\n\"hello\"",
			wantKind: "primitive",
			wantName: "string",
		},
		{
			name:     "trailing boolean",
			code:     "const x = 1;\ntrue",
			wantKind: "primitive",
			wantName: "boolean",
		},
		{
			name:     "trailing comparison",
			code:     "const a = 1;\nconst b = 2;\na === b",
			wantKind: "primitive",
			wantName: "boolean",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := NewAnalyzer()
			contract, err := analyzer.Analyze(tt.code)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if contract.Type == nil {
				t.Fatal("expected type definition")
			}

			if contract.Type.Kind != tt.wantKind {
				t.Errorf("expected %s kind, got %s", tt.wantKind, contract.Type.Kind)
			}

			if contract.Type.Name != tt.wantName {
				t.Errorf("expected %s type, got %s", tt.wantName, contract.Type.Name)
			}
		})
	}
}

func TestAnalyzeShorthandProperties(t *testing.T) {
	code := `
		declare const currentUser: {
			id: number;
			name: string;
			email: string;
			role: string;
		};
		
		export default { a: 'abc', b: 123, currentUser };
	`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if contract.Type == nil {
		t.Fatal("expected type definition")
	}

	if contract.Type.Kind != "object" {
		t.Errorf("expected object kind, got %s", contract.Type.Kind)
	}

	if len(contract.Type.Properties) != 3 {
		t.Errorf("expected 3 properties, got %d", len(contract.Type.Properties))
	}

	// Check each property
	propMap := make(map[string]*TypeDef)
	for _, p := range contract.Type.Properties {
		propMap[p.Name] = p.Type
	}

	// Check 'a' is string
	if a, ok := propMap["a"]; !ok {
		t.Error("expected property 'a'")
	} else if a.Name != "string" {
		t.Errorf("expected 'a' to be string, got %s", a.Name)
	}

	// Check 'b' is number
	if b, ok := propMap["b"]; !ok {
		t.Error("expected property 'b'")
	} else if b.Name != "number" {
		t.Errorf("expected 'b' to be number, got %s", b.Name)
	}

	// Check 'currentUser' is an object with the right properties
	if cu, ok := propMap["currentUser"]; !ok {
		t.Error("expected property 'currentUser'")
	} else {
		if cu.Kind != "object" {
			t.Errorf("expected 'currentUser' to be object, got %s", cu.Kind)
		}
		if len(cu.Properties) != 4 {
			t.Errorf("expected 'currentUser' to have 4 properties, got %d", len(cu.Properties))
		}
	}
}

func TestAnalyzeShorthandWithInterface(t *testing.T) {
	code := `
		interface User {
			id: number;
			name: string;
		}
		declare const user: User;
		
		export default { data: 'test', user };
	`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(contract.Type.Properties) != 2 {
		t.Errorf("expected 2 properties, got %d", len(contract.Type.Properties))
	}

	// Find user property
	var userProp *Property
	for i := range contract.Type.Properties {
		if contract.Type.Properties[i].Name == "user" {
			userProp = &contract.Type.Properties[i]
			break
		}
	}

	if userProp == nil {
		t.Fatal("expected 'user' property")
	}

	if userProp.Type.Name != "User" {
		t.Errorf("expected User type, got %s", userProp.Type.Name)
	}
}

func TestAnalyzeWithPreRegisteredGlobals(t *testing.T) {
	// Code without type declarations - relies on pre-registered globals
	code := `export default { a: 'abc', b: 123, currentUser, config };`

	analyzer := NewAnalyzer()

	// Pre-register interfaces
	analyzer.AddInterface("User", map[string]string{
		"id":    "number",
		"name":  "string",
		"email": "string",
		"role":  "string",
	})
	analyzer.AddInterface("Config", map[string]string{
		"apiUrl":  "string",
		"timeout": "number",
		"debug":   "boolean",
	})

	// Pre-register globals
	analyzer.AddGlobalFromTypeString("currentUser", "User")
	analyzer.AddGlobalFromTypeString("config", "Config")

	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if contract.Type == nil {
		t.Fatal("expected type definition")
	}

	if len(contract.Type.Properties) != 4 {
		t.Errorf("expected 4 properties, got %d", len(contract.Type.Properties))
	}

	// Check each property
	propMap := make(map[string]*TypeDef)
	for _, p := range contract.Type.Properties {
		propMap[p.Name] = p.Type
	}

	// Check 'a' is string
	if a, ok := propMap["a"]; !ok {
		t.Error("expected property 'a'")
	} else if a.Name != "string" {
		t.Errorf("expected 'a' to be string, got %s", a.Name)
	}

	// Check 'b' is number
	if b, ok := propMap["b"]; !ok {
		t.Error("expected property 'b'")
	} else if b.Name != "number" {
		t.Errorf("expected 'b' to be number, got %s", b.Name)
	}

	// Check 'currentUser' references User interface
	if cu, ok := propMap["currentUser"]; !ok {
		t.Error("expected property 'currentUser'")
	} else {
		if cu.Name != "User" {
			t.Errorf("expected 'currentUser' to be User, got %s (kind: %s)", cu.Name, cu.Kind)
		}
	}

	// Check 'config' references Config interface
	if cfg, ok := propMap["config"]; !ok {
		t.Error("expected property 'config'")
	} else {
		if cfg.Name != "Config" {
			t.Errorf("expected 'config' to be Config, got %s (kind: %s)", cfg.Name, cfg.Kind)
		}
	}

	// Generate TypeScript and verify it includes full types (not 'any')
	ts := contract.ToTypeScript()
	// The types should be expanded, not just "any"
	if strings.Contains(ts, "currentUser: any") {
		t.Errorf("expected TypeScript to NOT contain 'currentUser: any', got:\n%s", ts)
	}
	if strings.Contains(ts, "config: any") {
		t.Errorf("expected TypeScript to NOT contain 'config: any', got:\n%s", ts)
	}
	// Should contain the expanded properties from the interfaces
	if !strings.Contains(ts, "id: number") {
		t.Errorf("expected TypeScript to contain 'id: number' from User interface, got:\n%s", ts)
	}
	if !strings.Contains(ts, "apiUrl: string") {
		t.Errorf("expected TypeScript to contain 'apiUrl: string' from Config interface, got:\n%s", ts)
	}
}

func TestAnalyzeMemberAccessExpression(t *testing.T) {
	// Code that accesses properties of globals
	code := `export default { a: 'abc', b: 123, currentUser, url: config.apiUrl };`

	analyzer := NewAnalyzer()

	// Pre-register interfaces
	analyzer.AddInterface("User", map[string]string{
		"id":    "number",
		"name":  "string",
		"email": "string",
		"role":  "string",
	})
	analyzer.AddInterface("Config", map[string]string{
		"apiUrl":  "string",
		"timeout": "number",
		"debug":   "boolean",
	})

	// Pre-register globals
	analyzer.AddGlobalFromTypeString("currentUser", "User")
	analyzer.AddGlobalFromTypeString("config", "Config")

	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if contract.Type == nil {
		t.Fatal("expected type definition")
	}

	if len(contract.Type.Properties) != 4 {
		t.Errorf("expected 4 properties, got %d", len(contract.Type.Properties))
	}

	// Check each property
	propMap := make(map[string]*TypeDef)
	for _, p := range contract.Type.Properties {
		propMap[p.Name] = p.Type
	}

	// Check 'url' is string (from config.apiUrl)
	if url, ok := propMap["url"]; !ok {
		t.Error("expected property 'url'")
	} else {
		if url.Kind != "primitive" || url.Name != "string" {
			t.Errorf("expected 'url' to be string, got %s (kind: %s)", url.Name, url.Kind)
		}
	}

	// Generate TypeScript and verify url is string, not any
	ts := contract.ToTypeScript()
	if strings.Contains(ts, "url: any") {
		t.Errorf("expected TypeScript to NOT contain 'url: any', got:\n%s", ts)
	}
	if !strings.Contains(ts, "url: string") {
		t.Errorf("expected TypeScript to contain 'url: string', got:\n%s", ts)
	}
}
