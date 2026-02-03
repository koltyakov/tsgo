// Package benchmark provides performance benchmarks for tsgo engines.
package benchmark

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/koltyakov/tsgo/internal/engine/bun"
	"github.com/koltyakov/tsgo/internal/engine/goja"
	"github.com/koltyakov/tsgo/internal/transpiler"
	"github.com/koltyakov/tsgo/internal/types"
)

// Engine interface for benchmarking
type Engine interface {
	Execute(ctx context.Context, code string, globals map[string]any) (*types.Result, error)
	Close() error
}

// testCase defines a test scenario
type testCase struct {
	name        string
	code        string
	globals     map[string]any
	needsAsync  bool
	description string
}

// Test cases with varying complexity
var testCases = map[string]testCase{
	"simple_arithmetic": {
		name:        "Simple Arithmetic",
		code:        `export default 1 + 2 * 3 + 4 / 2`,
		description: "Basic math operations",
	},
	"string_operations": {
		name: "String Operations",
		code: `
			const str = "hello world";
			const upper = str.toUpperCase();
			const words = str.split(" ");
			const reversed = words.reverse().join("-");
			export default { upper, reversed, length: str.length };
		`,
		description: "String manipulation",
	},
	"array_operations": {
		name: "Array Operations",
		code: `
			const numbers = Array.from({ length: 100 }, (_, i) => i + 1);
			const evens = numbers.filter(n => n % 2 === 0);
			const doubled = evens.map(n => n * 2);
			const sum = doubled.reduce((a, b) => a + b, 0);
			export default sum;
		`,
		description: "Array filter/map/reduce on 100 items",
	},
	"object_manipulation": {
		name: "Object Manipulation",
		code: `
			const users = [
				{ id: 1, name: "Alice", age: 30 },
				{ id: 2, name: "Bob", age: 25 },
				{ id: 3, name: "Charlie", age: 35 },
			];
			const byId = users.reduce((acc, u) => ({ ...acc, [u.id]: u }), {});
			const names = users.map(u => u.name);
			const avgAge = users.reduce((sum, u) => sum + u.age, 0) / users.length;
			export default { byId, names, avgAge };
		`,
		description: "Object spread, reduce to map",
	},
	"recursive_fibonacci": {
		name: "Recursive Fibonacci",
		code: `
			function fib(n: number): number {
				if (n <= 1) return n;
				return fib(n - 1) + fib(n - 2);
			}
			export default fib(20);
		`,
		description: "Recursive function (fib 20 = 6765)",
	},
	"iterative_fibonacci": {
		name: "Iterative Fibonacci",
		code: `
			function fib(n: number): number {
				let a = 0, b = 1;
				for (let i = 0; i < n; i++) {
					[a, b] = [b, a + b];
				}
				return a;
			}
			export default fib(50);
		`,
		description: "Iterative function with destructuring",
	},
	"json_processing": {
		name: "JSON Processing",
		code: `
			const data = JSON.stringify({
				users: Array.from({ length: 50 }, (_, i) => ({
					id: i,
					name: "User" + i,
					email: "user" + i + "@example.com",
					tags: ["tag" + (i % 5), "tag" + (i % 3)],
				})),
			});
			const parsed = JSON.parse(data);
			const emails = parsed.users.map((u: any) => u.email);
			export default { count: emails.length, first: emails[0] };
		`,
		description: "JSON stringify/parse with 50 objects",
	},
	"regex_operations": {
		name: "Regex Operations",
		code: `
			const text = "The quick brown fox jumps over the lazy dog. " +
				"Pack my box with five dozen liquor jugs.";
			const words = text.match(/\\b\\w+\\b/g) || [];
			const vowelWords = words.filter(w => /^[aeiou]/i.test(w));
			const longWords = words.filter(w => w.length > 4);
			export default { 
				wordCount: words.length, 
				vowelWords: vowelWords.length,
				longWords: longWords.length 
			};
		`,
		description: "Regular expression matching and filtering",
	},
	"class_instantiation": {
		name: "Class Instantiation",
		code: `
			class Point {
				constructor(public x: number, public y: number) {}
				distance(other: Point): number {
					return Math.sqrt(
						Math.pow(this.x - other.x, 2) + 
						Math.pow(this.y - other.y, 2)
					);
				}
			}
			const points = Array.from({ length: 100 }, (_, i) => 
				new Point(Math.sin(i) * 100, Math.cos(i) * 100)
			);
			let totalDist = 0;
			for (let i = 1; i < points.length; i++) {
				totalDist += points[i-1].distance(points[i]);
			}
			export default Math.round(totalDist);
		`,
		description: "ES6 class with methods, 100 instances",
	},
	"nested_loops": {
		name: "Nested Loops",
		code: `
			let result = 0;
			for (let i = 0; i < 100; i++) {
				for (let j = 0; j < 100; j++) {
					result += (i * j) % 7;
				}
			}
			export default result;
		`,
		description: "10,000 iterations nested loop",
	},
	"closure_heavy": {
		name: "Closure Heavy",
		code: `
			function createCounter(start: number) {
				let count = start;
				return {
					increment: () => ++count,
					decrement: () => --count,
					getValue: () => count,
				};
			}
			const counters = Array.from({ length: 50 }, (_, i) => createCounter(i));
			counters.forEach(c => {
				for (let i = 0; i < 10; i++) c.increment();
			});
			const sum = counters.reduce((s, c) => s + c.getValue(), 0);
			export default sum;
		`,
		description: "50 closures with state",
	},
	"spread_destructure": {
		name: "Spread & Destructure",
		code: `
			const arr1 = [1, 2, 3];
			const arr2 = [4, 5, 6];
			const combined = [...arr1, ...arr2];
			const obj1 = { a: 1, b: 2 };
			const obj2 = { c: 3, d: 4 };
			const merged = { ...obj1, ...obj2 };
			const [first, , third] = combined;
			const { a, ...rest } = merged;
			export default { combined, merged, first, third, a, rest };
		`,
		description: "Array/object spread and destructuring",
	},
	"promise_chain": {
		name:       "Promise Chain",
		needsAsync: true,
		code: `
			export default async function() {
				const delay = (ms: number, val: number) => 
					new Promise<number>(r => setTimeout(() => r(val), ms));
				
				const result = await delay(1, 10)
					.then(v => delay(1, v * 2))
					.then(v => delay(1, v + 5));
				return result;
			}
		`,
		description: "Async promise chain (Bun only)",
	},
	"async_parallel": {
		name:       "Async Parallel",
		needsAsync: true,
		code: `
			export default async function() {
				const delay = (ms: number, val: number) => 
					new Promise<number>(r => setTimeout(() => r(val), ms));
				
				const results = await Promise.all([
					delay(1, 1),
					delay(1, 2),
					delay(1, 3),
					delay(1, 4),
					delay(1, 5),
				]);
				return results.reduce((a, b) => a + b, 0);
			}
		`,
		description: "Parallel async execution (Bun only)",
	},
	"with_globals": {
		name: "With Globals",
		code: `
			interface Config {
				multiplier: number;
				prefix: string;
			}
			const config = { multiplier, prefix } as Config;
			const values = items.map((x: number) => x * config.multiplier);
			const sum = values.reduce((a: number, b: number) => a + b, 0);
			export default { result: config.prefix + sum };
		`,
		globals: map[string]any{
			"multiplier": 3,
			"prefix":     "Total: ",
			"items":      []int{1, 2, 3, 4, 5},
		},
		description: "Using injected global variables",
	},
	"type_guards": {
		name: "Type Guards",
		code: `
			type Shape = { kind: "circle"; radius: number } | { kind: "rect"; w: number; h: number };
			
			function area(s: Shape): number {
				if (s.kind === "circle") {
					return Math.PI * s.radius * s.radius;
				}
				return s.w * s.h;
			}
			
			const shapes: Shape[] = [
				{ kind: "circle", radius: 5 },
				{ kind: "rect", w: 10, h: 20 },
				{ kind: "circle", radius: 3 },
			];
			
			const totalArea = shapes.reduce((sum, s) => sum + area(s), 0);
			export default Math.round(totalArea * 100) / 100;
		`,
		description: "TypeScript discriminated unions",
	},
	"generics": {
		name: "Generics",
		code: `
			function identity<T>(arg: T): T {
				return arg;
			}
			
			function map<T, U>(arr: T[], fn: (x: T) => U): U[] {
				return arr.map(fn);
			}
			
			const nums = [1, 2, 3, 4, 5];
			const doubled = map(nums, x => x * 2);
			const strs = map(doubled, x => "n" + x);
			
			export default { 
				identity: identity(42),
				doubled,
				strs 
			};
		`,
		description: "Generic functions",
	},
}

