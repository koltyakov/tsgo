// Command typeinfer compares type inference between Bun-based TypeScript Compiler API
// and the pure Go regex-based contract analyzer.
package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/koltyakov/tsgo/internal/contract"
	"github.com/koltyakov/tsgo/internal/typeinfer"
)

// TestCase defines a type inference test.
type TestCase struct {
	Name         string
	Code         string
	ExpectedType string
	ExpectedKind string
}

// TestCategory groups related test cases.
type TestCategory struct {
	Name  string
	Tests []TestCase
}

var testCategories = []TestCategory{
	{
		Name: "Primitive Types",
		Tests: []TestCase{
			{Name: "number literal", Code: `export default 42`, ExpectedType: "number", ExpectedKind: "primitive"},
			{Name: "string literal", Code: `export default "hello"`, ExpectedType: "string", ExpectedKind: "primitive"},
			{Name: "boolean true", Code: `export default true`, ExpectedType: "boolean", ExpectedKind: "primitive"},
			{Name: "boolean false", Code: `export default false`, ExpectedType: "boolean", ExpectedKind: "primitive"},
			{Name: "null", Code: `export default null`, ExpectedType: "null", ExpectedKind: "literal"},
			{Name: "undefined", Code: `export default undefined`, ExpectedType: "undefined", ExpectedKind: "primitive"},
		},
	},
	{
		Name: "Literal Types",
		Tests: []TestCase{
			{Name: "number literal type", Code: `const x = 42 as const; export default x`, ExpectedType: "42", ExpectedKind: "literal"},
			{Name: "string literal type", Code: `const x = "hello" as const; export default x`, ExpectedType: `"hello"`, ExpectedKind: "literal"},
			{Name: "true literal", Code: `const x = true as const; export default x`, ExpectedType: "true", ExpectedKind: "literal"},
		},
	},
	{
		Name: "Type Annotations",
		Tests: []TestCase{
			{Name: "annotated number", Code: `const x: number = 42; export default x`, ExpectedType: "number", ExpectedKind: "primitive"},
			{Name: "annotated string", Code: `const x: string = "hi"; export default x`, ExpectedType: "string", ExpectedKind: "primitive"},
			{Name: "annotated boolean", Code: `const x: boolean = true; export default x`, ExpectedType: "boolean", ExpectedKind: "primitive"},
			{Name: "annotated any", Code: `const x: any = 42; export default x`, ExpectedType: "any", ExpectedKind: "any"},
		},
	},
	{
		Name: "Array Types",
		Tests: []TestCase{
			{Name: "number array", Code: `const arr: number[] = [1, 2, 3]; export default arr`, ExpectedType: "number[]", ExpectedKind: "array"},
			{Name: "string array", Code: `const arr: string[] = ["a", "b"]; export default arr`, ExpectedType: "string[]", ExpectedKind: "array"},
			{Name: "inferred array", Code: `export default [1, 2, 3]`, ExpectedType: "number[]", ExpectedKind: "array"},
			{Name: "mixed array", Code: `export default [1, "a", true]`, ExpectedType: "(string | number | boolean)[]", ExpectedKind: "array"},
			{Name: "empty array", Code: `export default []`, ExpectedType: "never[]", ExpectedKind: "array"},
			{Name: "Array generic", Code: `const arr: Array<number> = [1, 2]; export default arr`, ExpectedType: "number[]", ExpectedKind: "array"},
		},
	},
	{
		Name: "Object Types",
		Tests: []TestCase{
			{Name: "simple object", Code: `export default { a: 1, b: "x" }`, ExpectedType: "{ a: number; b: string }", ExpectedKind: "object"},
			{Name: "nested object", Code: `export default { inner: { val: 42 } }`, ExpectedType: "{ inner: { val: number } }", ExpectedKind: "object"},
			{Name: "typed object", Code: `const obj: { x: number } = { x: 1 }; export default obj`, ExpectedType: "{ x: number }", ExpectedKind: "object"},
			{Name: "optional property", Code: `const obj: { a: number; b?: string } = { a: 1 }; export default obj`, ExpectedType: "{ a: number; b?: string }", ExpectedKind: "object"},
			{Name: "empty object", Code: `export default {}`, ExpectedType: "{}", ExpectedKind: "object"},
		},
	},
	{
		Name: "Interface Types",
		Tests: []TestCase{
			{Name: "interface instance", Code: `interface User { id: number; name: string; } const u: User = { id: 1, name: "Alice" }; export default u`, ExpectedType: "User", ExpectedKind: "object"},
			{Name: "interface with optional", Code: `interface Config { host: string; port?: number; } const c: Config = { host: "localhost" }; export default c`, ExpectedType: "Config", ExpectedKind: "object"},
			{Name: "nested interface", Code: `interface Inner { x: number; } interface Outer { inner: Inner; } const o: Outer = { inner: { x: 1 } }; export default o`, ExpectedType: "Outer", ExpectedKind: "object"},
		},
	},
	{
		Name: "Type Aliases",
		Tests: []TestCase{
			{Name: "type alias primitive", Code: `type ID = number; const id: ID = 42; export default id`, ExpectedType: "number", ExpectedKind: "primitive"},
			{Name: "type alias object", Code: `type Point = { x: number; y: number }; const p: Point = { x: 1, y: 2 }; export default p`, ExpectedType: "Point", ExpectedKind: "object"},
			{Name: "type alias union", Code: `type Result = "success" | "error"; const r: Result = "success"; export default r`, ExpectedType: `"success" | "error"`, ExpectedKind: "union"},
		},
	},
	{
		Name: "Union Types",
		Tests: []TestCase{
			{Name: "string | number", Code: `const x: string | number = "hi"; export default x`, ExpectedType: "string | number", ExpectedKind: "union"},
			{Name: "literal union", Code: `const x: "a" | "b" = "a"; export default x`, ExpectedType: `"a" | "b"`, ExpectedKind: "union"},
			{Name: "nullable", Code: `const x: string | null = null; export default x`, ExpectedType: "string | null", ExpectedKind: "union"},
			{Name: "optional param", Code: `const x: number | undefined = undefined; export default x`, ExpectedType: "number | undefined", ExpectedKind: "union"},
		},
	},
	{
		Name: "Function Types",
		Tests: []TestCase{
			{Name: "arrow function", Code: `const fn = (x: number): number => x * 2; export default fn`, ExpectedType: "(x: number) => number", ExpectedKind: "function"},
			{Name: "function expression", Code: `const fn = function(x: number): string { return String(x); }; export default fn`, ExpectedType: "(x: number) => string", ExpectedKind: "function"},
			{Name: "void return", Code: `const fn = (): void => {}; export default fn`, ExpectedType: "() => void", ExpectedKind: "function"},
			{Name: "multiple params", Code: `const fn = (a: number, b: string): boolean => true; export default fn`, ExpectedType: "(a: number, b: string) => boolean", ExpectedKind: "function"},
		},
	},
	{
		Name: "Generic Types",
		Tests: []TestCase{
			{Name: "Promise<number>", Code: `const p: Promise<number> = Promise.resolve(42); export default p`, ExpectedType: "Promise<number>", ExpectedKind: "object"},
			{Name: "Map<string, number>", Code: `const m: Map<string, number> = new Map(); export default m`, ExpectedType: "Map<string, number>", ExpectedKind: "object"},
			{Name: "Set<string>", Code: `const s: Set<string> = new Set(); export default s`, ExpectedType: "Set<string>", ExpectedKind: "object"},
		},
	},
	{
		Name: "Tuple Types",
		Tests: []TestCase{
			{Name: "simple tuple", Code: `const t: [number, string] = [1, "a"]; export default t`, ExpectedType: "[number, string]", ExpectedKind: "array"},
			{Name: "readonly tuple", Code: `const t = [1, "a"] as const; export default t`, ExpectedType: "readonly [1, \"a\"]", ExpectedKind: "array"},
			{Name: "tuple with optional", Code: `const t: [number, string?] = [1]; export default t`, ExpectedType: "[number, string?]", ExpectedKind: "array"},
		},
	},
	{
		Name: "Enum Types",
		Tests: []TestCase{
			{Name: "numeric enum", Code: `enum Color { Red, Green, Blue } const c: Color = Color.Green; export default c`, ExpectedType: "Color", ExpectedKind: "primitive"},
			{Name: "string enum", Code: `enum Dir { Up = "UP", Down = "DOWN" } const d: Dir = Dir.Up; export default d`, ExpectedType: "Dir", ExpectedKind: "primitive"},
			{Name: "enum value", Code: `enum Color { Red, Green, Blue } export default Color.Green`, ExpectedType: "Color.Green", ExpectedKind: "literal"},
		},
	},
	{
		Name: "Complex Types",
		Tests: []TestCase{
			{Name: "intersection", Code: `type A = { a: number }; type B = { b: string }; const x: A & B = { a: 1, b: "x" }; export default x`, ExpectedType: "A & B", ExpectedKind: "object"},
			{Name: "mapped type result", Code: `type Keys = "a" | "b"; const obj: Record<Keys, number> = { a: 1, b: 2 }; export default obj`, ExpectedType: "Record<Keys, number>", ExpectedKind: "object"},
			{Name: "partial type", Code: `interface Full { a: number; b: string }; const p: Partial<Full> = { a: 1 }; export default p`, ExpectedType: "Partial<Full>", ExpectedKind: "object"},
		},
	},
	{
		Name: "Inferred from Expressions",
		Tests: []TestCase{
			{Name: "arithmetic", Code: `const x = 10 + 5; export default x`, ExpectedType: "number", ExpectedKind: "primitive"},
			{Name: "string concat", Code: `const x = "hello" + " world"; export default x`, ExpectedType: "string", ExpectedKind: "primitive"},
			{Name: "comparison", Code: `const x = 5 > 3; export default x`, ExpectedType: "boolean", ExpectedKind: "primitive"},
			{Name: "ternary", Code: `const x = true ? 1 : 0; export default x`, ExpectedType: "number", ExpectedKind: "primitive"},
			{Name: "array method", Code: `const x = [1, 2, 3].map(n => n * 2); export default x`, ExpectedType: "number[]", ExpectedKind: "array"},
			{Name: "object spread", Code: `const a = { x: 1 }; const b = { ...a, y: 2 }; export default b`, ExpectedType: "{ y: number; x: number }", ExpectedKind: "object"},
		},
	},
	{
		Name: "Class Types",
		Tests: []TestCase{
			{Name: "class instance", Code: `class Point { constructor(public x: number, public y: number) {} } const p = new Point(1, 2); export default p`, ExpectedType: "Point", ExpectedKind: "object"},
			{Name: "class with methods", Code: `class Counter { count = 0; inc() { this.count++; } } const c = new Counter(); export default c`, ExpectedType: "Counter", ExpectedKind: "object"},
		},
	},
}

