// Command functions demonstrates tsgo function injection capabilities.
//
// tsgo supports injecting helper functions that scripts can call.
// There are two approaches:
//
// 1. TSCode only (recommended): Define once, works everywhere
// 2. TSCode + GoFunc: Optimize GOJA performance with native Go
package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/koltyakov/tsgo"
)

func main() {
	fmt.Println("=== tsgo Function Injection Example ===")
	fmt.Println()

	// Define functions using TSCode only (recommended approach)
	// These work identically on both GOJA and Bun engines
	functions := map[string]tsgo.FunctionDef{
		// Simple arithmetic - TSCode only
		"add": {
			TSCode: `function add(a: number, b: number): number { return a + b; }`,
		},
		"subtract": {
			TSCode: `function subtract(a: number, b: number): number { return a - b; }`,
		},

		// String utilities - TSCode only
		"capitalize": {
			TSCode: `function capitalize(s: string): string {
				return s.charAt(0).toUpperCase() + s.slice(1).toLowerCase();
			}`,
		},
		"slugify": {
			TSCode: `function slugify(s: string): string {
				return s.toLowerCase().trim().replace(/\s+/g, '-').replace(/[^\w-]/g, '');
			}`,
		},

		// Array helpers - TSCode only
		"unique": {
			TSCode: `function unique<T>(arr: T[]): T[] { return [...new Set(arr)]; }`,
		},
		"chunk": {
			TSCode: `function chunk<T>(arr: T[], size: number): T[][] {
				const result: T[][] = [];
				for (let i = 0; i < arr.length; i += size) {
					result.push(arr.slice(i, i + size));
				}
				return result;
			}`,
		},

		// Math utilities with GoFunc optimization
		// GOJA uses native Go (faster), Bun uses TSCode
		"sqrt": {
			TSCode: `function sqrt(x: number): number { return Math.sqrt(x); }`,
			GoFunc: math.Sqrt, // Native Go for GOJA performance
		},
		"pow": {
			TSCode: `function pow(base: number, exp: number): number { return Math.pow(base, exp); }`,
			GoFunc: math.Pow, // Native Go for GOJA performance
		},

		// Complex function with GoFunc for performance-critical code
		"fibonacci": {
			TSCode: `function fibonacci(n: number): number {
				if (n <= 1) return n;
				let a = 0, b = 1;
				for (let i = 2; i <= n; i++) {
					const c = a + b;
					a = b;
					b = c;
				}
				return b;
			}`,
			// Optional: Go implementation for heavy computation in GOJA
			GoFunc: func(n int) int {
				if n <= 1 {
					return n
				}
				a, b := 0, 1
				for i := 2; i <= n; i++ {
					a, b = b, a+b
				}
				return b
			},
		},

		// String manipulation with Go optimization
		"repeat": {
			TSCode: `function repeat(s: string, n: number): string { return s.repeat(n); }`,
			GoFunc: strings.Repeat, // Native Go for GOJA
		},
	}

	// Test with both engines to show identical behavior
	engines := []struct {
		name   string
		engine tsgo.EngineType
	}{
		{"GOJA", tsgo.EngineGOJA},
		{"Bun", tsgo.EngineBun},
	}

	for _, eng := range engines {
		fmt.Printf("--- Engine: %s ---\n\n", eng.name)

		executor := tsgo.New(
			tsgo.WithEngine(eng.engine),
			tsgo.WithTimeout(5*time.Second),
			tsgo.WithFunctions(functions),
		)

		ctx := context.Background()

		// Example 1: Simple arithmetic
		fmt.Println("1. Arithmetic functions:")
		result, err := executor.Execute(ctx, `
			const sum = add(10, 5);
			const diff = subtract(10, 5);
			export default { sum, diff };
		`)
		if err != nil {
			log.Printf("   Error: %v\n", err)
		} else {
			fmt.Printf("   Result: %v\n", result.Value)
		}

		// Example 2: String functions
		fmt.Println("\n2. String functions:")
		result, err = executor.Execute(ctx, `
			const title = capitalize("hello WORLD");
			const slug = slugify("Hello World! This is a Test");
			export default { title, slug };
		`)
		if err != nil {
			log.Printf("   Error: %v\n", err)
		} else {
			fmt.Printf("   Result: %v\n", result.Value)
		}

		// Example 3: Array helpers
		fmt.Println("\n3. Array helpers:")
		result, err = executor.Execute(ctx, `
			const deduped = unique([1, 2, 2, 3, 3, 3]);
			const chunks = chunk([1, 2, 3, 4, 5, 6], 2);
			export default { deduped, chunks };
		`)
		if err != nil {
			log.Printf("   Error: %v\n", err)
		} else {
			fmt.Printf("   Result: %v\n", result.Value)
		}

		// Example 4: Math with Go optimization (GOJA only)
		fmt.Println("\n4. Math functions (GOJA uses GoFunc, Bun uses TSCode):")
		result, err = executor.Execute(ctx, `
			const sqrtVal = sqrt(16);
			const powVal = pow(2, 10);
			const fib = fibonacci(20);
			export default { sqrtVal, powVal, fib };
		`)
		if err != nil {
			log.Printf("   Error: %v\n", err)
		} else {
			fmt.Printf("   Result: %v\n", result.Value)
		}

		// Example 5: String repeat with Go optimization (GOJA only)
		fmt.Println("\n5. String repeat (GOJA uses GoFunc, Bun uses TSCode):")
		result, err = executor.Execute(ctx, `
			export default repeat("Go! ", 3);
		`)
		if err != nil {
			log.Printf("   Error: %v\n", err)
		} else {
			fmt.Printf("   Result: %v\n", result.Value)
		}

		_ = executor.Close()
		fmt.Println()
	}

	fmt.Println("=== Summary ===")
	fmt.Println("• TSCode is required and works on both engines")
	fmt.Println("• GoFunc is optional and only used by GOJA for performance")
	fmt.Println("• Bun always executes TSCode (can't call Go functions)")
	fmt.Println("• Same results, engine-appropriate execution path")
}