// BenchmarkGOJA benchmarks the GOJA engine
func BenchmarkGOJA(b *testing.B) {
	trans := transpiler.New()
	engine := goja.New(goja.Config{PoolSize: 8})
	defer func() { _ = engine.Close() }()

	ctx := context.Background()

	for id, tc := range testCases {
		if tc.needsAsync {
			continue // GOJA doesn't support async
		}

		transpiled, _, _, _, err := trans.Transpile(tc.code)
		if err != nil {
			b.Logf("GOJA: %s - transpile error: %v", tc.name, err)
			continue
		}

		b.Run("GOJA_"+id, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := engine.Execute(ctx, transpiled, tc.globals)
				if err != nil {
					b.Fatalf("execution error: %v", err)
				}
			}
		})
	}
}

// BenchmarkBun benchmarks the Bun engine
func BenchmarkBun(b *testing.B) {
	if _, err := exec.LookPath("bun"); err != nil {
		b.Skip("Bun is not installed")
	}

	engine, err := bun.New(bun.Config{PoolSize: 4})
	if err != nil {
		b.Fatalf("failed to create engine: %v", err)
	}
	defer func() { _ = engine.Close() }()

	if !engine.IsAvailable() {
		b.Skip("Bun engine is not available")
	}

	for id, tc := range testCases {
		b.Run("Bun_"+id, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				_, err := engine.Execute(ctx, tc.code, tc.globals)
				cancel()
				if err != nil {
					b.Fatalf("execution error: %v", err)
				}
			}
		})
	}
}