func main() {
	fmt.Println("╔══════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    Type Inference Comparison Test Suite                      ║")
	fmt.Println("║                         Bun (TS Compiler) vs Go (Regex)                      ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	if !typeinfer.IsBunAvailable() {
		fmt.Println("Error: Bun is not available. This test requires Bun for TypeScript type inference.")
		os.Exit(1)
	}

	bunInferrer := typeinfer.NewInferrer()
	defer func() {
		_ = bunInferrer.Close()
	}()
	goAnalyzer := contract.NewAnalyzer()
	ctx := context.Background()

	type Result struct {
		BunType, BunKind, GoType, GoKind string
		BunMatch, GoMatch                bool
	}

	results := make(map[string]map[string]Result)
	totalTests, bunPass, goPass, bothPass, bothFail := 0, 0, 0, 0, 0

	for _, category := range testCategories {
		results[category.Name] = make(map[string]Result)

		for _, test := range category.Tests {
			totalTests++

			bunResult, bunErr := bunInferrer.InferDefaultExport(ctx, test.Code)
			bunType, bunKind := "", ""
			if bunErr == nil && bunResult.Error == "" {
				bunType = bunResult.Type
				bunKind = bunResult.Kind
			}

			goContract, goErr := goAnalyzer.Analyze(test.Code)
			goType, goKind := "", ""
			if goErr == nil && goContract.Type != nil {
				goType = goContract.Type.Name
				if goType == "" {
					goType = formatTypeDef(goContract.Type)
				}
				goKind = goContract.Type.Kind
			}

			bunMatch := normalizeType(bunType) == normalizeType(test.ExpectedType) || bunKind == test.ExpectedKind
			goMatch := normalizeType(goType) == normalizeType(test.ExpectedType) || goKind == test.ExpectedKind

			results[category.Name][test.Name] = Result{bunType, bunKind, goType, goKind, bunMatch, goMatch}

			if bunMatch {
				bunPass++
			}
			if goMatch {
				goPass++
			}
			if bunMatch && goMatch {
				bothPass++
			}
			if !bunMatch && !goMatch {
				bothFail++
			}
		}
	}

	printBox := func(content string) {
		padding := 78 - len(content)
		if padding < 0 {
			padding = 0
		}
		fmt.Printf("║%s%s║\n", content, strings.Repeat(" ", padding))
	}

	for _, category := range testCategories {
		fmt.Printf("┌─ %s ", category.Name)
		fmt.Println(strings.Repeat("─", 75-len(category.Name)) + "┐")

		for _, test := range category.Tests {
			r := results[category.Name][test.Name]
			bunIcon, goIcon := "✓", "✓"
			if !r.BunMatch {
				bunIcon = "✗"
			}
			if !r.GoMatch {
				goIcon = "✗"
			}

			status := "  "
			if !r.BunMatch && !r.GoMatch {
				status = "!!"
			} else if r.BunMatch != r.GoMatch {
				status = "!="
			}

			// Truncate name if too long
			name := test.Name
			if len(name) > 38 {
				name = name[:35] + "..."
			}

			// Fixed format: │ SS NAME(38)  Bun: X  Go: X                     │
			// Display width: 1 + 2 + 1 + 38 + 7 + 1 + 6 + 1 + 21 = 78
			fmt.Printf("│ %s %-38s  Bun: %s  Go: %s                     │\n", status, name, bunIcon, goIcon)
		}
		fmt.Println("└" + strings.Repeat("─", 78) + "┘")
		fmt.Println()
	}

	fmt.Println("╔══════════════════════════════════════════════════════════════════════════════╗")
	printBox("                                  Summary                                     ")
	fmt.Println("╠══════════════════════════════════════════════════════════════════════════════╣")
	printBox(fmt.Sprintf("  Total Tests:     %d", totalTests))
	printBox(fmt.Sprintf("  Bun Passed:      %d / %d (%5.1f%%)", bunPass, totalTests, float64(bunPass)/float64(totalTests)*100))
	printBox(fmt.Sprintf("  Go Passed:       %d / %d (%5.1f%%)", goPass, totalTests, float64(goPass)/float64(totalTests)*100))
	printBox(fmt.Sprintf("  Both Passed:     %d / %d (%5.1f%%)", bothPass, totalTests, float64(bothPass)/float64(totalTests)*100))
	printBox(fmt.Sprintf("  Both Failed:     %d", bothFail))
	fmt.Println("╚══════════════════════════════════════════════════════════════════════════════╝")

	if bothPass != totalTests {
		os.Exit(1)
	}
}

