# jbs help run

`jbs run <file.jbs>` evaluates a JBS file, creates the next run directory, prepares workpackages for selected `do` steps, executes them locally, and writes status and analyse outputs. `jbs <file.jbs>` is equivalent to `jbs run <file.jbs>`.

## Example

```jbs
jbs_name = "bench"

cases = table(x = [1, 2])

do run with cases {
        echo "value=$x" > result.txt
}

analyse run {
        ("value=%d" in "result.txt" as "value")
}
```

```bash
% jbs run bench.jbs
 100% |████████████████████████████████| (2/2, 26 it/s) 0R|0E

| step    | FINISHED | ERROR | BLOCKED | NOTSTARTED | RUNNING | INTERRUPTED | duration_s |
|---------|----------|-------|---------|------------|---------|-------------|------------|
| └── run |        2 |     0 |       0 |          0 |       0 |           0 |       0.09 |
|---------|----------|-------|---------|------------|---------|-------------|------------|
| total:  |        2 |     0 |       0 |          0 |       0 |           0 |       0.09 |

| analysis                     | nrows | ncols |
|------------------------------|-------|-------|
| bench/000000/run/analyse.csv |     2 |     2 |
```

This creates `bench/000000/`, workpackage directories below `bench/000000/run/`, and `bench/000000/run/analyse.csv`.

## Output Layout

`jbs_name` controls the benchmark output directory and defaults to `"jbs_benchmark"`.

Without [`jbs_benchmarks`](help_globals.md#jbs_benchmarks-split-a-script-into-multiple-benchmarks) variable configured, runs use the following directory structure.

```text
<jbs_name>/<run_id>/
```

With configured `jbs_benchmarks` dictionary, component runs use:

```text
<jbs_name>/<component>/<run_id>/
```

Run IDs are zero-padded numeric directories such as `000000`, `000001`, and `000002`.

Each run contains:

- `manifest.json`: persisted run metadata, source/template identity, selected workpackages, step metadata, analyse targets, template hashes, and work limit
- `status`: root benchmark status
- `<step>/<row>/run.sh`: generated shell script for one workpackage
- `<step>/<row>/status`: workpackage status, including duration after completion
- `<step>/<row>/stdout` and `<step>/<row>/stderr`: captured process streams
- `<step>/<row>/exitcode`: written after a process exits
- `<step>/analyse.csv`: CSV analyse output when the step has an analyse block and CSV mode is used

## Generated Shell Environment

Generated `run.sh` scripts export JBS metadata variables and every imported workpackage value:

- `JBS_RUN_DIR`: absolute path to the final run directory
- `JBS_SRC_DIR`: absolute path to the directory containing the entry `.jbs` file
- `JBS_STEP`: current `do` step name
- `JBS_ROW`: zero-padded workpackage row ID
- `JBS_WORK_DIR`: absolute path to the current workpackage directory
- variables imported through `with` or inherited through `after`

By default, generated scripts start with `set -euo pipefail`. Workpackages inherit environment, where `jbs` was started.

## Options

```bash
jbs run <file.jbs> [-n|--dry-run] [-w|--weak] [-l|--limit <n>] [--no-strict] [-b|--benchmark <name>]
jbs <file.jbs>     [-n|--dry-run] [-w|--weak] [-l|--limit <n>] [--no-strict] [-b|--benchmark <name>]
```

### `-n`, `--dry-run`

`--dry-run` creates the next run directory and prepares all selected workpackages, but it does not start shell commands and does not run analyses.  Workpackages have `NOTSTARTED` status.

```bash
jbs run --dry-run input.jbs
jbs run -n input.jbs
jbs -n input.jbs
```

Use `jbs continue <file.jbs>` to start or resume the latest prepared run.

### `-w`, `--weak`

`--weak` allows analyse outputs to be generated when some selected workpackages fail. The benchmark still finishes with status `ERROR`, and the command still exits non-zero.

In weak mode, analyse tables include the `"jbs_status"` column. Finished workpackages are analysed normally. Failed or blocked workpackages selected for analysis produce rows with missing analyse values and their status, such as `ERROR` or `BLOCKED`.  Weak mode does not run analyses after interrupted runs.

```bash
jbs run --weak input.jbs
jbs run -w input.jbs
```

Weak mode is also available for continuation with `jbs continue -w`.

### `-l`, `--limit <n>`

`--limit <n>` creates and runs only the first `n` workpackages DAG branches. A branch is a target workpackage plus all dependency workpackages required by that target.

If analyses are selected, analysed steps are the branch targets. Otherwise JBS uses terminal steps in the selected benchmark component. Configured benchmark components are limited independently.

`--limit` reduces created workpackage directories and scheduler work. Workpackage row IDs are preserved, so limited runs may contain gaps in complex DAGs.

### `-b`, `--benchmark <name>`

`--benchmark <name>` selects one component from `jbs_benchmarks`. The option requires a non-empty `jbs_benchmarks` dictionary, and the selected name must exist in that dictionary.

```jbs
jbs_benchmarks = {
        "small": "run_small",
        "large": "run_large",
        "all": "*",
}
```

```bash
jbs run -b small input.jbs
jbs run --benchmark all input.jbs
```

When `jbs_benchmarks` is configured and `--benchmark` is omitted, `jbs run` runs every configured component.

A component target in `jbs_benchmarks` dictionary can be a `do` step, an analyse target, a list of targets, or `"*"` for the full workplan. A do-only target runs the target step and its dependencies without selecting analyse output for that target. An analyse target runs the analysed step and writes that analyse output. Dependency analyse blocks are not selected implicitly.

See [`jbs help globals`](help_globals.md) for the full `jbs_benchmarks` reference.

### `--no-strict`

`--no-strict` omits `set -euo pipefail` from newly generated `run.sh` files. It applies only while creating a new run directory. It does not rewrite scripts inside an existing run continued later.

```bash
jbs run --no-strict input.jbs
jbs --no-strict input.jbs
```

## Concurrency

`jbs_nproc` sets the global concurrency limit. It defaults to `0`, meaning the number of CPUs available to the `jbs` process. Each `do` block can also use `nproc <int>` to limit concurrency for that step.  `nproc 0` uses the same detected CPU count.

See [`jbs help do`](help_do.md) and [`jbs help globals`](help_globals.md) for the detailed syntax.

## Analyse Output

In CSV mode, each analysed step writes `<step>/analyse.csv` inside the run directory. CSV mode is the default.

When `jbs_database` is non-empty, analyse outputs are written to SQLite instead of per-step CSV files. The command output prints the table names and row/column counts for the current run.

After execution, `jbs run` prints the status table and, when analysis ran, a summary of generated analyse outputs.

## Status And Failures

Status values are `NOTSTARTED`, `RUNNING`, `FINISHED`, `ERROR`, `BLOCKED`, and `INTERRUPTED`. `BLOCKED` means a workpackage did not run because a dependency failed. If workpackages fail, the post-run summary prints absolute paths to the failed workpackage directories.

The `jbs` returns non-zero exit code when planning fails, a workpackage fails, analysis fails, or the run is interrupted.