// BenchmarkConcurrent tests concurrent execution performance
func BenchmarkConcurrent(b *testing.B) {
	trans := transpiler.New()
	gojaEngine := goja.New(goja.Config{PoolSize: 16})
	defer func() { _ = gojaEngine.Close() }()

	ctx := context.Background()
	code := testCases["array_operations"].code

	transpiled, _, _, _, err := trans.Transpile(code)
	if err != nil {
		b.Fatalf("transpile error: %v", err)
	}

	concurrencyLevels := []int{1, 4, 8, 16, 32}

	for _, level := range concurrencyLevels {
		b.Run(fmt.Sprintf("GOJA_concurrent_%d", level), func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var wg sync.WaitGroup
				wg.Add(level)
				for j := 0; j < level; j++ {
					go func() {
						defer wg.Done()
						_, _ = gojaEngine.Execute(ctx, transpiled, nil)
					}()
				}
				wg.Wait()
			}
		})
	}

	// Bun concurrent - test various concurrency levels
	if _, err := exec.LookPath("bun"); err == nil {
		bunEngine, err := bun.New(bun.Config{PoolSize: 8})
		if err == nil && bunEngine.IsAvailable() {
			defer func() { _ = bunEngine.Close() }()

			for _, level := range concurrencyLevels {
				b.Run(fmt.Sprintf("Bun_concurrent_%d", level), func(b *testing.B) {
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						var wg sync.WaitGroup
						wg.Add(level)
						for j := 0; j < level; j++ {
							go func() {
								defer wg.Done()
								ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
								_, _ = bunEngine.Execute(ctx, code, nil)
								cancel()
							}()
						}
						wg.Wait()
					}
				})
			}
		}
	}
}

// BenchmarkTranspiler benchmarks the esbuild transpiler
func BenchmarkTranspiler(b *testing.B) {
	trans := transpiler.New()
	transWithCache := transpiler.New()

	ctx := context.Background()
	_ = ctx

	for id, tc := range testCases {
		if tc.needsAsync {
			continue
		}

		b.Run("Transpile_"+id, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _, _, _, err := trans.Transpile(tc.code)
				if err != nil {
					b.Fatalf("transpile error: %v", err)
				}
			}
		})

		// Warm the cache for cached benchmark
		_, _, _, _, _ = transWithCache.Transpile(tc.code)
		b.Run("Transpile_Cached_"+id, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _, _, _, err := transWithCache.Transpile(tc.code)
				if err != nil {
					b.Fatalf("transpile error: %v", err)
				}
			}
		})
	}
}

