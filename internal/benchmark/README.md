# tsgo Benchmark Suite

Performance benchmarks comparing TypeScript execution engines.

## Engines

| Engine   | Type                  | Async Support | Status   |
| -------- | --------------------- | ------------- | -------- |
| **GOJA** | Pure Go JS runtime    | ❌ No         | ✅ Ready |
| **Bun**  | External process (V8) | ✅ Yes        | ✅ Ready |

## Running Benchmarks

### Statistical Benchmark Runner (Recommended)

The statistical benchmark runner executes each test multiple times and calculates
mean, standard deviation, min, p50, and p95 values. It outputs reproducible
markdown that can be directly copied to documentation.

```bash
# Run statistical benchmarks (15 runs per test case, outputs markdown)
go run ./cmd/benchmark -runs=15

# Save results to file
go run ./cmd/benchmark -runs=15 -output=internal/benchmark/RESULTS.md

# Using make targets
make benchmark-stats      # Print to stdout
make benchmark-stats-md   # Save to file

# Output specific sections only
go run ./cmd/benchmark -runs=15 -section=comparison   # Performance table only
go run ./cmd/benchmark -runs=15 -section=concurrency  # Concurrency scaling only
go run ./cmd/benchmark -runs=15 -section=detailed     # Full statistics only
```

### Quick Tests (Human-readable output)

```bash
# Feature support matrix - shows what each engine supports
go test ./internal/benchmark/... -run TestFeatureSupport -v

# Performance comparison - 100 iterations per test case
go test ./internal/benchmark/... -run TestPerformanceComparison -v

# Concurrency scaling - tests GOJA pool from 1-32 concurrent executions
go test ./internal/benchmark/... -run TestConcurrencyScaling -v
```

### Go Benchmarks

```bash
# GOJA engine benchmarks
go test ./internal/benchmark/... -bench=BenchmarkGOJA -benchtime=1s -run=NONE

# Bun engine benchmarks
go test ./internal/benchmark/... -bench=BenchmarkBun -benchtime=500ms -run=NONE

# Concurrent execution benchmarks
go test ./internal/benchmark/... -bench=BenchmarkConcurrent -benchtime=500ms -run=NONE

# Transpiler benchmarks (esbuild)
go test ./internal/benchmark/... -bench=BenchmarkTranspiler -benchtime=500ms -run=NONE

# All benchmarks with memory stats
go test ./internal/benchmark/... -bench=. -benchmem -benchtime=1s -run=NONE
```

## Test Cases

The benchmark suite includes 17 test scenarios:

| Category            | Test Cases                                                |
| ------------------- | --------------------------------------------------------- |
| **Basic**           | Simple arithmetic, string operations                      |
| **Collections**     | Array operations (filter/map/reduce), object manipulation |
| **Algorithms**      | Recursive Fibonacci, iterative Fibonacci, nested loops    |
| **Data Processing** | JSON stringify/parse, regex operations                    |
| **OOP**             | ES6 class instantiation, closures                         |
| **ES6+ Features**   | Spread/destructuring, generics                            |
| **TypeScript**      | Type guards, discriminated unions                         |
| **Async**           | Promise chains, Promise.all (Bun only)                    |
| **Integration**     | Global variable injection                                 |

## Benchmark Results

Results from Apple M4 Pro (darwin/arm64):

### Feature Support Matrix

```
Feature                   | GOJA       | Bun        | Description
--------------------------------------------------------------------------------
simple_arithmetic         | ✅          | ✅          | Basic math operations
string_operations         | ✅          | ✅          | String manipulation
array_operations          | ✅          | ✅          | Array filter/map/reduce on 100 items
object_manipulation       | ✅          | ✅          | Object spread, reduce to map
recursive_fibonacci       | ✅          | ✅          | Recursive function (fib 20 = 6765)
iterative_fibonacci       | ✅          | ✅          | Iterative function with destructuring
json_processing           | ✅          | ✅          | JSON stringify/parse with 50 objects
regex_operations          | ✅          | ✅          | Regular expression matching and filtering
class_instantiation       | ✅          | ✅          | ES6 class with methods, 100 instances
nested_loops              | ✅          | ✅          | 10,000 iterations nested loop
closure_heavy             | ✅          | ✅          | 50 closures with state
spread_destructure        | ✅          | ✅          | Array/object spread and destructuring
type_guards               | ✅          | ✅          | TypeScript discriminated unions
generics                  | ✅          | ✅          | Generic functions
with_globals              | ✅          | ✅          | Using injected global variables
promise_chain             | N/A        | ✅          | Async promise chain (Bun only)
async_parallel            | N/A        | ✅          | Parallel async execution (Bun only)
```

