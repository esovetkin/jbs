# jbs help globals

## `jbs_name`: output directory name

`jbs_name` names the benchmark directory. It defaults to `jbs_benchmark` and must evaluate to a string.

## `jbs_benchmarks`: split a script into multiple benchmarks

`jbs_benchmarks` optionally splits one JBS script into named benchmark components. It must be a dictionary and defaults to `{}`. When it is empty, JBS uses the single-directory layout and executes all `do` and `analyse` blocks.

When `jbs_benchmarks` is non-empty, it must be a dictionary whose keys are component names and whose values are benchmark targets. A target is the name of a `do` step or an `analyse` target:

```jbs
jbs_benchmarks = {
        "setup-only": "prepare",
        "small": ["prepare", "run_small", "summary"],
        "large": "run_large",
}
```

Calling `jbs run -b small` executes all dependency steps needed for the `prepare`, `run_small`, and `summary` targets and saves results in `<jbs_name>/<component>/` directories. A do-only target runs the selected step and its dependencies without generating analyse output for that target. A target with an `analyse` block runs the analysed step and its dependencies and writes that analyse output. Dependency analyse blocks are not selected implicitly; list a dependency step explicitly if its analyse output should be generated. You can also run `jbs continue -b small` for an individual benchmark. Without `--benchmark`, `jbs run` runs every configured component.

## `jbs_database`: write results to a SQLite database

`jbs_database` is the path to the analyse SQLite database. It defaults to `""`, which uses CSV mode and keeps per-step `analyse.csv` files. A non-empty relative path is resolved from the directory where `jbs run` is executed.

The database contains one table per analysed step and run. Single-benchmark table names use `<benchmark_name>_<run_id>_<step_name>`, for example `bench_000000_run`. Component table names use `<benchmark_name>_<component>_<run_id>_<step_name>`, for example `bench_small_000000_run`. Later runs create new tables in the same database instead of overwriting previous runs. `jbs continue` rewrites the table for the original run, and command output prints a path, row count, and column count summary for the current run's tables.

## `jbs_nproc`: set global concurrency limit

`jbs_nproc` is the global concurrency limit. It defaults to `0`, which means the number of available CPUs. Use the `nproc` clause in `do` blocks to control concurrency for individual execution steps (see [jbs help do](help_do.md)).
