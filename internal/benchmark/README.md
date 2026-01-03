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
simple_arithmetic         | ~68µs           | ~98µs           | GOJA (1.4x)
string_operations         | ~73µs           | ~61µs           | Bun (1.2x)
regex_operations          | ~89µs           | ~101µs          | GOJA (1.1x)
array_operations          | ~108µs          | ~166µs          | GOJA (1.5x)
type_guards               | ~88µs           | ~81µs           | Bun (1.1x)
generics                  | ~87µs           | ~100µs          | GOJA (1.1x)
object_manipulation       | ~97µs           | ~168µs          | GOJA (1.7x)
spread_destructure        | ~95µs           | ~132µs          | GOJA (1.4x)
with_globals              | ~79µs           | ~125µs          | GOJA (1.6x)
iterative_fibonacci       | ~129µs          | ~161µs          | GOJA (1.2x)
class_instantiation       | ~209µs          | ~240µs          | GOJA (1.1x)
closure_heavy             | ~203µs          | ~103µs          | Bun (2.0x)
json_processing           | ~305µs          | ~249µs          | Bun (1.2x)
nested_loops              | ~805µs          | ~285µs          | Bun (2.8x)
recursive_fibonacci       | ~1.4ms          | ~366µs          | Bun (3.8x)
```

### Concurrency Scaling

```
                    GOJA                              Bun
Concurrency  | Per Op      | Memory       | Per Op      | Memory
------------------------------------------------------------------------
1            | 126µs       | 105KB        | 104µs       | 1.6KB
4            | 331µs       | 430KB        | 166µs       | 6.4KB
8            | 583µs       | 859KB        | 239µs       | 12.7KB
16           | 1.04ms      | 1.7MB        | 509µs       | 25.4KB
32           | 1.74ms      | 3.5MB        | 1.30ms      | 50.8KB
```

### Raw Benchmark Results

**GOJA Engine:**

```
BenchmarkGOJA/GOJA_simple_arithmetic-14          17587      68199 ns/op      71943 B/op    1111 allocs/op
BenchmarkGOJA/GOJA_string_operations-14          16976      72854 ns/op      79154 B/op    1225 allocs/op
BenchmarkGOJA/GOJA_regex_operations-14           13828      89087 ns/op      96992 B/op    1382 allocs/op
BenchmarkGOJA/GOJA_array_operations-14           10000     107542 ns/op     104895 B/op    1638 allocs/op
BenchmarkGOJA/GOJA_type_guards-14                13516      87887 ns/op      93493 B/op    1414 allocs/op
BenchmarkGOJA/GOJA_generics-14                   13735      87453 ns/op      94040 B/op    1438 allocs/op
BenchmarkGOJA/GOJA_object_manipulation-14        12164      97374 ns/op     103484 B/op    1591 allocs/op
BenchmarkGOJA/GOJA_spread_destructure-14         12957      94615 ns/op     102527 B/op    1560 allocs/op
BenchmarkGOJA/GOJA_with_globals-14               15163      79070 ns/op      84639 B/op    1308 allocs/op
BenchmarkGOJA/GOJA_iterative_fibonacci-14         9232     129019 ns/op     176903 B/op    2367 allocs/op
BenchmarkGOJA/GOJA_class_instantiation-14         5430     208586 ns/op     184429 B/op    4120 allocs/op
BenchmarkGOJA/GOJA_closure_heavy-14               5877     203177 ns/op     246734 B/op    3302 allocs/op
BenchmarkGOJA/GOJA_json_processing-14             3843     305463 ns/op     335338 B/op    8058 allocs/op
BenchmarkGOJA/GOJA_nested_loops-14                1485     805102 ns/op     227835 B/op   19712 allocs/op
BenchmarkGOJA/GOJA_recursive_fibonacci-14          859    1393877 ns/op      76827 B/op    1208 allocs/op
```

**Bun Engine:**

```
BenchmarkBun/Bun_simple_arithmetic-14            11737      98078 ns/op       1403 B/op      25 allocs/op
BenchmarkBun/Bun_string_operations-14            19566      61132 ns/op       1919 B/op      34 allocs/op
BenchmarkBun/Bun_regex_operations-14             11744     101323 ns/op       2151 B/op      28 allocs/op
BenchmarkBun/Bun_array_operations-14              7738     166487 ns/op       1518 B/op      25 allocs/op
BenchmarkBun/Bun_type_guards-14                  13054      81046 ns/op       1874 B/op      25 allocs/op
BenchmarkBun/Bun_generics-14                     13665      99731 ns/op       2878 B/op      57 allocs/op
BenchmarkBun/Bun_object_manipulation-14           6230     168241 ns/op       3952 B/op      73 allocs/op
BenchmarkBun/Bun_spread_destructure-14            8182     131574 ns/op       3196 B/op      56 allocs/op
BenchmarkBun/Bun_with_globals-14                 11743     125337 ns/op       2550 B/op      36 allocs/op
BenchmarkBun/Bun_iterative_fibonacci-14           6962     161450 ns/op       1468 B/op      25 allocs/op
BenchmarkBun/Bun_class_instantiation-14           4420     240380 ns/op       1873 B/op      25 allocs/op
BenchmarkBun/Bun_closure_heavy-14                10000     103235 ns/op       1810 B/op      25 allocs/op
BenchmarkBun/Bun_json_processing-14               4725     248807 ns/op       2102 B/op      31 allocs/op
BenchmarkBun/Bun_nested_loops-14                  3909     284745 ns/op       1420 B/op      25 allocs/op
BenchmarkBun/Bun_recursive_fibonacci-14           3680     366155 ns/op       1389 B/op      25 allocs/op
BenchmarkBun/Bun_promise_chain-14                  267    4676565 ns/op       1615 B/op      25 allocs/op
BenchmarkBun/Bun_async_parallel-14                 660    1823522 ns/op       1681 B/op      25 allocs/op
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
