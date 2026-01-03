# Statistical Benchmark Runner

A statistical benchmark tool for comparing tsgo TypeScript execution engines.
Runs each test multiple times and outputs reproducible markdown with statistical analysis.

## Usage

```bash
# Run with default 15 runs per test case
go run ./cmd/benchmark

# Run with custom number of runs
go run ./cmd/benchmark -runs=100

# Save results to file
go run ./cmd/benchmark -runs=100 -output=results.md

# Output specific sections
go run ./cmd/benchmark -section=comparison   # Performance table only
go run ./cmd/benchmark -section=concurrency  # Concurrency scaling only
go run ./cmd/benchmark -section=memory       # Memory usage only
go run ./cmd/benchmark -section=detailed     # Full statistics only

# Using make targets
make benchmark-stats      # Print to stdout (15 runs)
make benchmark-stats-md   # Save to internal/benchmark/Results.md
```

## Flags

| Flag       | Default | Description                                              |
|:-----------|:--------|:---------------------------------------------------------|
| `-runs`    | 15      | Number of benchmark runs per test case                   |
| `-warmup`  | 3       | Number of warmup runs before measurement                 |
| `-output`  | stdout  | Output file for markdown results                         |
| `-section` | all     | Output section: `comparison`, `concurrency`, `memory`, `detailed`, `all` |

## Output Sections

The tool generates four sections of markdown tables:

1. **Performance Comparison** - Mean execution time per test with winner and speedup ratio
2. **Concurrency Scaling** - How engines scale from 1 to 32 concurrent executions
3. **Memory Usage** - Go-side memory allocations per execution
4. **Detailed Statistics** - Mean, StdDev, Min, P50 (median), P95 per test

## Key Insights

- **GOJA wins** on simple/lightweight operations: arithmetic, strings, regex, object manipulation
- **Bun wins** on CPU-intensive work: recursive fibonacci (~7x), nested loops (~3x), JSON processing (~2.5x)
- **Concurrency**: Bun scales better at high concurrency (16+ workers)
- **Cold start**: Bun ~127ms vs GOJA ~71µs - always reuse engine instances!

## See Also

- [Benchmark Suite](../../internal/benchmark/README.md) - Test cases, cold start analysis, engine selection guide
- [Statistical Results](../../internal/benchmark/Results.md) - Full 100-run results with mean, stddev, percentiles
