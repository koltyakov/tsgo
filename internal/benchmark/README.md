# tsgo Benchmark Suite

Performance benchmarks comparing TypeScript execution engines.

## Engines

| Engine   | Type                  | Async Support | Status   |
| -------- | --------------------- | ------------- | -------- |
| **GOJA** | Pure Go JS runtime    | ❌ No         | ✅ Ready |
| **Bun**  | External process (V8) | ✅ Yes        | ✅ Ready |

## Running Benchmarks

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
simple_arithmetic         | ~54µs           | ~62µs           | GOJA (1.1x)
string_operations         | ~61µs           | ~80µs           | GOJA (1.3x)
regex_operations          | ~74µs           | ~175µs          | GOJA (2.4x)
array_operations          | ~92µs           | ~83µs           | Bun (1.1x)
type_guards               | ~74µs           | ~319µs          | GOJA (4.3x)
generics                  | ~73µs           | ~352µs          | GOJA (4.8x)
object_manipulation       | ~78µs           | ~314µs          | GOJA (4.0x)
spread_destructure        | ~77µs           | ~107µs          | GOJA (1.4x)
with_globals              | ~66µs           | ~247µs          | GOJA (3.7x)
iterative_fibonacci       | ~117µs          | ~116µs          | Tie
class_instantiation       | ~204µs          | ~491µs          | GOJA (2.4x)
closure_heavy             | ~196µs          | ~440µs          | GOJA (2.2x)
json_processing           | ~298µs          | ~409µs          | GOJA (1.4x)
nested_loops              | ~818µs          | ~724µs          | Bun (1.1x)
recursive_fibonacci       | ~1.4ms          | ~249µs          | Bun (5.6x)
```

### Concurrency Scaling

```
                    GOJA                              Bun
Concurrency  | Per Op      | Memory       | Per Op      | Memory
------------------------------------------------------------------------
1            | 114µs       | 105KB        | 102µs       | 1.5KB
4            | 296µs       | 426KB        | 195µs       | 6.2KB
8            | 565µs       | 855KB        | 472µs       | 12.3KB
16           | 983µs       | 1.7MB        | 1.34ms      | 24.7KB
32           | 1.49ms      | 3.4MB        | 1.81ms      | 49.3KB
```

### Raw Benchmark Results

**GOJA Engine:**

```
BenchmarkGOJA/GOJA_simple_arithmetic-14          21900      54242 ns/op      70899 B/op    1105 allocs/op
BenchmarkGOJA/GOJA_string_operations-14          19386      61391 ns/op      78983 B/op    1219 allocs/op
BenchmarkGOJA/GOJA_regex_operations-14           15972      73025 ns/op      96530 B/op    1376 allocs/op
BenchmarkGOJA/GOJA_array_operations-14           13062      88931 ns/op     104638 B/op    1632 allocs/op
BenchmarkGOJA/GOJA_type_guards-14                16696      71695 ns/op      93247 B/op    1408 allocs/op
BenchmarkGOJA/GOJA_generics-14                   16425      71349 ns/op      93858 B/op    1432 allocs/op
BenchmarkGOJA/GOJA_object_manipulation-14        15160      78188 ns/op     101606 B/op    1585 allocs/op
BenchmarkGOJA/GOJA_spread_destructure-14         15328      76646 ns/op     102320 B/op    1554 allocs/op
BenchmarkGOJA/GOJA_with_globals-14               17049      64940 ns/op      84415 B/op    1302 allocs/op
BenchmarkGOJA/GOJA_iterative_fibonacci-14        10000     115753 ns/op     175522 B/op    2361 allocs/op
BenchmarkGOJA/GOJA_class_instantiation-14         5926     199938 ns/op     184232 B/op    4114 allocs/op
BenchmarkGOJA/GOJA_closure_heavy-14               5916     196887 ns/op     245796 B/op    3296 allocs/op
BenchmarkGOJA/GOJA_json_processing-14             3856     294808 ns/op     333990 B/op    8051 allocs/op
BenchmarkGOJA/GOJA_nested_loops-14                1447     818351 ns/op     226377 B/op   19706 allocs/op
BenchmarkGOJA/GOJA_recursive_fibonacci-14          849    1396540 ns/op      76546 B/op    1202 allocs/op
```

**Bun Engine:**

```
BenchmarkBun/Bun_simple_arithmetic-14            21590      57548 ns/op       1369 B/op      25 allocs/op
BenchmarkBun/Bun_string_operations-14            15195      81433 ns/op       1885 B/op      34 allocs/op
BenchmarkBun/Bun_regex_operations-14              8479     136841 ns/op       2109 B/op      28 allocs/op
BenchmarkBun/Bun_array_operations-14             13971      88016 ns/op       1485 B/op      25 allocs/op
BenchmarkBun/Bun_type_guards-14                   3278     369322 ns/op       1846 B/op      25 allocs/op
BenchmarkBun/Bun_generics-14                      3019     365683 ns/op       2842 B/op      57 allocs/op
BenchmarkBun/Bun_object_manipulation-14           3950     324520 ns/op       3916 B/op      73 allocs/op
BenchmarkBun/Bun_spread_destructure-14           10000     100454 ns/op       3160 B/op      56 allocs/op
BenchmarkBun/Bun_with_globals-14                  4342     268869 ns/op       2223 B/op      35 allocs/op
BenchmarkBun/Bun_iterative_fibonacci-14          10000     115345 ns/op       1435 B/op      25 allocs/op
BenchmarkBun/Bun_class_instantiation-14           1818     591823 ns/op       1835 B/op      25 allocs/op
BenchmarkBun/Bun_closure_heavy-14                 3392     512221 ns/op       1774 B/op      25 allocs/op
BenchmarkBun/Bun_json_processing-14               3092     403749 ns/op       2069 B/op      31 allocs/op
BenchmarkBun/Bun_nested_loops-14                  1807     701852 ns/op       1388 B/op      25 allocs/op
BenchmarkBun/Bun_recursive_fibonacci-14           4551     262041 ns/op       1354 B/op      25 allocs/op
BenchmarkBun/Bun_promise_chain-14                  194    6116226 ns/op       2342 B/op      26 allocs/op
BenchmarkBun/Bun_async_parallel-14                 487    2309433 ns/op       1640 B/op      25 allocs/op
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