func formatTypeDef(t *contract.TypeDef) string {
	if t == nil {
		return "unknown"
	}
	switch t.Kind {
	case "primitive", "any":
		if t.Name != "" {
			return t.Name
		}
		return t.Kind
	case "literal":
		if t.LiteralValue != nil {
			if s, ok := t.LiteralValue.(string); ok {
				return fmt.Sprintf("%q", s)
			}
			return fmt.Sprint(t.LiteralValue)
		}
		return t.Name
	case "array":
		if t.ElementType != nil {
			return formatTypeDef(t.ElementType) + "[]"
		}
		return "any[]"
	case "object":
		if t.Name != "" {
			return t.Name
		}
		if len(t.Properties) == 0 {
			return "{}"
		}
		props := make([]string, 0, len(t.Properties))
		for _, p := range t.Properties {
			opt := ""
			if !p.Required {
				opt = "?"
			}
			props = append(props, fmt.Sprintf("%s%s: %s", p.Name, opt, formatTypeDef(p.Type)))
		}
		sort.Strings(props)
		return "{ " + strings.Join(props, "; ") + " }"
	case "union":
		if len(t.UnionTypes) > 0 {
			parts := make([]string, len(t.UnionTypes))
			for i, ut := range t.UnionTypes {
				parts[i] = formatTypeDef(ut)
			}
			return strings.Join(parts, " | ")
		}
		return t.Name
	case "function":
		ret := "void"
		if t.ReturnType != nil {
			ret = formatTypeDef(t.ReturnType)
		}
		return "() => " + ret
	default:
		if t.Name != "" {
			return t.Name
		}
		return t.Kind
	}
}

func normalizeType(t string) string {
	t = strings.TrimSpace(t)
	t = strings.ReplaceAll(t, "  ", " ")
	t = strings.ReplaceAll(t, "; }", " }")
	t = strings.ReplaceAll(t, " ;", ";")
	return t
}
