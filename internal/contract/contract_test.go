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

	// Nullable types are now represented as union types with null member
	if contract.Type.Kind != "union" {
		t.Errorf("expected union type, got %s", contract.Type.Kind)
	}
	if len(contract.Type.UnionTypes) != 2 {
		t.Errorf("expected 2 union types, got %d", len(contract.Type.UnionTypes))
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

func TestAnalyzeWithDeclaredGlobals(t *testing.T) {
	// Note: declare const/var/let statements are ambient type declarations in TypeScript.
	// They tell the compiler a global exists at runtime but don't create it.
	// These are used for typing built-in globals (like Bun, process, Buffer).
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

	// Contract should still be created successfully with object type
	if contract.Type == nil {
		t.Fatal("expected type definition")
	}
	if contract.Type.Kind != "object" {
		t.Errorf("expected object type, got %s", contract.Type.Kind)
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

func TestAnalyzeAsyncFunctionExport(t *testing.T) {
	tests := []struct {
		name         string
		code         string
		expectedKind string
		expectedName string
	}{
		{
			name:         "async arrow function with Promise return type",
			code:         `export default async (): Promise<string> => { return await 'abc'; };`,
			expectedKind: "function",
			expectedName: "() => string",
		},
		{
			name:         "async arrow function with Promise object return",
			code:         `export default async (): Promise<{ status: string }> => { return { status: 'ok' }; };`,
			expectedKind: "function",
			expectedName: "() => { status: string }",
		},
		{
			name:         "async function declaration",
			code:         `export default async function(): Promise<number> { return 42; };`,
			expectedKind: "function",
			expectedName: "() => number",
		},
		{
			name:         "sync arrow function with return type",
			code:         `export default (): string => { return 'abc'; };`,
			expectedKind: "function",
			expectedName: "() => string",
		},
		{
			name:         "sync function with return type",
			code:         `export default function(): boolean { return true; };`,
			expectedKind: "function",
			expectedName: "() => boolean",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			analyzer := NewAnalyzer()
			contract, err := analyzer.Analyze(tc.code)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if contract.Type == nil {
				t.Fatal("expected type definition")
			}

			if contract.Type.Kind != tc.expectedKind {
				t.Errorf("expected kind %s, got %s", tc.expectedKind, contract.Type.Kind)
			}

			if tc.expectedName != "" && contract.Type.Name != tc.expectedName {
				t.Errorf("expected name %s, got %s", tc.expectedName, contract.Type.Name)
			}
		})
	}
}

func TestAnalyzeAwaitExpressionExport(t *testing.T) {
	tests := []struct {
		name           string
		code           string
		expectKind     string
		expectName     string
		expectPropName string // if we expect a property in the type
		expectPropType string // expected type of the property
	}{
		{
			name: "await function call with explicit return type",
			code: `
async function getData(): Promise<{ status: string }> {
  return { status: "ok" };
}
export default await getData();
`,
			expectKind:     "object",
			expectPropName: "status",
		},
		{
			name: "await function call without explicit return type - infers from body",
			code: `
async function main() {
  return {
    message: "hello",
    count: 42,
  };
}
export default await main();
`,
			expectKind:     "object",
			expectPropName: "message",
		},
		{
			name: "await function returning primitive via Promise type",
			code: `
async function getNumber(): Promise<number> {
  return 42;
}
export default await getNumber();
`,
			expectKind: "primitive",
			expectName: "number",
		},
		{
			name: "member access through await-assigned variable with interface",
			code: `
interface Post {
  id: number;
  title: string;
}

interface PostDetails {
  post: Post;
  count: number;
}

async function getDetails(): Promise<PostDetails> {
  return { post: { id: 1, title: "Test" }, count: 5 };
}

async function main() {
  const details = await getDetails();
  return {
    postTitle: details.post.title,
  };
}

export default await main();
`,
			expectKind:     "object",
			expectPropName: "postTitle",
			expectPropType: "string",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			analyzer := NewAnalyzer()
			contract, err := analyzer.Analyze(tc.code)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if contract.Type == nil {
				t.Fatal("expected type definition")
			}

			if tc.expectKind != "" && contract.Type.Kind != tc.expectKind {
				t.Errorf("expected kind %s, got %s", tc.expectKind, contract.Type.Kind)
			}

			if tc.expectName != "" && contract.Type.Name != tc.expectName {
				t.Errorf("expected name %s, got %s", tc.expectName, contract.Type.Name)
			}

			if tc.expectPropName != "" {
				var foundProp *Property
				for i, prop := range contract.Type.Properties {
					if prop.Name == tc.expectPropName {
						foundProp = &contract.Type.Properties[i]
						break
					}
				}
				if foundProp == nil {
					t.Errorf("expected property %s not found in type %+v", tc.expectPropName, contract.Type)
				} else if tc.expectPropType != "" {
					if foundProp.Type == nil {
						t.Errorf("property %s has nil type", tc.expectPropName)
					} else if foundProp.Type.Name != tc.expectPropType {
						t.Errorf("expected property %s to have type %s, got %s", tc.expectPropName, tc.expectPropType, foundProp.Type.Name)
					}
				}
			}
		})
	}
}

// TestAnalyzeInferFromInitializer tests type inference for variables without type annotations
func TestAnalyzeInferFromInitializer(t *testing.T) {
	// Similar to hello-world sample: context has types, main code uses them without annotations
	code := `
		interface User {
			id: number;
			name: string;
			role: 'admin' | 'user' | 'guest';
		}

		interface Config {
			apiUrl: string;
			timeout: number;
		}

		export const currentUser: User = {
			id: 1,
			name: "John Doe",
			role: "admin"
		};

		export const config: Config = {
			apiUrl: "https://api.example.com",
			timeout: 5000
		};

		export function sum(x: number, y: number): number {
			return x + y;
		}

		// Main code - uses above context
		const user: User = currentUser;
		const greeting = ` + "`Hello, ${user.name}!`" + `;
		const apiEndpoint = config.apiUrl;
		const result = sum(10, 5);

		export default {
			greeting,
			userId: user.id,
			userRole: user.role,
			apiEndpoint,
			calculatedSum: result,
		};
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

	// Check each property
	propMap := make(map[string]*TypeDef)
	for _, p := range contract.Type.Properties {
		propMap[p.Name] = p.Type
	}

	// Check 'greeting' is string (inferred from template literal)
	if greeting, ok := propMap["greeting"]; !ok {
		t.Error("expected property 'greeting'")
	} else if greeting.Name != "string" {
		t.Errorf("expected 'greeting' to be string, got %s (kind: %s)", greeting.Name, greeting.Kind)
	}

	// Check 'userId' is number (from user.id)
	if userId, ok := propMap["userId"]; !ok {
		t.Error("expected property 'userId'")
	} else if userId.Name != "number" {
		t.Errorf("expected 'userId' to be number, got %s (kind: %s)", userId.Name, userId.Kind)
	}

	// Check 'userRole' is union type
	if userRole, ok := propMap["userRole"]; !ok {
		t.Error("expected property 'userRole'")
	} else if userRole.Kind != "union" {
		t.Errorf("expected 'userRole' to be union, got kind: %s, name: %s", userRole.Kind, userRole.Name)
	}

	// Check 'apiEndpoint' is string (from config.apiUrl)
	if apiEndpoint, ok := propMap["apiEndpoint"]; !ok {
		t.Error("expected property 'apiEndpoint'")
	} else if apiEndpoint.Name != "string" {
		t.Errorf("expected 'apiEndpoint' to be string, got %s (kind: %s)", apiEndpoint.Name, apiEndpoint.Kind)
	}

	// Check 'calculatedSum' is number (from sum() return type)
	if calcSum, ok := propMap["calculatedSum"]; !ok {
		t.Error("expected property 'calculatedSum'")
	} else if calcSum.Name != "number" {
		t.Errorf("expected 'calculatedSum' to be number, got %s (kind: %s)", calcSum.Name, calcSum.Kind)
	}

	// Verify TypeScript output doesn't contain 'any'
	ts := contract.ToTypeScript()
	if strings.Contains(ts, "greeting: any") {
		t.Errorf("greeting should not be 'any', got:\n%s", ts)
	}
	if strings.Contains(ts, "apiEndpoint: any") {
		t.Errorf("apiEndpoint should not be 'any', got:\n%s", ts)
	}
	if strings.Contains(ts, "calculatedSum: any") {
		t.Errorf("calculatedSum should not be 'any', got:\n%s", ts)
	}
}

// TestAnalyzeArithmeticExpression tests type inference for arithmetic expressions
func TestAnalyzeArithmeticExpression(t *testing.T) {
	code := `
		export function calculateCompoundGrowth(principal: number, rate: number): number {
			return principal * Math.pow(1 + rate, 12);
		}

		const investmentGrowth = calculateCompoundGrowth(10000, 0.07 / 12);

		export default {
			simple: 10 + 5,
			multiply: 100 * 2,
			divide: 200 / 4,
			modulo: 17 % 5,
			complex: 10 + 5 * 2,
			withParens: (10 + 5) * 2,
			mathRound: Math.round(3.7),
			mathFloor: Math.floor(3.9),
			mathCeil: Math.ceil(3.1),
			mathAbs: Math.abs(-5),
			mathPow: Math.pow(2, 8),
			roundedCalc: Math.round(investmentGrowth * 100) / 100,
		};
	`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if contract.Type == nil {
		t.Fatal("expected type definition")
	}

	// Check each property
	propMap := make(map[string]*TypeDef)
	for _, p := range contract.Type.Properties {
		propMap[p.Name] = p.Type
	}

	// All properties should be number
	expectedNumberProps := []string{
		"simple", "multiply", "divide", "modulo", "complex", "withParens",
		"mathRound", "mathFloor", "mathCeil", "mathAbs", "mathPow", "roundedCalc",
	}

	for _, propName := range expectedNumberProps {
		prop, ok := propMap[propName]
		if !ok {
			t.Errorf("expected property '%s'", propName)
			continue
		}
		if prop.Name != "number" {
			t.Errorf("expected '%s' to be number, got %s (kind: %s)", propName, prop.Name, prop.Kind)
		}
	}

	// Verify TypeScript output
	ts := contract.ToTypeScript()
	for _, propName := range expectedNumberProps {
		if strings.Contains(ts, propName+": any") {
			t.Errorf("%s should not be 'any', got:\n%s", propName, ts)
		}
	}
}

func TestAnalyzeObjectWithArrowCallbacks(t *testing.T) {
	// Regression test: object literals containing arrow function callbacks
	// should not be mistaken for comparison expressions (=> contains >)
	code := `
interface Product {
  id: number;
  name: string;
  price: number;
  category: string;
  inStock: boolean;
}

const products: Product[] = [];
const byCategory: Record<string, Product[]> = {};

export default {
  productNames: products.map(p => p.name),
  categories: Object.keys(byCategory).map(cat => ({
    name: cat,
    count: byCategory[cat].length,
  })),
  filtered: products.filter(p => p.inStock),
};
`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be an object, not boolean
	if contract.Type == nil {
		t.Fatal("expected type definition")
	}
	if contract.Type.Kind != "object" {
		t.Errorf("expected object type, got %s", contract.Type.Kind)
	}
	if contract.Type.Name == "boolean" {
		t.Error("type should not be boolean - arrow functions contain > but are not comparisons")
	}

	// Should have the expected properties
	expectedProps := []string{"productNames", "categories", "filtered"}
	propMap := make(map[string]bool)
	for _, p := range contract.Type.Properties {
		propMap[p.Name] = true
	}
	for _, name := range expectedProps {
		if !propMap[name] {
			t.Errorf("expected property %s", name)
		}
	}
}

func TestAnalyzeBuiltinMethodTypes(t *testing.T) {
	// Test inference of built-in JavaScript method return types
	code := `
interface Product {
  id: number;
  name: string;
  price: number;
  category: string;
  inStock: boolean;
}

const products: Product[] = [];
const byCategory: Record<string, Product[]> = {};
const price = 99.99;
const text = "hello";

export default {
  productNames: products.map(p => p.name),
  productPrices: products.map(p => p.price),
  filtered: products.filter(p => p.inStock),
  hasItems: products.some(p => p.inStock),
  allInStock: products.every(p => p.inStock),
  itemCount: products.length,
  priceStr: price.toFixed(2),
  upper: text.toUpperCase(),
  parts: text.split(","),
  hasPrefix: text.startsWith("h"),
  index: text.indexOf("e"),
  keys: Object.keys(byCategory),
  categories: Object.keys(byCategory).map(cat => ({
    name: cat,
    count: byCategory[cat].length,
  })),
  label: "Price: " + price,
  suffix: price + "%",
  json: JSON.stringify(products),
};
`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if contract.Type == nil {
		t.Fatal("expected type definition")
	}

	// Build property map
	propMap := make(map[string]*TypeDef)
	for _, p := range contract.Type.Properties {
		propMap[p.Name] = p.Type
	}

	// Check specific type inferences
	tests := []struct {
		prop     string
		wantKind string
		wantName string
	}{
		// Array map returns typed arrays
		{"productNames", "array", ""},
		{"productPrices", "array", ""},
		{"filtered", "array", ""},

		// Boolean methods
		{"hasItems", "primitive", "boolean"},
		{"allInStock", "primitive", "boolean"},
		{"hasPrefix", "primitive", "boolean"},

		// Number properties/methods
		{"itemCount", "primitive", "number"},
		{"index", "primitive", "number"},

		// String methods
		{"priceStr", "primitive", "string"},
		{"upper", "primitive", "string"},
		{"json", "primitive", "string"},

		// String arrays
		{"keys", "array", ""},
		{"parts", "array", ""},

		// String concatenation
		{"label", "primitive", "string"},
		{"suffix", "primitive", "string"},

		// Nested object array
		{"categories", "array", ""},
	}

	for _, tc := range tests {
		prop, ok := propMap[tc.prop]
		if !ok {
			t.Errorf("missing property: %s", tc.prop)
			continue
		}
		if prop.Kind != tc.wantKind {
			t.Errorf("%s: want kind %s, got %s", tc.prop, tc.wantKind, prop.Kind)
		}
		if tc.wantName != "" && prop.Name != tc.wantName {
			t.Errorf("%s: want name %s, got %s", tc.prop, tc.wantName, prop.Name)
		}
	}

	// Check array element types
	if prop, ok := propMap["productNames"]; ok && prop.ElementType != nil {
		if prop.ElementType.Name != "string" {
			t.Errorf("productNames element type: want string, got %s", prop.ElementType.Name)
		}
	}
	if prop, ok := propMap["productPrices"]; ok && prop.ElementType != nil {
		if prop.ElementType.Name != "number" {
			t.Errorf("productPrices element type: want number, got %s", prop.ElementType.Name)
		}
	}
	if prop, ok := propMap["keys"]; ok && prop.ElementType != nil {
		if prop.ElementType.Name != "string" {
			t.Errorf("keys element type: want string, got %s", prop.ElementType.Name)
		}
	}
	if prop, ok := propMap["parts"]; ok && prop.ElementType != nil {
		if prop.ElementType.Name != "string" {
			t.Errorf("parts element type: want string, got %s", prop.ElementType.Name)
		}
	}

	// Check categories has object element type with properties
	if prop, ok := propMap["categories"]; ok && prop.ElementType != nil {
		if prop.ElementType.Kind != "object" {
			t.Errorf("categories element type: want object, got %s", prop.ElementType.Kind)
		}
		// Check the nested object has name and count properties
		elemProps := make(map[string]string)
		for _, p := range prop.ElementType.Properties {
			elemProps[p.Name] = p.Type.Name
		}
		if elemProps["name"] != "string" {
			t.Errorf("categories[].name: want string, got %s", elemProps["name"])
		}
		if elemProps["count"] != "number" {
			t.Errorf("categories[].count: want number, got %s", elemProps["count"])
		}
	}
}

func TestAnalyzeTernaryExpression(t *testing.T) {
	code := `
interface ValidationResult {
  valid: boolean;
  errors: string[];
}

const result: ValidationResult = {
  valid: true,
  errors: [],
};

export default {
  message: result.valid ? "Success" : "Failed",
  status: result.errors.length === 0 ? "OK" : "Error",
  count: result.valid ? 1 : 0,
};
`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Build property map
	propMap := make(map[string]*TypeDef)
	for _, p := range contract.Type.Properties {
		propMap[p.Name] = p.Type
	}

	// Ternary with string literals should be string
	if prop, ok := propMap["message"]; ok {
		if prop.Name != "string" {
			t.Errorf("message: want string, got %s", prop.Name)
		}
	} else {
		t.Error("message property not found")
	}

	// Ternary with string literals should be string
	if prop, ok := propMap["status"]; ok {
		if prop.Name != "string" {
			t.Errorf("status: want string, got %s", prop.Name)
		}
	} else {
		t.Error("status property not found")
	}

	// Ternary with number literals should be number
	if prop, ok := propMap["count"]; ok {
		if prop.Name != "number" {
			t.Errorf("count: want number, got %s", prop.Name)
		}
	} else {
		t.Error("count property not found")
	}
}

func TestAnalyzePartialType(t *testing.T) {
	code := `
interface User {
  id: number;
  name: string;
  email: string;
}

const fullUser: User = {
  id: 1,
  name: "John",
  email: "john@example.com",
};

const partialUser: Partial<User> = {
  name: "Jane",
};

export default {
  fullId: fullUser.id,
  fullName: fullUser.name,
  partialName: partialUser.name,
};
`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Build property map
	propMap := make(map[string]*TypeDef)
	for _, p := range contract.Type.Properties {
		propMap[p.Name] = p.Type
	}

	// Properties from full User type
	if prop, ok := propMap["fullId"]; ok {
		if prop.Name != "number" {
			t.Errorf("fullId: want number, got %s", prop.Name)
		}
	} else {
		t.Error("fullId property not found")
	}

	if prop, ok := propMap["fullName"]; ok {
		if prop.Name != "string" {
			t.Errorf("fullName: want string, got %s", prop.Name)
		}
	} else {
		t.Error("fullName property not found")
	}

	// Properties from Partial<User> type should also work
	if prop, ok := propMap["partialName"]; ok {
		if prop.Name != "string" {
			t.Errorf("partialName: want string, got %s", prop.Name)
		}
	} else {
		t.Error("partialName property not found")
	}
}

func TestAnalyzeInlineObjectReturnType(t *testing.T) {
	code := `
interface Order {
  id: string;
  state: string;
}

// Function with inline object return type
function processOrder(order: Order, action: string): { success: boolean; order: Order; error?: string } {
  return { success: true, order, error: undefined };
}

const order: Order = { id: "123", state: "pending" };
const result = processOrder(order, "confirm");

export default {
  success: result.success,
  errorMessage: result.error,
  orderId: result.order.id,
};
`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Build property map
	propMap := make(map[string]*TypeDef)
	for _, p := range contract.Type.Properties {
		propMap[p.Name] = p.Type
	}

	// success should be boolean
	if prop, ok := propMap["success"]; ok {
		if prop.Name != "boolean" {
			t.Errorf("success: want boolean, got %s", prop.Name)
		}
	} else {
		t.Error("success property not found")
	}

	// errorMessage should be string (from error?: string)
	if prop, ok := propMap["errorMessage"]; ok {
		if prop.Name != "string" {
			t.Errorf("errorMessage: want string, got %s", prop.Name)
		}
	} else {
		t.Error("errorMessage property not found")
	}

	// orderId should be string (from result.order.id)
	if prop, ok := propMap["orderId"]; ok {
		if prop.Name != "string" {
			t.Errorf("orderId: want string, got %s", prop.Name)
		}
	} else {
		t.Error("orderId property not found")
	}
}

func TestAnalyzeNestedInterfaceInType(t *testing.T) {
	// Test that interfaces with nested objects in their types are parsed correctly
	code := `
interface Config {
  carriers: Record<string, { name: string; rate: number }>;
  defaultCarrier: string;
  threshold: number;
}

export const config: Config = {
  carriers: {},
  defaultCarrier: 'standard',
  threshold: 100
};

export default {
  carrier: config.defaultCarrier,
  limit: config.threshold,
};
`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Build property map
	propMap := make(map[string]*TypeDef)
	for _, p := range contract.Type.Properties {
		propMap[p.Name] = p.Type
	}

	// carrier should be string
	if prop, ok := propMap["carrier"]; ok {
		if prop.Name != "string" {
			t.Errorf("carrier: want string, got %s", prop.Name)
		}
	} else {
		t.Error("carrier property not found")
	}

	// limit should be number
	if prop, ok := propMap["limit"]; ok {
		if prop.Name != "number" {
			t.Errorf("limit: want number, got %s", prop.Name)
		}
	} else {
		t.Error("limit property not found")
	}
}

func TestAnalyzeTypeAliasExpansion(t *testing.T) {
	// Test that type aliases (especially union types) are expanded inline
	code := `
type OrderState = 
  | "pending"
  | "confirmed"
  | "shipped"
  | "delivered";

interface Order {
  id: string;
  state: OrderState;
}

const order: Order = {
  id: "123",
  state: "delivered",
};

export default {
  finalState: order.state,
};
`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Build property map
	propMap := make(map[string]*TypeDef)
	for _, p := range contract.Type.Properties {
		propMap[p.Name] = p.Type
	}

	// finalState should be a union type (expanded from OrderState)
	if prop, ok := propMap["finalState"]; ok {
		if prop.Kind != "union" {
			t.Errorf("finalState: want kind union, got %s", prop.Kind)
		}
		if len(prop.UnionTypes) != 4 {
			t.Errorf("finalState: want 4 union types, got %d", len(prop.UnionTypes))
		}
	} else {
		t.Error("finalState property not found")
	}

	// Check the formatted output includes the union type
	output := typeDefToTSFormatted(contract.Type, 0)
	if !strings.Contains(output, `"pending"`) || !strings.Contains(output, `"delivered"`) {
		t.Error("Expected type alias to be expanded in output")
	}
}

func TestAnalyzeInlineArrayMapCallback(t *testing.T) {
	// Test that .map() on inline array literals infers element type correctly
	code := `
const scenarios = [
  { name: "Test", tier: "gold", amount: 100 },
];

const results = scenarios.map(s => ({
  scenario: s.name,
  tier: s.tier,
}));

export default { results };
`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Build property map
	propMap := make(map[string]*TypeDef)
	for _, p := range contract.Type.Properties {
		propMap[p.Name] = p.Type
	}

	// results should be an array
	if prop, ok := propMap["results"]; ok {
		if prop.Kind != "array" {
			t.Errorf("results: want kind array, got %s", prop.Kind)
		}
		if prop.ElementType == nil {
			t.Fatal("results: expected element type")
		}

		// Element type should have scenario and tier properties
		elemProps := make(map[string]*TypeDef)
		for _, p := range prop.ElementType.Properties {
			elemProps[p.Name] = p.Type
		}

		if scenario, ok := elemProps["scenario"]; ok {
			if scenario.Kind != "primitive" || scenario.Name != "string" {
				t.Errorf("scenario: want string, got %s %s", scenario.Kind, scenario.Name)
			}
		} else {
			t.Error("scenario property not found in results element")
		}

		if tier, ok := elemProps["tier"]; ok {
			if tier.Kind != "primitive" || tier.Name != "string" {
				t.Errorf("tier: want string, got %s %s", tier.Kind, tier.Name)
			}
		} else {
			t.Error("tier property not found in results element")
		}
	} else {
		t.Error("results property not found")
	}
}

func TestAnalyzeIndexedRecordAccess(t *testing.T) {
	// Test that varName[key].property on Record<string, Type> resolves correctly
	code := `
interface LoyaltyTier {
  name: string;
  minPoints: number;
}

export const loyaltyTiers: Record<string, LoyaltyTier> = {
  gold: { name: 'Gold', minPoints: 5000 },
};

const scenarios = [{ tier: "gold" }];

const results = scenarios.map(s => ({
  tierName: loyaltyTiers[s.tier].name,
}));

export default { results };
`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Build property map
	propMap := make(map[string]*TypeDef)
	for _, p := range contract.Type.Properties {
		propMap[p.Name] = p.Type
	}

	// results should be an array with tierName: string
	if prop, ok := propMap["results"]; ok {
		if prop.Kind != "array" || prop.ElementType == nil {
			t.Fatal("results should be an array with element type")
		}

		elemProps := make(map[string]*TypeDef)
		for _, p := range prop.ElementType.Properties {
			elemProps[p.Name] = p.Type
		}

		if tierName, ok := elemProps["tierName"]; ok {
			if tierName.Kind != "primitive" || tierName.Name != "string" {
				t.Errorf("tierName: want string, got %s %s", tierName.Kind, tierName.Name)
			}
		} else {
			t.Error("tierName property not found")
		}
	} else {
		t.Error("results property not found")
	}
}

func TestAnalyzeObjectValuesMapCallback(t *testing.T) {
	// Test that Object.values(record).map() infers value type correctly
	code := `
interface LoyaltyTier {
  name: string;
  minPoints: number;
}

export const loyaltyTiers: Record<string, LoyaltyTier> = {
  gold: { name: 'Gold', minPoints: 5000 },
};

const tiers = Object.values(loyaltyTiers).map(t => ({
  tier: t.name,
  min: t.minPoints,
}));

export default { tiers };
`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	propMap := make(map[string]*TypeDef)
	for _, p := range contract.Type.Properties {
		propMap[p.Name] = p.Type
	}

	if prop, ok := propMap["tiers"]; ok {
		if prop.Kind != "array" || prop.ElementType == nil {
			t.Fatal("tiers should be an array")
		}

		elemProps := make(map[string]*TypeDef)
		for _, p := range prop.ElementType.Properties {
			elemProps[p.Name] = p.Type
		}

		if tier, ok := elemProps["tier"]; ok {
			if tier.Kind != "primitive" || tier.Name != "string" {
				t.Errorf("tier: want string, got %s %s", tier.Kind, tier.Name)
			}
		} else {
			t.Error("tier property not found")
		}

		if min, ok := elemProps["min"]; ok {
			if min.Kind != "primitive" || min.Name != "number" {
				t.Errorf("min: want number, got %s %s", min.Kind, min.Name)
			}
		} else {
			t.Error("min property not found")
		}
	} else {
		t.Error("tiers property not found")
	}
}

func TestAnalyzeReduceWithInitialValue(t *testing.T) {
	// Test that .reduce() with numeric initial value returns number
	code := `
const items = [{ amount: 100 }, { amount: 200 }];
const total = items.reduce((sum, item) => sum + item.amount, 0);
export default { total };
`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	propMap := make(map[string]*TypeDef)
	for _, p := range contract.Type.Properties {
		propMap[p.Name] = p.Type
	}

	if prop, ok := propMap["total"]; ok {
		if prop.Kind != "primitive" || prop.Name != "number" {
			t.Errorf("total: want number, got %s %s", prop.Kind, prop.Name)
		}
	} else {
		t.Error("total property not found")
	}
}

func TestAnalyzeCommentStripping(t *testing.T) {
	// Test that comments are stripped from object literals
	code := `
const config = {
  maxRules: 3, // Maximum number of rules
  maxDiscount: 40, /* Cap discount */
  enabled: true,
};

export default { config };
`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := typeDefToTSFormatted(contract.Type, 0)
	if strings.Contains(output, "//") || strings.Contains(output, "/*") {
		t.Error("Output should not contain comments")
	}
	if strings.Contains(output, "Maximum") || strings.Contains(output, "Cap") {
		t.Error("Output should not contain comment text")
	}

	// Verify properties have correct types
	propMap := make(map[string]*TypeDef)
	for _, p := range contract.Type.Properties {
		propMap[p.Name] = p.Type
	}

	if config, ok := propMap["config"]; ok {
		configProps := make(map[string]*TypeDef)
		for _, p := range config.Properties {
			configProps[p.Name] = p.Type
		}

		if maxRules, ok := configProps["maxRules"]; ok {
			if maxRules.Kind != "primitive" || maxRules.Name != "number" {
				t.Errorf("maxRules: want number, got %s %s", maxRules.Kind, maxRules.Name)
			}
		} else {
			t.Error("maxRules property not found")
		}
	} else {
		t.Error("config property not found")
	}
}

func TestAnalyzeInlineObjectTypeAnnotation(t *testing.T) {
	// Test that inline object type annotation on array is properly parsed
	code := `
interface PricingContext {
  cartTotal: number;
  itemCount: number;
}

const scenarios: { name: string; context: PricingContext }[] = [
  { name: "Test", context: { cartTotal: 100, itemCount: 5 } },
];

const results = scenarios.map(scenario => ({
  scenario: scenario.name,
  total: scenario.context.cartTotal,
}));

export default { results };
`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	propMap := make(map[string]*TypeDef)
	for _, p := range contract.Type.Properties {
		propMap[p.Name] = p.Type
	}

	if prop, ok := propMap["results"]; ok {
		if prop.Kind != "array" || prop.ElementType == nil {
			t.Fatal("results should be an array")
		}

		elemProps := make(map[string]*TypeDef)
		for _, p := range prop.ElementType.Properties {
			elemProps[p.Name] = p.Type
		}

		if scenario, ok := elemProps["scenario"]; ok {
			if scenario.Kind != "primitive" || scenario.Name != "string" {
				t.Errorf("scenario: want string, got %s %s", scenario.Kind, scenario.Name)
			}
		} else {
			t.Error("scenario property not found")
		}

		if total, ok := elemProps["total"]; ok {
			if total.Kind != "primitive" || total.Name != "number" {
				t.Errorf("total: want number, got %s %s", total.Kind, total.Name)
			}
		} else {
			t.Error("total property not found")
		}
	} else {
		t.Error("results property not found")
	}
}

func TestAnalyzeNestedPropertyAccess(t *testing.T) {
	// Test that nested property access scenario.context.cartTotal works correctly
	// when scenarios has inline object type annotation with nested interface references
	code := `
interface PricingContext {
  cartTotal: number;
  itemCount: number;
}

const scenarios: { name: string; context: PricingContext }[] = [
  { name: "Test", context: { cartTotal: 100, itemCount: 5 } },
];

const results = scenarios.map(scenario => ({
  scenario: scenario.name,
  total: scenario.context.cartTotal,
}));

export default { results };
`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	propMap := make(map[string]*TypeDef)
	for _, p := range contract.Type.Properties {
		propMap[p.Name] = p.Type
	}

	if prop, ok := propMap["results"]; ok {
		if prop.Kind != "array" || prop.ElementType == nil {
			t.Fatal("results should be an array")
		}

		elemProps := make(map[string]*TypeDef)
		for _, p := range prop.ElementType.Properties {
			elemProps[p.Name] = p.Type
		}

		// scenario should be string (from scenario.name)
		if scenario, ok := elemProps["scenario"]; ok {
			if scenario.Kind != "primitive" || scenario.Name != "string" {
				t.Errorf("scenario: want string, got %s %s", scenario.Kind, scenario.Name)
			}
		} else {
			t.Error("scenario property not found")
		}

		// total should be number (from scenario.context.cartTotal)
		if total, ok := elemProps["total"]; ok {
			if total.Kind != "primitive" || total.Name != "number" {
				t.Errorf("total: want number, got %s %s", total.Kind, total.Name)
			}
		} else {
			t.Error("total property not found")
		}
	} else {
		t.Error("results property not found")
	}

	// Also verify no 'any' types in output
	output := typeDefToTSFormatted(contract.Type, 0)
	if strings.Contains(output, ": any") {
		t.Errorf("Output should not contain 'any' types:\n%s", output)
	}
}

// TestAnalyzeAsyncFetchSample tests the async-fetch sample pattern
func TestAnalyzeAsyncFetchSample(t *testing.T) {
	// Simplified version of async-fetch.ts sample
	code := `
interface ApiConfig {
  baseUrl: string;
  timeout: number;
}

const apiConfig: ApiConfig = {
  baseUrl: "https://api.example.com",
  timeout: 5000,
};

async function main() {
  return {
    apiConfiguration: {
      baseUrl: apiConfig.baseUrl,
      timeout: apiConfig.timeout + 'ms',
    },
    stats: {
      postsLoaded: 5,
      averageTitleLength: Math.round(42.5),
    },
  };
}

export default await main();
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
		t.Errorf("expected object, got %s", contract.Type.Kind)
	}

	// Verify top-level properties exist
	propNames := make(map[string]bool)
	for _, p := range contract.Type.Properties {
		propNames[p.Name] = true
	}

	if !propNames["apiConfiguration"] {
		t.Error("expected apiConfiguration property")
	}
	if !propNames["stats"] {
		t.Error("expected stats property")
	}

	// Verify no 'any' types
	ts := contract.ToTypeScript()
	if strings.Contains(ts, ": any") || strings.Contains(ts, "any;") {
		t.Errorf("type should not contain 'any':\n%s", ts)
	}
}

