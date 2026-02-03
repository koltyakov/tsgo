/*
Statistical benchmark runner for tsgo engines.
Runs benchmarks multiple times and outputs markdown-formatted results.

Usage:

	go run ./cmd/benchmark -runs=15
	go run ./cmd/benchmark -runs=20 -output=results.md
*/
package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/koltyakov/tsgo/internal/engine/bun"
	"github.com/koltyakov/tsgo/internal/engine/goja"
	"github.com/koltyakov/tsgo/internal/transpiler"
)

// testCase defines a benchmark scenario
type testCase struct {
	id          string
	name        string
	code        string
	globals     map[string]any
	needsAsync  bool
	description string
}

var testCases = []testCase{
	{
		id:          "simple_arithmetic",
		name:        "Simple Arithmetic",
		code:        `export default 1 + 2 * 3 + 4 / 2`,
		description: "Basic math operations",
	},
	{
		id:   "string_operations",
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
	{
		id:   "array_operations",
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
	{
		id:   "object_manipulation",
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
	{
		id:   "recursive_fibonacci",
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
	{
		id:   "iterative_fibonacci",
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
	{
		id:   "json_processing",
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
	{
		id:   "regex_operations",
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
	{
		id:   "class_instantiation",
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
	{
		id:   "nested_loops",
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
	{
		id:   "closure_heavy",
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
	{
		id:   "spread_destructure",
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
	{
		id:   "type_guards",
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
	{
		id:   "generics",
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
	{
		id:   "with_globals",
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
}

// Stats holds statistical data for a benchmark
type Stats struct {
	Mean   time.Duration
	StdDev time.Duration
	Min    time.Duration
	Max    time.Duration
	P50    time.Duration // Median
	P95    time.Duration
	Runs   int
}

// MemoryStats holds memory usage statistics
type MemoryStats struct {
	AllocBytes uint64 // Bytes allocated
	AllocCount uint64 // Number of allocations
}

// BenchResult holds benchmark results for a test case
type BenchResult struct {
	TestID     string
	GOJAStats  *Stats
	BunStats   *Stats
	GOJAMemory *MemoryStats
	BunMemory  *MemoryStats
}

// ConcurrencyResult holds concurrency benchmark results
type ConcurrencyResult struct {
	Level     int
	GOJAStats *Stats
	BunStats  *Stats
}

func main() {
	runs := flag.Int("runs", 15, "Number of benchmark runs per test case")
	warmup := flag.Int("warmup", 3, "Number of warmup runs before measurement")
	output := flag.String("output", "", "Output file for markdown (default: stdout)")
	section := flag.String("section", "", "Output specific section: comparison, concurrency, detailed, all (default: all)")
	flag.Parse()

	if *runs < 5 {
		fmt.Fprintf(os.Stderr, "Warning: Using at least 5 runs for meaningful statistics\n")
		*runs = 5
	}

	// Check for Bun availability
	hasBun := false
	if _, err := exec.LookPath("bun"); err == nil {
		hasBun = true
	}

	fmt.Fprintf(os.Stderr, "Running benchmarks: %d runs + %d warmup per test case\n", *runs, *warmup)
	fmt.Fprintf(os.Stderr, "Engines: GOJA%s\n\n", map[bool]string{true: ", Bun", false: ""}[hasBun])

	// Run benchmarks
	results := runBenchmarks(*runs, *warmup, hasBun)
	concurrencyResults := runConcurrencyBenchmarks(*runs, *warmup, hasBun)

	// Generate markdown
	md := generateMarkdown(results, concurrencyResults, *runs, hasBun, *section)

	// Output
	if *output != "" {
		if err := os.WriteFile(*output, []byte(md), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Results written to %s\n", *output)
	} else {
		fmt.Println(md)
	}
}

func runBenchmarks(runs, warmup int, hasBun bool) []BenchResult {
	trans := transpiler.New()

	// Create engines
	gojaEngine := goja.New(goja.Config{PoolSize: 8})
	defer func() { _ = gojaEngine.Close() }()

	var bunEngine *bun.Engine
	if hasBun {
		var err error
		bunEngine, err = bun.New(bun.Config{PoolSize: 4})
		if err != nil || !bunEngine.IsAvailable() {
			hasBun = false
		} else {
			defer func() { _ = bunEngine.Close() }()
		}
	}

	ctx := context.Background()
	var results []BenchResult

	for _, tc := range testCases {
		if tc.needsAsync {
			continue
		}

		fmt.Fprintf(os.Stderr, "Benchmarking: %s...\n", tc.id)

		transpiled, _, _, _, err := trans.Transpile(tc.code)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Transpile error: %v\n", err)
			continue
		}

		result := BenchResult{TestID: tc.id}

		// Benchmark GOJA
		gojaTimes := make([]time.Duration, 0, runs)

		// Warmup
		for i := 0; i < warmup; i++ {
			_, _ = gojaEngine.Execute(ctx, transpiled, tc.globals)
		}

		// Memory measurement for GOJA (single run after warmup)
		runtime.GC()
		var memBefore, memAfter runtime.MemStats
		runtime.ReadMemStats(&memBefore)
		_, _ = gojaEngine.Execute(ctx, transpiled, tc.globals)
		runtime.ReadMemStats(&memAfter)
		result.GOJAMemory = &MemoryStats{
			AllocBytes: memAfter.TotalAlloc - memBefore.TotalAlloc,
			AllocCount: memAfter.Mallocs - memBefore.Mallocs,
		}

		// Measured runs (time only)
		for i := 0; i < runs; i++ {
			start := time.Now()
			_, err := gojaEngine.Execute(ctx, transpiled, tc.globals)
			elapsed := time.Since(start)
			if err == nil {
				gojaTimes = append(gojaTimes, elapsed)
			}
		}
		result.GOJAStats = calculateStats(gojaTimes)

		// Benchmark Bun
		if hasBun {
			bunTimes := make([]time.Duration, 0, runs)

			// Warmup
			for i := 0; i < warmup; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				_, _ = bunEngine.Execute(ctx, tc.code, tc.globals)
				cancel()
			}

			// Memory measurement for Bun (single run after warmup)
			// Note: Bun runs in separate process, so we can only measure Go-side allocations
			runtime.GC()
			runtime.ReadMemStats(&memBefore)
			ctxMem, cancelMem := context.WithTimeout(context.Background(), 5*time.Second)
			_, _ = bunEngine.Execute(ctxMem, tc.code, tc.globals)
			cancelMem()
			runtime.ReadMemStats(&memAfter)
			result.BunMemory = &MemoryStats{
				AllocBytes: memAfter.TotalAlloc - memBefore.TotalAlloc,
				AllocCount: memAfter.Mallocs - memBefore.Mallocs,
			}

			// Measured runs (time only)
			for i := 0; i < runs; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				start := time.Now()
				_, err := bunEngine.Execute(ctx, tc.code, tc.globals)
				elapsed := time.Since(start)
				cancel()
				if err == nil {
					bunTimes = append(bunTimes, elapsed)
				}
			}
			result.BunStats = calculateStats(bunTimes)
		}

		results = append(results, result)
	}

	return results
}

func runConcurrencyBenchmarks(runs, warmup int, hasBun bool) []ConcurrencyResult {
	trans := transpiler.New()
	code := `
		const numbers = Array.from({ length: 100 }, (_, i) => i + 1);
		const evens = numbers.filter(n => n % 2 === 0);
		const doubled = evens.map(n => n * 2);
		const sum = doubled.reduce((a, b) => a + b, 0);
		export default sum;
	`

	transpiled, _, _, _, _ := trans.Transpile(code)

	gojaEngine := goja.New(goja.Config{PoolSize: 32})
	defer func() { _ = gojaEngine.Close() }()

	var bunEngine *bun.Engine
	if hasBun {
		var err error
		bunEngine, err = bun.New(bun.Config{PoolSize: 8})
		if err != nil || !bunEngine.IsAvailable() {
			hasBun = false
		} else {
			defer func() { _ = bunEngine.Close() }()
		}
	}

	ctx := context.Background()
	levels := []int{1, 4, 8, 16, 32}
	var results []ConcurrencyResult

	fmt.Fprintf(os.Stderr, "Benchmarking concurrency scaling...\n")

	for _, level := range levels {
		fmt.Fprintf(os.Stderr, "  Concurrency level: %d\n", level)

		result := ConcurrencyResult{Level: level}

		// GOJA concurrent
		gojaTimes := make([]time.Duration, 0, runs)

		// Warmup
		for i := 0; i < warmup; i++ {
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

		// Measured runs
		for i := 0; i < runs; i++ {
			var wg sync.WaitGroup
			wg.Add(level)
			start := time.Now()
			for j := 0; j < level; j++ {
				go func() {
					defer wg.Done()
					_, _ = gojaEngine.Execute(ctx, transpiled, nil)
				}()
			}
			wg.Wait()
			gojaTimes = append(gojaTimes, time.Since(start))
		}
		result.GOJAStats = calculateStats(gojaTimes)

		// Bun concurrent
		if hasBun {
			bunTimes := make([]time.Duration, 0, runs)

			// Warmup
			for i := 0; i < warmup; i++ {
				var wg sync.WaitGroup
				wg.Add(level)
				for j := 0; j < level; j++ {
					go func() {
						defer wg.Done()
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						_, _ = bunEngine.Execute(ctx, code, nil)
						cancel()
					}()
				}
				wg.Wait()
			}

			// Measured runs
			for i := 0; i < runs; i++ {
				var wg sync.WaitGroup
				wg.Add(level)
				start := time.Now()
				for j := 0; j < level; j++ {
					go func() {
						defer wg.Done()
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						_, _ = bunEngine.Execute(ctx, code, nil)
						cancel()
					}()
				}
				wg.Wait()
				bunTimes = append(bunTimes, time.Since(start))
			}
			result.BunStats = calculateStats(bunTimes)
		}

		results = append(results, result)
	}

	return results
}

func calculateStats(times []time.Duration) *Stats {
	if len(times) == 0 {
		return nil
	}

	// Sort for percentiles
	sorted := make([]time.Duration, len(times))
	copy(sorted, times)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	// Calculate mean
	var sum time.Duration
	for _, t := range times {
		sum += t
	}
	mean := sum / time.Duration(len(times))

	// Calculate standard deviation
	var variance float64
	for _, t := range times {
		diff := float64(t - mean)
		variance += diff * diff
	}
	variance /= float64(len(times))
	stdDev := time.Duration(math.Sqrt(variance))

	// Percentiles
	p50idx := len(sorted) / 2
	p95idx := int(float64(len(sorted)) * 0.95)
	if p95idx >= len(sorted) {
		p95idx = len(sorted) - 1
	}

	return &Stats{
		Mean:   mean,
		StdDev: stdDev,
		Min:    sorted[0],
		Max:    sorted[len(sorted)-1],
		P50:    sorted[p50idx],
		P95:    sorted[p95idx],
		Runs:   len(times),
	}
}

func generateMarkdown(results []BenchResult, concurrencyResults []ConcurrencyResult, runs int, hasBun bool, section string) string {
	var sb strings.Builder

	showAll := section == "" || section == "all"
	showComparison := showAll || section == "comparison"
	showConcurrency := showAll || section == "concurrency"
	showMemory := showAll || section == "memory"
	showDetailed := showAll || section == "detailed"

	// Sort results by test ID for consistent output
	sort.Slice(results, func(i, j int) bool {
		return results[i].TestID < results[j].TestID
	})

	// Header (only for full output)
	if showAll {
		sb.WriteString("# Benchmark Results\n\n")
		sb.WriteString(fmt.Sprintf("Statistical benchmark results from %d runs per test case.\n\n", runs))
		sb.WriteString(fmt.Sprintf("> **Environment:** %s %s (%s/%s)  \n", getCPUModel(), runtime.Version(), runtime.GOOS, runtime.GOARCH))
		sb.WriteString(fmt.Sprintf("> **Generated by:** `go run ./cmd/benchmark -runs=%d`  \n", runs))
		sb.WriteString("> **See also:** [Benchmark Suite](README.md) for test cases and analysis\n\n")
		sb.WriteString("---\n\n")
	}

	if showComparison {
		sb.WriteString("## Performance Comparison\n\n")
		sb.WriteString(fmt.Sprintf("_Statistical results from %d runs per test case (after warmup)._\n\n", runs))

		sb.WriteString("```\n")
		sb.WriteString(fmt.Sprintf("%-25s | %-15s | %-15s | %s\n", "Test Case", "GOJA", "Bun", "Winner"))
		sb.WriteString(strings.Repeat("-", 80) + "\n")

		for _, r := range results {
			gojaStr := "N/A"
			bunStr := "N/A"
			winner := "-"

			if r.GOJAStats != nil {
				gojaStr = formatDuration(r.GOJAStats.Mean)
			}
			if r.BunStats != nil {
				bunStr = formatDuration(r.BunStats.Mean)
			}

			if r.GOJAStats != nil && r.BunStats != nil {
				ratio := float64(r.GOJAStats.Mean) / float64(r.BunStats.Mean)
				if ratio > 1.1 {
					winner = fmt.Sprintf("Bun (%.1fx)", ratio)
				} else if ratio < 0.9 {
					winner = fmt.Sprintf("GOJA (%.1fx)", 1/ratio)
				} else {
					winner = "Tie"
				}
			} else if r.GOJAStats != nil {
				winner = "GOJA"
			}

			sb.WriteString(fmt.Sprintf("%-25s | %-15s | %-15s | %s\n", r.TestID, gojaStr, bunStr, winner))
		}

		sb.WriteString("```\n\n")
	}

	if showConcurrency {
		sb.WriteString("## Concurrency Scaling\n\n")
		sb.WriteString("```\n")
		sb.WriteString(fmt.Sprintf("%-12s | %-15s | %-15s\n", "Concurrency", "GOJA", "Bun"))
		sb.WriteString(strings.Repeat("-", 50) + "\n")

		for _, r := range concurrencyResults {
			gojaStr := "N/A"
			bunStr := "N/A"

			if r.GOJAStats != nil {
				gojaStr = formatDuration(r.GOJAStats.Mean)
			}
			if r.BunStats != nil {
				bunStr = formatDuration(r.BunStats.Mean)
			}

			sb.WriteString(fmt.Sprintf("%-12d | %-15s | %-15s\n", r.Level, gojaStr, bunStr))
		}

		sb.WriteString("```\n\n")
	}

	if showMemory {
		sb.WriteString("## Memory Usage\n\n")
		sb.WriteString("_Memory allocated per execution (Go-side allocations, single run after warmup)._\n\n")
		sb.WriteString("```\n")
		sb.WriteString(fmt.Sprintf("%-25s | %-15s | %-15s | %s\n", "Test Case", "GOJA", "Bun", "Winner"))
		sb.WriteString(strings.Repeat("-", 80) + "\n")

		for _, r := range results {
			gojaStr := "N/A"
			bunStr := "N/A"
			winner := "-"

			if r.GOJAMemory != nil {
				gojaStr = formatBytes(r.GOJAMemory.AllocBytes)
			}
			if r.BunMemory != nil {
				bunStr = formatBytes(r.BunMemory.AllocBytes)
			}

			if r.GOJAMemory != nil && r.BunMemory != nil && r.GOJAMemory.AllocBytes > 0 && r.BunMemory.AllocBytes > 0 {
				ratio := float64(r.GOJAMemory.AllocBytes) / float64(r.BunMemory.AllocBytes)
				if ratio > 1.1 {
					winner = fmt.Sprintf("Bun (%.1fx)", ratio)
				} else if ratio < 0.9 {
					winner = fmt.Sprintf("GOJA (%.1fx)", 1/ratio)
				} else {
					winner = "Tie"
				}
			} else if r.GOJAMemory != nil {
				winner = "GOJA"
			}

			sb.WriteString(fmt.Sprintf("%-25s | %-15s | %-15s | %s\n", r.TestID, gojaStr, bunStr, winner))
		}

		sb.WriteString("```\n\n")
	}

	if showDetailed {
		sb.WriteString("## Detailed Statistics\n\n")
		sb.WriteString("```\n")
		sb.WriteString(fmt.Sprintf("%-25s | %-12s | %-12s | %-12s | %-12s | %-12s\n",
			"Test Case", "Mean", "StdDev", "Min", "P50", "P95"))
		sb.WriteString(strings.Repeat("-", 100) + "\n")

		for _, r := range results {
			if r.GOJAStats != nil {
				sb.WriteString(fmt.Sprintf("%-25s | %-12s | %-12s | %-12s | %-12s | %-12s\n",
					"GOJA_"+r.TestID,
					formatDuration(r.GOJAStats.Mean),
					formatDuration(r.GOJAStats.StdDev),
					formatDuration(r.GOJAStats.Min),
					formatDuration(r.GOJAStats.P50),
					formatDuration(r.GOJAStats.P95),
				))
			}
			if r.BunStats != nil {
				sb.WriteString(fmt.Sprintf("%-25s | %-12s | %-12s | %-12s | %-12s | %-12s\n",
					"Bun_"+r.TestID,
					formatDuration(r.BunStats.Mean),
					formatDuration(r.BunStats.StdDev),
					formatDuration(r.BunStats.Min),
					formatDuration(r.BunStats.P50),
					formatDuration(r.BunStats.P95),
				))
			}
		}

		sb.WriteString("```\n")
	}

	return sb.String()
}

func getCPUModel() string {
	// Try to get CPU model on macOS
	if runtime.GOOS == "darwin" {
		cmd := exec.Command("sysctl", "-n", "machdep.cpu.brand_string")
		out, err := cmd.Output()
		if err == nil {
			model := strings.TrimSpace(string(out))
			return model
		}
	}
	// Try to get CPU info on Linux
	if runtime.GOOS == "linux" {
		cmd := exec.Command("sh", "-c", "grep 'model name' /proc/cpuinfo | head -1 | cut -d: -f2")
		out, err := cmd.Output()
		if err == nil {
			model := strings.TrimSpace(string(out))
			if model != "" {
				return model
			}
		}
	}
	return fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func formatDuration(d time.Duration) string {
	if d >= time.Second {
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
	if d >= time.Millisecond {
		return fmt.Sprintf("%.2fms", float64(d)/float64(time.Millisecond))
	}
	return fmt.Sprintf("~%dµs", d.Microseconds())
}