### Performance Comparison

```
Test Case                 | GOJA            | Bun             | Winner
--------------------------------------------------------------------------------
simple_arithmetic         | ~55µs           | ~70µs           | GOJA (1.3x)
string_operations         | ~70µs           | ~70µs           | Tie
regex_operations          | ~73µs           | ~109µs          | GOJA (1.5x)
array_operations          | ~91µs           | ~144µs          | GOJA (1.6x)
type_guards               | ~74µs           | ~103µs          | GOJA (1.4x)
generics                  | ~70µs           | ~83µs           | GOJA (1.2x)
object_manipulation       | ~82µs           | ~142µs          | GOJA (1.7x)
spread_destructure        | ~76µs           | ~109µs          | GOJA (1.4x)
with_globals              | ~64µs           | ~117µs          | GOJA (1.8x)
iterative_fibonacci       | ~115µs          | ~125µs          | GOJA (1.1x)
class_instantiation       | ~209µs          | ~294µs          | GOJA (1.4x)
closure_heavy             | ~190µs          | ~110µs          | Bun (1.7x)
json_processing           | ~281µs          | ~188µs          | Bun (1.5x)
nested_loops              | ~827µs          | ~546µs          | Bun (1.5x)
recursive_fibonacci       | ~1.4ms          | ~297µs          | Bun (4.7x)
```

### Concurrency Scaling

```
                    GOJA                              Bun
Concurrency  | Per Op      | Memory       | Per Op      | Memory
------------------------------------------------------------------------
1            | 114µs       | 105KB        | 101µs       | 1.5KB
4            | 297µs       | 426KB        | 164µs       | 6.2KB
8            | 565µs       | 856KB        | 248µs       | 12.4KB
16           | 974µs       | 1.7MB        | 506µs       | 24.7KB
32           | 1.45ms      | 3.4MB        | 1.11ms      | 49.4KB
```

### Cold Start vs Warm Reuse

This benchmark demonstrates the critical importance of reusing engine instances
rather than creating new ones per request.

```
Benchmark                 | Time          | Memory    | Allocs
--------------------------------------------------------------------------------
GOJA_cold_start           | 72µs          | 101KB     | 1,380
GOJA_warm_reuse           | 57µs          | 71KB      | 1,105
Bun_cold_start            | 126ms         | 23KB      | 150
Bun_warm_reuse            | 50µs          | 1.3KB     | 24
```

**Key insight:** Bun cold start is **2,500x slower** than warm reuse (126ms vs 50µs).
Always reuse engine instances in long-running applications!

Use `BackgroundWarmup: true` to parallelize Bun startup with other initialization:
```go
engine, _ := bun.New(bun.Config{
    BackgroundWarmup: true, // New() returns immediately, ~120ms startup happens in background
})
```

### Raw Benchmark Results

**GOJA Engine:**

```
BenchmarkGOJA/GOJA_simple_arithmetic-14          22102      54556 ns/op      70907 B/op    1105 allocs/op
BenchmarkGOJA/GOJA_string_operations-14          20011      69812 ns/op      78925 B/op    1219 allocs/op
BenchmarkGOJA/GOJA_regex_operations-14           16129      73070 ns/op      97124 B/op    1377 allocs/op
BenchmarkGOJA/GOJA_array_operations-14           13036      91398 ns/op     104664 B/op    1632 allocs/op
BenchmarkGOJA/GOJA_type_guards-14                14574      73616 ns/op      93303 B/op    1408 allocs/op
BenchmarkGOJA/GOJA_generics-14                   16856      69779 ns/op      93819 B/op    1432 allocs/op
BenchmarkGOJA/GOJA_object_manipulation-14        15237      82283 ns/op     101674 B/op    1585 allocs/op
BenchmarkGOJA/GOJA_spread_destructure-14         15633      75900 ns/op     102234 B/op    1554 allocs/op
BenchmarkGOJA/GOJA_with_globals-14               18548      63904 ns/op      84444 B/op    1302 allocs/op
BenchmarkGOJA/GOJA_iterative_fibonacci-14        10000     114880 ns/op     175526 B/op    2361 allocs/op
BenchmarkGOJA/GOJA_class_instantiation-14         6103     209225 ns/op     184153 B/op    4114 allocs/op
BenchmarkGOJA/GOJA_closure_heavy-14               6238     189893 ns/op     245854 B/op    3296 allocs/op
BenchmarkGOJA/GOJA_json_processing-14             3948     280627 ns/op     333928 B/op    8051 allocs/op
BenchmarkGOJA/GOJA_nested_loops-14                1465     827091 ns/op     226452 B/op   19706 allocs/op
BenchmarkGOJA/GOJA_recursive_fibonacci-14          859    1402726 ns/op      76563 B/op    1202 allocs/op
```

