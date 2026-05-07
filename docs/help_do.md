# jbs help do

The `do` block defines shell commands executed by direct `jbs run` workpackages. JBS imports the required data bindings for each step and reports collisions, missing imports, and unused imports.

## Syntax

```jbs
do <name>
        [after <step0>, <step1>, ...]
        [with <source>, <source2>[<col0>, <col1>, ...], ...]
        [nproc <int>]
{
        # shell commands
        # variables can be used as $<var> or ${<var>}
}
```

`nproc` configures per-step direct-run concurrency:

- `nproc 0` uses the number of CPUs available to the `jbs` process for this step
- `nproc 4` allows at most four workpackages from this step to run at once
- missing `nproc` is equivalent to `nproc 0`

The global `jbs_nproc` assignment limits total running workpackages across the benchmark:

```jbs
jbs_nproc = 16
```

`jbs_nproc = 0` uses the number of CPUs available to the `jbs` process as the global limit.

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

### `JBS_...`: useful direct-run variables inside `do`

Each workpackage executes the `do` body in its own work directory. Useful runtime variables include:

- `$JBS_RUN_DIR`
- `$JBS_STEP`
- `$JBS_ROW`
- `$JBS_WORK_DIR`

Each workpackage directory also contains `run.sh`, `status`, `stdout`, `stderr`, and, after completion, `exitcode`.

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

On terminals, direct run renders completed workpackages over total workpackages. The `nR|nE` suffix reports currently running and errored jobs.

Direct `do` blocks can still call scheduler tools such as `sbatch` from inside `run.sh`, but direct `jbs run` only supervises the local shell process it starts.