func TestAnalyzeWorkerPoolNestedProperty(t *testing.T) {
	// Test nested property access like workerPool.retryPolicy.maxRetries
	code := `
interface WorkerPool {
  maxConcurrency: number;
  taskTimeout: number;
  retryPolicy: {
    maxRetries: number;
    backoffMs: number;
  };
}

export const workerPool: WorkerPool = {
  maxConcurrency: 4,
  taskTimeout: 10000,
  retryPolicy: {
    maxRetries: 3,
    backoffMs: 100
  }
};

export default {
  maxRetries: workerPool.retryPolicy.maxRetries,
};
`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Find maxRetries property
	var maxRetriesType *TypeDef
	for _, p := range contract.Type.Properties {
		if p.Name == "maxRetries" {
			maxRetriesType = p.Type
			break
		}
	}

	if maxRetriesType == nil {
		t.Fatal("maxRetries property not found")
	}

	if maxRetriesType.Kind != "primitive" || maxRetriesType.Name != "number" {
		t.Errorf("maxRetries: want number, got %s %s", maxRetriesType.Kind, maxRetriesType.Name)
	}
}

func TestAnalyzeDeclaredGlobalMethodCall(t *testing.T) {
	// Test that method calls on declared globals infer the return type correctly
	code := `
		declare const Bun: {
			version: string;
			nanoseconds(): bigint;
			hash: {
				md5(data: string): string;
				crc32(data: string): number;
			};
		};

		const timing = Bun.nanoseconds();
		const hash = Bun.hash.md5("test");
		const crc = Bun.hash.crc32("test");

		export default { timing, hash, crc };
	`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if contract.Type == nil || len(contract.Type.Properties) != 3 {
		t.Fatalf("expected 3 properties, got %v", contract.Type)
	}

	// Check timing property - should be bigint from Bun.nanoseconds()
	var timingType, hashType, crcType *TypeDef
	for _, prop := range contract.Type.Properties {
		switch prop.Name {
		case "timing":
			timingType = prop.Type
		case "hash":
			hashType = prop.Type
		case "crc":
			crcType = prop.Type
		}
	}

	if timingType == nil || timingType.Name != "bigint" {
		t.Errorf("timing: want bigint, got %v", timingType)
	}

	if hashType == nil || hashType.Name != "string" {
		t.Errorf("hash: want string, got %v", hashType)
	}

	if crcType == nil || crcType.Name != "number" {
		t.Errorf("crc: want number, got %v", crcType)
	}
}

