# tsgo Benchmark Suite

Performance benchmarks comparing TypeScript execution engines.

> **Quick Start:** Use the [Statistical Benchmark Runner](../../cmd/benchmark/README.md) for reproducible markdown output.

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
go run ./cmd/benchmark -runs=15 -output=internal/benchmark/Results.md

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

Results from Apple M4 Pro (darwin/arm64). For full statistical data with mean, stddev, and percentiles, see [Results.md](Results.md).

### Feature Support Matrix

Both engines support all synchronous TypeScript/ES6+ features:

| Feature | GOJA | Bun | Notes |
|:--------|:----:|:---:|:------|
| ES6+ syntax | ✅ | ✅ | Classes, spread, destructuring, generics |
| TypeScript | ✅ | ✅ | GOJA via esbuild, Bun native |
| Global injection | ✅ | ✅ | Pass Go values to scripts |
| Async/await | ❌ | ✅ | Bun only |
| Promises | ❌ | ✅ | Bun only |

### Performance Summary

| Workload Type | Winner | Speedup | Examples |
|:--------------|:-------|:--------|:---------|
| Simple operations | GOJA | 1.3-2.7x | arithmetic, strings, regex |
| Object/array work | GOJA | 1.4-1.8x | manipulation, spread, type guards |
| CPU-intensive | Bun | 1.5-6x | fibonacci, nested loops, JSON |
| High concurrency | Bun | 1.6-2.2x | 16-32 concurrent workers |

### Cold Start vs Warm Reuse

⚠️ **Critical:** Always reuse engine instances in production.

| Engine | Cold Start | Warm Reuse | Ratio |
|:-------|:-----------|:-----------|:------|
| GOJA   | ~71µs      | ~53µs      | 1.3x  |
| Bun    | **~127ms** | ~51µs      | 2,500x |

**Bun optimization:** Use `BackgroundWarmup: true` to parallelize startup:

```go
engine, _ := bun.New(bun.Config{
    BackgroundWarmup: true, // New() returns immediately
})
// Do other initialization here while Bun starts (~120ms in background)
// First Execute() call waits if process isn't ready
```

### Raw Benchmark Output

Run `go test -bench` to get raw Go benchmark output with memory stats:

```bash
go test ./internal/benchmark/... -bench=. -benchmem -benchtime=1s -run=NONE
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
- Long-running services where ~127ms startup cost is amortized
- High concurrency (process pool scales better than GOJA at 16+ workers)

❌ **Limitations:**

- **Cold start: ~127ms** - must reuse engine instances
- IPC overhead (~50-100µs per call)
- Requires Bun installed on system
- Process pool management complexity
- Not suitable for serverless/cold-start-sensitive environments

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
