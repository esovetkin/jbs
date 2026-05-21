# jbs help do

A `do` block defines shell commands and workpackages that are executed by `jbs run`. JBS imports the required data bindings for each step and reports collisions, missing imports, and unused imports.

## Syntax

```jbs
do <name>
        [after <step0>, <step1>, ...]
        [with <source>, <source2>["<col0>", "<col1>", ...], ...]
        [fsub "<template>" { "<regex>": <expr>, ... }]
        [nproc <int>]
{
        # shell commands
        # variables can be used as $<var> or ${<var>}
}
```

### `after`: step dependency declarations

`after` defines execution dependencies. A dependent step also inherits every variable visible in predecessor steps, including names inherited transitively. Any name collisions result in an error.

### `with`: import data bindings into the step

`with` imports scalar, list, tuple, table, or dictionary data bindings produced by top-level assignments or by imported modules.

Examples:

- `with cases`
- `with cases["x"]`
- `with cases["x", "y"]`
- `with defaults.rows["x", "y"]`
- `with case_id, cases["x"]`

Rules:

- variables are not visible unless imported through `with` or inherited through `after`
- imported values are emitted into generated scripts as exported environment variables
- importing a scalar creates one workpackage and exposes that scalar under its source name
- importing a list or tuple creates one workpackage per element; non-scalar elements are exported as environment variables using `str(value)`, and JBS emits a warning
- importing a table source such as `with cases` exposes all of its columns
- importing a dictionary acts like `with table(dict_value)` and exposes dictionary keys as table columns
- importing selected columns such as `with cases["x", "y"]` exposes only those names
- importing multiple sources such as `with cases["x"], env["host"]` creates a Cartesian product across those sources
- name collisions across different imported or inherited sources are errors
- overlapping imports from the same source are tolerated; a dependent full-source import keeps only columns not already inherited

### `fsub`: copy and substitute a template file

`fsub "path" { ... }` copies a template into every workpackage directory for the step, preserves the template's regular permission bits, and applies ordered regular-expression substitutions before `run.sh` starts. Relative template paths are resolved relative to the `.jbs` file that defines the step. The copied filename is the template basename.

```jbs
cases = table(x = [1, 2.5], label = ["a", "b"], i = [7])

# > cat input.template
# ###X###
# ###LABEL###
# tuple: (one, two)
# id=0
# x=0.0 label=old
# literal=%

do run
        with cases
        fsub "input.template" {
                "###X###": x,
                "###LABEL###": label,
                # capture groups are replaced from tuple values
                "tuple: \((\S+), (\S+)\)": (x, label),
                # %d, %f, %w shortcuts are allowed. % is escaped with %%
                "id=%d": i,
                "x=%f label=%w": (x, label),
                "literal=%%": "literal=%"
        }
{
        cat input.template
}
```

Rules:

- rule keys are Go regular expressions with analyse-style placeholders: `%d` matches an integer, `%f` matches a floating-point value, `%w` matches a word, and `%%` matches a literal percent
- rules run in declaration order
- a rule must match at least once in every workpackage template
- multiple matches are all replaced, and JBS reports a warning
- without capture groups, the whole match is replaced by one scalar value
- with capture groups, including `%d`, `%f`, and `%w` placeholders, provide a tuple or list with one scalar value per group
- replacement expressions can use variables visible in the step through `with` or `after`
- using `fsub "<filepath>" {}` simply copies the file without any replacements

A dry run creates substituted files without executing work. `jbs continue` resumes the already prepared files and rejects the run if configured template hashes no longer match.

### `nproc`: control concurrency levels

`nproc` configures per-step direct-run concurrency:

- `nproc 0` uses the number of CPUs available to the `jbs` process for this step
- `nproc 4` allows at most four workpackages from this step to run at once
- missing `nproc` is equivalent to `nproc 0`

The global `jbs_nproc` assignment limits total running workpackages across the benchmark:

```jbs
jbs_nproc = 16
```