**Bun Engine:**

```
BenchmarkBun/Bun_simple_arithmetic-14            18867      69829 ns/op       1369 B/op      25 allocs/op
BenchmarkBun/Bun_string_operations-14            18396      70049 ns/op       1884 B/op      34 allocs/op
BenchmarkBun/Bun_regex_operations-14             11150     109260 ns/op       2111 B/op      28 allocs/op
BenchmarkBun/Bun_array_operations-14              9093     144187 ns/op       1484 B/op      25 allocs/op
BenchmarkBun/Bun_type_guards-14                  10887     102976 ns/op       1837 B/op      25 allocs/op
BenchmarkBun/Bun_generics-14                     14472      83473 ns/op       2835 B/op      57 allocs/op
BenchmarkBun/Bun_object_manipulation-14           7237     142105 ns/op       3916 B/op      73 allocs/op
BenchmarkBun/Bun_spread_destructure-14           11251     108958 ns/op       3160 B/op      56 allocs/op
BenchmarkBun/Bun_with_globals-14                 10862     116842 ns/op       2223 B/op      35 allocs/op
BenchmarkBun/Bun_iterative_fibonacci-14          10408     125348 ns/op       1435 B/op      25 allocs/op
BenchmarkBun/Bun_class_instantiation-14           4560     293715 ns/op       1839 B/op      25 allocs/op
BenchmarkBun/Bun_closure_heavy-14                10926     110330 ns/op       1773 B/op      25 allocs/op
BenchmarkBun/Bun_json_processing-14               6564     188238 ns/op       2071 B/op      31 allocs/op
BenchmarkBun/Bun_nested_loops-14                  2274     546141 ns/op       1387 B/op      25 allocs/op
BenchmarkBun/Bun_recursive_fibonacci-14           4144     296696 ns/op       1355 B/op      25 allocs/op
BenchmarkBun/Bun_promise_chain-14                  250    4850060 ns/op       1559 B/op      25 allocs/op
BenchmarkBun/Bun_async_parallel-14                 508    2554001 ns/op       1661 B/op      25 allocs/op
```

## Key Findings

### When to use GOJA (Pure Go)

✅ **Best for:**

- Simple expressions and lightweight operations
- High-concurrency scenarios (scales well with goroutines)
- Environments where external dependencies are not allowed
- Embedding in Go applications without CGO
- Short-lived executions with low latency requirements

❌ **Limitations:**

- No async/await support
- Slower for CPU-intensive operations
- No native TypeScript (requires transpilation)

### When to use Bun (External V8)

✅ **Best for:**

- CPU-intensive computations (recursive algorithms, large loops)
- Async/await and Promise-based code
- Complex TypeScript with native execution
- Long-running scripts where startup cost is amortized

❌ **Limitations:**

- IPC overhead (~50-100µs per call)
- Requires Bun installed on system
- Process pool management complexity
- Not suitable for high-frequency short operations

## Engine Selection Guide

```
┌─────────────────────────────────────────────────────────────┐
│                    Need async/await?                        │
└─────────────────────────────────────────────────────────────┘
                          │
              ┌───────────┴───────────┐
              │                       │
             Yes                      No
              │                       │
              ▼                       ▼
        ┌─────────┐         ┌─────────────────────┐
        │   Bun   │         │ CPU-intensive work? │
        └─────────┘         └─────────────────────┘
                                      │
                          ┌───────────┴───────────┐
                          │                       │
                         Yes                      No
                          │                       │
                          ▼                       ▼
                    ┌─────────┐             ┌─────────┐
                    │   Bun   │             │  GOJA   │
                    └─────────┘             └─────────┘
```
