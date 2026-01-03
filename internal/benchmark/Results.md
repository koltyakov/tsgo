### Performance Comparison

_Statistical results from 100 runs per test case (after warmup)._

```
Test Case                 | GOJA            | Bun             | Winner
--------------------------------------------------------------------------------
array_operations          | ~90µs           | ~103µs          | GOJA (1.1x)
class_instantiation       | ~196µs          | ~137µs          | Bun (1.4x)
closure_heavy             | ~183µs          | ~113µs          | Bun (1.6x)
generics                  | ~69µs           | ~80µs           | GOJA (1.2x)
iterative_fibonacci       | ~112µs          | ~88µs           | Bun (1.3x)
json_processing           | ~284µs          | ~119µs          | Bun (2.4x)
nested_loops              | ~786µs          | ~281µs          | Bun (2.8x)
object_manipulation       | ~77µs           | ~123µs          | GOJA (1.6x)
recursive_fibonacci       | 1.35ms          | ~227µs          | Bun (5.9x)
regex_operations          | ~68µs           | ~79µs           | GOJA (1.2x)
simple_arithmetic         | ~54µs           | ~124µs          | GOJA (2.3x)
spread_destructure        | ~75µs           | ~86µs           | GOJA (1.1x)
string_operations         | ~57µs           | ~120µs          | GOJA (2.1x)
type_guards               | ~68µs           | ~103µs          | GOJA (1.5x)
with_globals              | ~63µs           | ~95µs           | GOJA (1.5x)
```

### Concurrency Scaling

```
Concurrency  | GOJA            | Bun            
--------------------------------------------------
1            | ~124µs          | ~244µs         
4            | ~302µs          | ~228µs         
8            | ~608µs          | ~312µs         
16           | ~976µs          | ~334µs         
32           | 1.45ms          | ~652µs         
```

### Detailed Statistics

```
Test Case                 | Mean         | StdDev       | Min          | P50          | P95         
----------------------------------------------------------------------------------------------------
GOJA_array_operations     | ~90µs        | ~64µs        | ~65µs        | ~73µs        | ~342µs      
Bun_array_operations      | ~103µs       | ~45µs        | ~64µs        | ~91µs        | ~158µs      
GOJA_class_instantiation  | ~196µs       | ~86µs        | ~149µs       | ~162µs       | ~479µs      
Bun_class_instantiation   | ~137µs       | ~91µs        | ~81µs        | ~118µs       | ~199µs      
GOJA_closure_heavy        | ~183µs       | ~100µs       | ~132µs       | ~147µs       | ~455µs      
Bun_closure_heavy         | ~113µs       | ~52µs        | ~71µs        | ~101µs       | ~151µs      
GOJA_generics             | ~69µs        | ~45µs        | ~48µs        | ~53µs        | ~214µs      
Bun_generics              | ~80µs        | ~22µs        | ~57µs        | ~75µs        | ~118µs      
GOJA_iterative_fibonacci  | ~112µs       | ~71µs        | ~75µs        | ~91µs        | ~305µs      
Bun_iterative_fibonacci   | ~88µs        | ~28µs        | ~55µs        | ~79µs        | ~152µs      
GOJA_json_processing      | ~284µs       | ~108µs       | ~211µs       | ~232µs       | ~566µs      
Bun_json_processing       | ~119µs       | ~40µs        | ~80µs        | ~107µs       | ~197µs      
GOJA_nested_loops         | ~786µs       | ~94µs        | ~696µs       | ~758µs       | 1.04ms      
Bun_nested_loops          | ~281µs       | ~55µs        | ~217µs       | ~264µs       | ~364µs      
GOJA_object_manipulation  | ~77µs        | ~54µs        | ~53µs        | ~60µs        | ~214µs      
Bun_object_manipulation   | ~123µs       | ~46µs        | ~70µs        | ~113µs       | ~220µs      
GOJA_recursive_fibonacci  | 1.35ms       | ~48µs        | 1.25ms       | 1.35ms       | 1.43ms      
Bun_recursive_fibonacci   | ~227µs       | ~67µs        | ~175µs       | ~210µs       | ~315µs      
GOJA_regex_operations     | ~68µs        | ~50µs        | ~48µs        | ~54µs        | ~177µs      
Bun_regex_operations      | ~79µs        | ~52µs        | ~53µs        | ~66µs        | ~130µs      
GOJA_simple_arithmetic    | ~54µs        | ~50µs        | ~36µs        | ~41µs        | ~193µs      
Bun_simple_arithmetic     | ~124µs       | ~73µs        | ~52µs        | ~108µs       | ~237µs      
GOJA_spread_destructure   | ~75µs        | ~53µs        | ~50µs        | ~57µs        | ~212µs      
Bun_spread_destructure    | ~86µs        | ~85µs        | ~46µs        | ~72µs        | ~124µs      
GOJA_string_operations    | ~57µs        | ~41µs        | ~42µs        | ~47µs        | ~148µs      
Bun_string_operations     | ~120µs       | ~54µs        | ~60µs        | ~102µs       | ~247µs      
GOJA_type_guards          | ~68µs        | ~55µs        | ~48µs        | ~53µs        | ~196µs      
Bun_type_guards           | ~103µs       | ~97µs        | ~57µs        | ~81µs        | ~190µs      
GOJA_with_globals         | ~63µs        | ~48µs        | ~44µs        | ~50µs        | ~231µs      
Bun_with_globals          | ~95µs        | ~57µs        | ~50µs        | ~79µs        | ~177µs      
```