`jbs_nproc = 0` uses the number of CPUs available to the `jbs` process as the global limit.

### `JBS_...`: useful direct-run variables inside `do`

Each workpackage executes the `do` body in its own work directory. Useful runtime variables include:

- `$JBS_RUN_DIR`: absolute path to the final run directory
- `$JBS_STEP`: current `do` step name
- `$JBS_ROW`: zero-padded row/workpackage ID
- `$JBS_WORK_DIR`: absolute path to the current workpackage directory
- `$JBS_SRC_DIR`: absolute path to the directory containing the entry `.jbs` file

Each workpackage directory also contains `run.sh`, `status`, `stdout`, `stderr`, and, after completion, `exitcode`. Workpackage status is one of `NOTSTARTED`, `RUNNING`, `FINISHED`, `ERROR`, `BLOCKED`, or `INTERRUPTED`. `BLOCKED` means the workpackage did not run because a dependency failed. Generated `run.sh` files use `set -euo pipefail` by default; pass `--no-strict` to `jbs run` or the `jbs file.jbs` shorthand to omit it for newly created run directories.

Pass `--weak` or `-w` to `jbs run` or `jbs continue` to generate analyse outputs even when some workpackages fail. The benchmark still ends with status `ERROR`, and the command still exits non-zero, but analyse tables are written for selected analyse steps. Weak mode does not apply to interrupted runs.

Pass `--limit <n>` or `-l <n>` to run only the first `n` selected DAG branches. A branch is a target workpackage plus all dependency workpackages needed by that target. If analyses are selected, the analysed steps are the branch targets; otherwise JBS uses terminal steps in the selected benchmark component. Configured benchmark components are limited independently.

`--limit` reduces created workpackage directories and scheduler work. It does not guarantee exactly `n` analyse rows when one workpackage emits multiple pattern matches or no matches. Workpackage row IDs are preserved, so limited runs may contain gaps in complex DAGs.

Create the directory structure without starting work:

```bash
jbs run --dry-run do.jbs
jbs run -n do.jbs
jbs run --weak do.jbs
jbs run -w do.jbs
jbs continue -w do.jbs
jbs run --limit 1 do.jbs
jbs run -l 1 do.jbs
jbs run -n -l 1 do.jbs
jbs -n do.jbs
jbs -l 1 do.jbs
jbs do.jbs -n
jbs continue do.jbs
```

A dry run creates the next numeric run directory with all workpackages marked `NOTSTARTED`. It does not execute workpackages or analyses.

## Example

```jbs
# do.jbs
a = (1, 2)
b = ("a", "b")
base_cases = table(a = a, b = b)

d = ("x", "y")
extra_cases = table(d = d)

do step0
        with base_cases["a", "b"]
{
        echo "${a} ${b}" > prepared.txt
}

do step1
        after step0
        with extra_cases["d"]
{
        test -f step0/prepared.txt
        echo "${a} ${b} ${d}"
}
```

In that example:

- `step1` waits for `step0`
- `a` and `b` come from inherited visibility through `after step0`
- `d` is added by the explicit `with extra_cases["d"]`

```bash
% jbs do.jbs
 100% |████████████████████████████████| (6/6, 34 it/s) 0R|0E

| step          | FINISHED | ERROR | BLOCKED | NOTSTARTED | RUNNING | INTERRUPTED |
|---------------|----------|-------|---------|------------|---------|-------------|
| └── step0     |        2 |     0 |       0 |          0 |       0 |           0 |
|     └── step1 |        4 |     0 |       0 |          0 |       0 |           0 |
|---------------|----------|-------|---------|------------|---------|-------------|
| total:        |        6 |     0 |       0 |          0 |       0 |           0 |
```

### `sbatch` example

Direct `do` blocks can still call scheduler tools such as `sbatch` from inside `run.sh`, but direct `jbs run` only supervises the local shell process it starts. See [examples/do_sbatch.jbs](../examples/do_sbatch.jbs) for several ways to wait for submitted jobs.
