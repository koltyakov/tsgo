// Command basic demonstrates tsgo TypeScript execution capabilities.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/koltyakov/tsgo"
)

func main() {
	fmt.Println("=== tsgo Basic Example ===")
	fmt.Println()

	// Create executor with GOJA engine
	executor := tsgo.New(
		tsgo.WithEngine(tsgo.EngineGOJA),
		tsgo.WithTimeout(5*time.Second),
		tsgo.WithGlobals(map[string]any{
			"userId":   42,
			"userName": "Alice",
			"isAdmin":  true,
		}),
	)
	defer executor.Close()

	ctx := context.Background()

	// Example 1: Simple expression
	fmt.Println("1. Simple Expression:")
	result, err := executor.Execute(ctx, `export default 1 + 2 * 3`)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("   Result: %v\n\n", result.Value)

	// Example 2: TypeScript with types
	fmt.Println("2. TypeScript with Types:")
	result, err = executor.Execute(ctx, `
		interface User {
			id: number;
			name: string;
			active: boolean;
		}

		const user: User = {
			id: userId,
			name: userName,
			active: isAdmin
		};

		export default JSON.stringify(user);
	`)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("   Result: %v\n\n", result.Value)

	// Example 3: Function execution
	fmt.Println("3. Function Execution:")
	result, err = executor.Execute(ctx, `
		const greet = (name: string): string => {
			return "Hello, " + name + "!";
		};

		export default greet(userName);
	`)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("   Result: %v\n\n", result.Value)

	// Example 4: Array operations
	fmt.Println("4. Array Operations:")
	result, err = executor.Execute(ctx, `
		const numbers: number[] = [1, 2, 3, 4, 5];
		const doubled = numbers.map(n => n * 2);
		const sum = doubled.reduce((a, b) => a + b, 0);
		export default sum;
	`)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("   Result: %v\n\n", result.Value)

	// Example 5: Object spread
	fmt.Println("5. Object Spread:")
	result, err = executor.Execute(ctx, `
		const base = { a: 1, b: 2 };
		const extended = { ...base, c: 3 };
		export default JSON.stringify(extended);
	`)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("   Result: %v\n\n", result.Value)

	// Example 6: Object spread
	fmt.Println("6. Default Export Function:")
	result, err = executor.Execute(ctx, `
		const multiply = (x: number, y: number): number => {
			return x * y;
		};
		export default function() {
			return multiply(6, 7);
		};
	`)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("   Result: %v\n\n", result.Value)

	fmt.Println("=== Type Generation ===")
	fmt.Println()

	// Generate TypeScript definitions for globals
	dts := tsgo.GenerateContextTypes(map[string]any{
		"userId":   42,
		"userName": "Alice",
		"isAdmin":  true,
	})
	fmt.Println("Generated .d.ts:")
	fmt.Println(dts)

	fmt.Println("=== Done ===")
}
