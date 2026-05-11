# jbs help do

The `do` block defines shell commands and workpackages executed by `jbs run`. JBS imports the required data bindings for each step and reports collisions, missing imports, and unused imports.

## Syntax

```jbs
do <name>
        [after <step0>, <step1>, ...]
        [with <source>, <source2>[<col0>, <col1>, ...], ...]
        [fsub "<template>" { "<regex>": <expr>, ... }]
        [nproc <int>]
{
        # shell commands
        # variables can be used as $<var> or ${<var>}
}
```

### `after`: step dependency declarations

`after` defines execution dependencies. A dependent step also inherits every variable visible in predecessor steps, including names those predecessors inherited transitively. Any name collisions result in an error.

### `with`: import data bindings into the step

`with` imports table or scalar data bindings produced by top-level assignments or by imported modules.

Examples:

- `with cases`
- `with cases[x]`
- `with cases[x, y]`
- `with defaults.rows[x, y]`

Rules:

- variables are not visible unless imported through `with` or inherited through `after`
- importing a table source such as `with cases` exposes all of its columns
- importing selected columns such as `with cases[x, y]` exposes only those names
- importing multiple sources such as `with cases[x], env[host]` creates a Cartesian product across those sources
- name collisions across imported or inherited variables are errors

### `fsub`: copy and substitute a template file

`fsub "path" { ... }` copies a template into every workpackage directory for the step and applies ordered regular-expression substitutions before `run.sh` starts. Relative template paths are resolved relative to the `.jbs` file that defines the step. The copied filename is the template basename.

```jbs
cases = table(x = [1, 2], label = ["a", "b"])

do run
        with cases
        fsub "input.template" {
                "###X###": x,
                "###LABEL###": label,
        }
{
        ./solver input.template
}
```

Rules:

- rule keys are Go regular expressions
- rules run in declaration order
- a rule must match at least once in every workpackage template
- multiple matches are all replaced and reported as warnings
- without capture groups, the whole match is replaced by one scalar value
- with capture groups, provide a tuple/list with one scalar value per group
- replacement expressions can use variables visible in the step through `with` or `after`

Dry-run creates substituted files without executing work. `jbs continue` resumes the already prepared files and rejects the run if configured template hashes no longer match.

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
- `$JBS_ROW`: zero-padded row/workpackage id
- `$JBS_WORK_DIR`: absolute path to the current workpackage directory
- `$JBS_SRC_DIR`: absolute path to the directory containing the entry `.jbs` file

Each workpackage directory also contains `run.sh`, `status`, `stdout`, `stderr`, and, after completion, `exitcode`. Workpackage status is one of `NOTSTARTED`, `RUNNING`, `FINISHED`, `ERROR`, `BLOCKED`, or `INTERRUPTED`. `BLOCKED` means the workpackage did not run because a dependency failed. Generated `run.sh` files use `set -euo pipefail` by default; pass `--no-strict` to `jbs run` or the `jbs file.jbs` shorthand to omit it for newly created run directories.

Create the directory structure without starting work:

```bash
jbs run --dry-run do.jbs
jbs run -n do.jbs
jbs -n do.jbs
jbs do.jbs -n
jbs continue do.jbs
```

Dry-run creates the next numeric run directory with all workpackages marked `NOTSTARTED`. It does not execute workpackages or analyses.

## Example

```jbs
# do.jbs
a = (1, 2)
b = ("a", "b")
base_cases = table(a = a, b = b)

d = ("x", "y")
extra_cases = table(d = d)

do step0
        with base_cases[a, b]
{
        echo "${a} ${b}" > prepared.txt
}

do step1
        after step0
        with extra_cases[d]
{
        test -f ../step0/work/prepared.txt
        echo "${a} ${b} ${d}"
}
```

In that example:

- `step1` waits for `step0`
- `a` and `b` come from inherited visibility through `after step0`
- `d` is added by the explicit `with extra_cases[d]`

```bash
% jbs run do.jbs
42% |████████████████                | (42/100, 18 it/s) 3R|1E
```

### `sbatch` example

Direct `do` blocks can still call scheduler tools such as `sbatch` from inside `run.sh`, but direct `jbs run` only supervises the local shell process it starts.
