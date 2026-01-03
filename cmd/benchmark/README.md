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
go run ./cmd/benchmark -section=detailed     # Full statistics only

# Using make targets
make benchmark-stats      # Print to stdout (15 runs)
make benchmark-stats-md   # Save to internal/benchmark/RESULTS.md
```

## Flags

| Flag       | Default | Description                                         |
| ---------- | ------- | --------------------------------------------------- |
| `-runs`    | 15      | Number of benchmark runs per test case              |
| `-warmup`  | 3       | Number of warmup runs before measurement            |
| `-output`  | stdout  | Output file for markdown results                    |
| `-section` | all     | Output section: `comparison`, `concurrency`, `detailed`, `all` |

## Output

The tool generates three sections of markdown:

1. **Performance Comparison** - Mean execution time per test with winner and speedup ratio
2. **Concurrency Scaling** - How engines scale from 1 to 32 concurrent executions
3. **Detailed Statistics** - Mean, StdDev, Min, P50 (median), P95 per test

## Benchmark Results (100 runs)

Results from Apple M4 Pro (darwin/arm64):

### Performance Comparison

_Statistical results from 100 runs per test case (after warmup)._

```
Test Case                 | GOJA            | Bun             | Winner
--------------------------------------------------------------------------------
array_operations          | ~101µs          | ~134µs          | GOJA (1.3x)
class_instantiation       | ~194µs          | ~131µs          | Bun (1.5x)
closure_heavy             | ~178µs          | ~113µs          | Bun (1.6x)
generics                  | ~70µs           | ~82µs           | GOJA (1.2x)
iterative_fibonacci       | ~117µs          | ~96µs           | Bun (1.2x)
json_processing           | ~287µs          | ~114µs          | Bun (2.5x)
nested_loops              | ~791µs          | ~275µs          | Bun (2.9x)
object_manipulation       | ~79µs           | ~134µs          | GOJA (1.7x)
recursive_fibonacci       | 1.37ms          | ~199µs          | Bun (6.9x)
regex_operations          | ~70µs           | ~72µs           | Tie
simple_arithmetic         | ~61µs           | ~165µs          | GOJA (2.7x)
spread_destructure        | ~74µs           | ~68µs           | Tie
string_operations         | ~63µs           | ~115µs          | GOJA (1.8x)
type_guards               | ~71µs           | ~111µs          | GOJA (1.6x)
with_globals              | ~65µs           | ~100µs          | GOJA (1.5x)
```

### Concurrency Scaling

```
Concurrency  | GOJA            | Bun            
--------------------------------------------------
1            | ~123µs          | ~234µs         
4            | ~309µs          | ~245µs         
8            | ~588µs          | ~291µs         
16           | ~984µs          | ~369µs         
32           | 1.45ms          | ~895µs         
```

### Detailed Statistics

```
Test Case                 | Mean         | StdDev       | Min          | P50          | P95         
----------------------------------------------------------------------------------------------------
GOJA_array_operations     | ~101µs       | ~99µs        | ~63µs        | ~79µs        | ~313µs      
Bun_array_operations      | ~134µs       | ~71µs        | ~65µs        | ~121µs       | ~219µs      
GOJA_class_instantiation  | ~194µs       | ~89µs        | ~148µs       | ~168µs       | ~450µs      
Bun_class_instantiation   | ~131µs       | ~63µs        | ~76µs        | ~118µs       | ~288µs      
GOJA_closure_heavy        | ~178µs       | ~89µs        | ~132µs       | ~145µs       | ~445µs      
Bun_closure_heavy         | ~113µs       | ~46µs        | ~74µs        | ~104µs       | ~152µs      
GOJA_generics             | ~70µs        | ~56µs        | ~48µs        | ~58µs        | ~233µs      
Bun_generics              | ~82µs        | ~57µs        | ~44µs        | ~67µs        | ~207µs      
GOJA_iterative_fibonacci  | ~117µs       | ~104µs       | ~73µs        | ~84µs        | ~349µs      
Bun_iterative_fibonacci   | ~96µs        | ~45µs        | ~53µs        | ~89µs        | ~162µs      
GOJA_json_processing      | ~287µs       | ~107µs       | ~205µs       | ~248µs       | ~542µs      
Bun_json_processing       | ~114µs       | ~55µs        | ~82µs        | ~99µs        | ~220µs      
GOJA_nested_loops         | ~791µs       | ~98µs        | ~703µs       | ~763µs       | 1.04ms      
Bun_nested_loops          | ~275µs       | ~37µs        | ~225µs       | ~267µs       | ~328µs      
GOJA_object_manipulation  | ~79µs        | ~50µs        | ~52µs        | ~63µs        | ~226µs      
Bun_object_manipulation   | ~134µs       | ~65µs        | ~76µs        | ~120µs       | ~203µs      
GOJA_recursive_fibonacci  | 1.37ms       | ~65µs        | 1.26ms       | 1.36ms       | 1.46ms      
Bun_recursive_fibonacci   | ~199µs       | ~31µs        | ~157µs       | ~192µs       | ~239µs      
GOJA_regex_operations     | ~70µs        | ~55µs        | ~48µs        | ~56µs        | ~208µs      
Bun_regex_operations      | ~72µs        | ~64µs        | ~38µs        | ~55µs        | ~168µs      
GOJA_simple_arithmetic    | ~61µs        | ~56µs        | ~36µs        | ~47µs        | ~226µs      
Bun_simple_arithmetic     | ~165µs       | ~212µs       | ~55µs        | ~125µs       | ~347µs      
GOJA_spread_destructure   | ~74µs        | ~53µs        | ~50µs        | ~57µs        | ~242µs      
Bun_spread_destructure    | ~68µs        | ~23µs        | ~47µs        | ~60µs        | ~110µs      
GOJA_string_operations    | ~63µs        | ~53µs        | ~41µs        | ~50µs        | ~177µs      
Bun_string_operations     | ~115µs       | ~54µs        | ~47µs        | ~104µs       | ~204µs      
GOJA_type_guards          | ~71µs        | ~58µs        | ~47µs        | ~54µs        | ~240µs      
Bun_type_guards           | ~111µs       | ~76µs        | ~50µs        | ~94µs        | ~241µs      
GOJA_with_globals         | ~65µs        | ~61µs        | ~43µs        | ~49µs        | ~192µs      
Bun_with_globals          | ~100µs       | ~83µs        | ~52µs        | ~81µs        | ~149µs      
```

## Key Insights

- **GOJA wins** on simple/lightweight operations: arithmetic, strings, regex, object manipulation
- **Bun wins** on CPU-intensive work: recursive fibonacci (6.9x), nested loops (2.9x), JSON processing (2.5x)
- **Concurrency**: Bun scales better at high concurrency (32 workers: 895µs vs 1.45ms)
- **Ties**: regex_operations and spread_destructure show comparable performance
