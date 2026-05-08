# jbs help globals

Built-in globals:

```jbs
# Benchmark directory name.
jbs_name = "jbs_benchmark"

# Optional benchmark components. Empty means one benchmark named by jbs_name.
# Non-empty values map component names to analyse block names.
jbs_benchmarks = {}

# Global concurrency limit. 0 uses available CPU count.
jbs_nproc = 0

# Analyse database path. Empty string keeps per-step analyse.csv files.
# Non-empty values write tables named <benchmark_name>_<run_id>_<step_name>.
# Component benchmarks use <benchmark_name>_<component>_<run_id>_<step_name>.
# Relative paths are resolved from the directory where `jbs run` is executed.
jbs_database = ""
```

`jbs_benchmarks` must be a dictionary whose keys are component names and whose values are analyse block names:

```jbs
jbs_benchmarks = {
        "small": ["run_small", "summary"],
        "large": "run_large",
}
```

When it is non-empty, each component is written below `<jbs_name>/<component>/` and runs only the steps needed by the requested analyse blocks. Use `jbs run -b <component> input.jbs` or `jbs continue -b <component> input.jbs` for one component.

User globals can hold scalars, lists, tuples, dictionaries, tables, and functions. `do` and `analyse` blocks see the globals visible at the point where the block is declared.