func TestAnalyzeDeclaredGlobalMethodCallDirect(t *testing.T) {
	// Test direct export of method call result
	code := `
		declare const Bun: {
			nanoseconds(): bigint;
		};

		export default Bun.nanoseconds();
	`

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if contract.Type == nil {
		t.Fatal("expected type, got nil")
	}

	t.Logf("Got type: kind=%s name=%s", contract.Type.Kind, contract.Type.Name)

	if contract.Type.Kind != "primitive" || contract.Type.Name != "bigint" {
		t.Errorf("expected bigint, got kind=%s name=%s", contract.Type.Kind, contract.Type.Name)
	}
}

func TestAnalyzeDeclaredGlobalWithRealContext(t *testing.T) {
	// Simulate what happens with context + code in Monaco
	contextCode := `// Context: Bun runtime utilities (Bun only)

// Bun type declarations (these APIs exist at runtime)
declare const Bun: {
  version: string;
  revision: string;
  main: string;
  nanoseconds(): bigint;
  hash: {
    md5(data: string): string;
  };
};
`

	mainCode := `export default Bun.nanoseconds();`

	// This is how the server combines them
	fullCode := contextCode + "\n\n" + mainCode

	analyzer := NewAnalyzer()
	contract, err := analyzer.Analyze(fullCode)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if contract.Type == nil {
		t.Fatal("expected type, got nil")
	}

	t.Logf("Got type: kind=%s name=%s", contract.Type.Kind, contract.Type.Name)

	if contract.Type.Kind != "primitive" || contract.Type.Name != "bigint" {
		t.Errorf("expected bigint, got kind=%s name=%s", contract.Type.Kind, contract.Type.Name)
	}
}
