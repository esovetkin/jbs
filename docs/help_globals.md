# jbs help globals

Built-in globals:

```jbs
# Benchmark directory name.
jbs_name = "jbs_benchmark"

# Global concurrency limit. 0 uses available CPU count.
jbs_nproc = 0

# Analyse database path. Empty string keeps per-step analyse.csv files.
# Non-empty values write tables named <benchmark_name>_<run_id>_<step_name>.
# Relative paths are resolved from the directory where `jbs run` is executed.
jbs_database = ""
```

User globals can hold scalars, lists, tuples, tables, and functions. `do` and `analyse` blocks see the globals visible at the point where the block is declared.
