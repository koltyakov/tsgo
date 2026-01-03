# tsgo Benchmark Suite

Performance benchmarks comparing TypeScript execution engines.

## Engines

| Engine | Type | Async Support | Status |
|--------|------|---------------|--------|
| **GOJA** | Pure Go JS runtime | ❌ No | ✅ Ready |
| **Bun** | External process (V8) | ✅ Yes | ✅ Ready |
| **WASM** | QuickJS in wazero | ❌ No | ⚠️ Requires QuickJS WASM binary |

### Why WASM is not included in benchmarks

The WASM engine requires a QuickJS WASM binary (`quickjs.wasm`) that must be compiled separately. This binary is not included in the repository because:

1. **Build complexity**: QuickJS must be compiled to WASM using Emscripten or similar toolchain
2. **Binary size**: The compiled WASM module is ~1-2MB
3. **Licensing**: QuickJS is MIT licensed but binary distribution adds complexity

To enable the WASM engine:
```bash
# Build QuickJS to WASM (requires Emscripten)
git clone https://github.com/nicofff/nicofff.quickjs
cd nicofff.quickjs
make wasm
cp build/quickjs.wasm /path/to/tsgo/internal/engine/wasm/
```

Once the binary is in place, the WASM engine will be automatically available.

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

| Category | Test Cases |
|----------|------------|
| **Basic** | Simple arithmetic, string operations |
| **Collections** | Array operations (filter/map/reduce), object manipulation |
| **Algorithms** | Recursive Fibonacci, iterative Fibonacci, nested loops |
| **Data Processing** | JSON stringify/parse, regex operations |
| **OOP** | ES6 class instantiation, closures |
| **ES6+ Features** | Spread/destructuring, generics |
| **TypeScript** | Type guards, discriminated unions |
| **Async** | Promise chains, Promise.all (Bun only) |
| **Integration** | Global variable injection |

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

### Performance Comparison (100 iterations average)

```
Test Case                 | GOJA            | Bun             | Winner
--------------------------------------------------------------------------------
simple_arithmetic         | ~65µs           | ~100µs          | GOJA
string_operations         | ~75µs           | ~98µs           | GOJA
regex_operations          | ~90µs           | ~87µs           | Bun
array_operations          | ~106µs          | ~156µs          | GOJA
type_guards               | ~88µs           | ~95µs           | GOJA
generics                  | ~87µs           | ~83µs           | Bun
object_manipulation       | ~96µs           | ~93µs           | Bun
spread_destructure        | ~92µs           | ~62µs           | Bun
with_globals              | ~83µs           | ~83µs           | Tie
iterative_fibonacci       | ~130µs          | ~71µs           | Bun
class_instantiation       | ~227µs          | ~129µs          | Bun
closure_heavy             | ~206µs          | ~102µs          | Bun
json_processing           | ~306µs          | ~107µs          | Bun
nested_loops              | ~827µs          | ~280µs          | Bun
recursive_fibonacci       | ~1.4ms          | ~222µs          | Bun
```

### GOJA Concurrency Scaling

```
Concurrency     | Total Time      | Per Op         
--------------------------------------------------
1               | 54.7ms          | 1.09ms      
2               | 47.1ms          | 471µs      
4               | 64.4ms          | 322µs      
8               | 105.6ms         | 264µs      
16              | 191.9ms         | 240µs      
32              | 323.5ms         | 202µs      
```

### Raw Benchmark Results (GOJA)

```
BenchmarkGOJA/GOJA_simple_arithmetic-14          19033      64797 ns/op
BenchmarkGOJA/GOJA_string_operations-14          15643      75306 ns/op
BenchmarkGOJA/GOJA_regex_operations-14           13110      89556 ns/op
BenchmarkGOJA/GOJA_array_operations-14           10000     106041 ns/op
BenchmarkGOJA/GOJA_type_guards-14                13771      87968 ns/op
BenchmarkGOJA/GOJA_generics-14                   13645      87039 ns/op
BenchmarkGOJA/GOJA_object_manipulation-14        12495      95953 ns/op
BenchmarkGOJA/GOJA_spread_destructure-14         12423      92046 ns/op
BenchmarkGOJA/GOJA_with_globals-14               14716      82827 ns/op
BenchmarkGOJA/GOJA_iterative_fibonacci-14         9505     129472 ns/op
BenchmarkGOJA/GOJA_class_instantiation-14         5168     226721 ns/op
BenchmarkGOJA/GOJA_closure_heavy-14               5790     206225 ns/op
BenchmarkGOJA/GOJA_json_processing-14             3926     305661 ns/op
BenchmarkGOJA/GOJA_nested_loops-14                1460     827458 ns/op
BenchmarkGOJA/GOJA_recursive_fibonacci-14          858    1390763 ns/op
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

### When to use WASM (Sandboxed)

✅ **Best for:**
- Security-critical environments requiring full sandboxing
- Untrusted code execution with strict memory limits
- Environments without external runtime dependencies
- Reproducible execution across platforms

❌ **Limitations:**
- Requires QuickJS WASM binary (not included)
- Slower than native execution
- No async support (QuickJS limitation)
- Memory overhead for WASM runtime

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
                    ┌─────────┐         ┌─────────────────────┐
                    │   Bun   │         │ Need sandboxing?    │
                    └─────────┘         └─────────────────────┘
                                                  │
                                      ┌───────────┴───────────┐
                                      │                       │
                                     Yes                      No
                                      │                       │
                                      ▼                       ▼
                                ┌─────────┐             ┌─────────┐
                                │  WASM   │             │  GOJA   │
                                └─────────┘             └─────────┘
```