// TestFeatureSupport shows which features each engine supports
func TestFeatureSupport(t *testing.T) {
	trans := transpiler.New()

	gojaEngine := goja.New(goja.Config{PoolSize: 4})
	defer func() { _ = gojaEngine.Close() }()

	var bunEngine *bun.Engine
	hasBun := false
	if _, err := exec.LookPath("bun"); err == nil {
		bunEngine, err = bun.New(bun.Config{PoolSize: 2})
		if err == nil && bunEngine.IsAvailable() {
			hasBun = true
			defer func() { _ = bunEngine.Close() }()
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Println("\n=== Engine Feature Support Matrix ===")
	fmt.Println()
	fmt.Printf("%-25s | %-10s | %-10s | %s\n", "Feature", "GOJA", "Bun", "Description")
	fmt.Println(strings.Repeat("-", 80))

	for id, tc := range testCases {
		gojaSupport := "❌"
		bunSupport := "❌"

		// Test GOJA
		if !tc.needsAsync {
			transpiled, _, _, _, err := trans.Transpile(tc.code)
			if err == nil {
				_, err = gojaEngine.Execute(ctx, transpiled, tc.globals)
				if err == nil {
					gojaSupport = "✅"
				}
			}
		} else {
			gojaSupport = "N/A"
		}

		// Test Bun
		if hasBun {
			_, err := bunEngine.Execute(ctx, tc.code, tc.globals)
			if err == nil {
				bunSupport = "✅"
			}
		} else {
			bunSupport = "N/A"
		}

		fmt.Printf("%-25s | %-10s | %-10s | %s\n", id, gojaSupport, bunSupport, tc.description)
	}

	fmt.Println()
}

// TestPerformanceComparison runs a comparative performance test
func TestPerformanceComparison(t *testing.T) {
	trans := transpiler.New()

	gojaEngine := goja.New(goja.Config{PoolSize: 8})
	defer func() { _ = gojaEngine.Close() }()

	var bunEngine *bun.Engine
	hasBun := false
	if _, err := exec.LookPath("bun"); err == nil {
		bunEngine, err = bun.New(bun.Config{PoolSize: 4})
		if err == nil && bunEngine.IsAvailable() {
			hasBun = true
			defer func() { _ = bunEngine.Close() }()
		}
	}

	ctx := context.Background()
	iterations := 100

	fmt.Println("\n=== Performance Comparison (100 iterations) ===")
	fmt.Println()
	fmt.Printf("%-25s | %-15s | %-15s | %s\n", "Test Case", "GOJA", "Bun", "Winner")
	fmt.Println(strings.Repeat("-", 80))

	for id, tc := range testCases {
		if tc.needsAsync {
			continue
		}

		var gojaTime, bunTime time.Duration

		// Benchmark GOJA
		transpiled, _, _, _, err := trans.Transpile(tc.code)
		if err != nil {
			continue
		}

		start := time.Now()
		for i := 0; i < iterations; i++ {
			_, _ = gojaEngine.Execute(ctx, transpiled, tc.globals)
		}
		gojaTime = time.Since(start)

		// Benchmark Bun
		if hasBun {
			start = time.Now()
			for i := 0; i < iterations; i++ {
				_, _ = bunEngine.Execute(ctx, tc.code, tc.globals)
			}
			bunTime = time.Since(start)
		}

		gojaAvg := gojaTime / time.Duration(iterations)
		bunAvg := bunTime / time.Duration(iterations)

		winner := "GOJA"
		if hasBun && bunTime > 0 && bunTime < gojaTime {
			winner = "Bun"
		}
		if !hasBun {
			winner = "-"
		}

		gojaStr := fmt.Sprintf("%v", gojaAvg)
		bunStr := "N/A"
		if hasBun {
			bunStr = fmt.Sprintf("%v", bunAvg)
		}

		fmt.Printf("%-25s | %-15s | %-15s | %s\n", id, gojaStr, bunStr, winner)
	}

	fmt.Println()
}

// TestConcurrencyScaling tests how engines scale with concurrent load
func TestConcurrencyScaling(t *testing.T) {
	trans := transpiler.New()

	gojaEngine := goja.New(goja.Config{PoolSize: 32})
	defer func() { _ = gojaEngine.Close() }()

	ctx := context.Background()
	code := testCases["nested_loops"].code

	transpiled, _, _, _, err := trans.Transpile(code)
	if err != nil {
		t.Fatalf("transpile error: %v", err)
	}

	concurrencyLevels := []int{1, 2, 4, 8, 16, 32}
	iterations := 50

	fmt.Println("\n=== GOJA Concurrency Scaling ===")
	fmt.Println()
	fmt.Printf("%-15s | %-15s | %-15s\n", "Concurrency", "Total Time", "Per Op")
	fmt.Println(strings.Repeat("-", 50))

	for _, level := range concurrencyLevels {
		start := time.Now()
		for i := 0; i < iterations; i++ {
			var wg sync.WaitGroup
			wg.Add(level)
			for j := 0; j < level; j++ {
				go func() {
					defer wg.Done()
					_, _ = gojaEngine.Execute(ctx, transpiled, nil)
				}()
			}
			wg.Wait()
		}
		elapsed := time.Since(start)
		perOp := elapsed / time.Duration(iterations*level)

		fmt.Printf("%-15d | %-15v | %-15v\n", level, elapsed, perOp)
	}

	fmt.Println()
}

// BenchmarkColdStart benchmarks engine creation + execution (no reuse)
// This demonstrates the overhead of creating a new engine per request
func BenchmarkColdStart(b *testing.B) {
	code := testCases["simple_arithmetic"].code
	trans := transpiler.New()
	transpiled, _, _, _, _ := trans.Transpile(code)

	b.Run("GOJA_cold_start", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			engine := goja.New(goja.Config{PoolSize: 1})
			ctx := context.Background()
			_, err := engine.Execute(ctx, transpiled, nil)
			if err != nil {
				b.Fatalf("execution error: %v", err)
			}
			_ = engine.Close()
		}
	})

	b.Run("GOJA_warm_reuse", func(b *testing.B) {
		engine := goja.New(goja.Config{PoolSize: 1})
		defer func() { _ = engine.Close() }()
		ctx := context.Background()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := engine.Execute(ctx, transpiled, nil)
			if err != nil {
				b.Fatalf("execution error: %v", err)
			}
		}
	})

	// Bun cold start vs warm
	if _, err := exec.LookPath("bun"); err != nil {
		return
	}

	b.Run("Bun_cold_start", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			engine, err := bun.New(bun.Config{PoolSize: 1})
			if err != nil {
				b.Fatalf("engine creation error: %v", err)
			}
			if !engine.IsAvailable() {
				b.Skip("Bun not available")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, err = engine.Execute(ctx, code, nil)
			cancel()
			if err != nil {
				b.Fatalf("execution error: %v", err)
			}
			_ = engine.Close()
		}
	})

	b.Run("Bun_warm_reuse", func(b *testing.B) {
		engine, err := bun.New(bun.Config{PoolSize: 1})
		if err != nil {
			b.Fatalf("engine creation error: %v", err)
		}
		if !engine.IsAvailable() {
			b.Skip("Bun not available")
		}
		defer func() { _ = engine.Close() }()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, err := engine.Execute(ctx, code, nil)
			cancel()
			if err != nil {
				b.Fatalf("execution error: %v", err)
			}
		}
	})

	b.Run("Bun_background_warmup", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Engine creation is fast with background warmup
			engine, err := bun.New(bun.Config{
				PoolSize:         1,
				BackgroundWarmup: true,
			})
			if err != nil {
				b.Fatalf("engine creation error: %v", err)
			}
			if !engine.IsAvailable() {
				b.Skip("Bun not available")
			}
			// First request may wait for process startup
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, err = engine.Execute(ctx, code, nil)
			cancel()
			if err != nil {
				b.Fatalf("execution error: %v", err)
			}
			_ = engine.Close()
		}
	})

	// This benchmark shows that New() returns immediately with BackgroundWarmup
	// This is useful when you want to do other initialization while the engine warms up
	b.Run("Bun_parallel_init_blocking", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Blocking: must wait for engine before doing other work
			engine, _ := bun.New(bun.Config{PoolSize: 1})
			// Simulate other initialization work (50ms)
			time.Sleep(50 * time.Millisecond)
			// Execute
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, _ = engine.Execute(ctx, code, nil)
			cancel()
			_ = engine.Close()
		}
	})

	b.Run("Bun_parallel_init_background", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Background: engine warms up while we do other work
			engine, _ := bun.New(bun.Config{
				PoolSize:         1,
				BackgroundWarmup: true,
			})
			// Simulate other initialization work (50ms) - engine warms up in parallel
			time.Sleep(50 * time.Millisecond)
			// Execute - process may already be ready!
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, _ = engine.Execute(ctx, code, nil)
			cancel()
			_ = engine.Close()
		}
	})
}
