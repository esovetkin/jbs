# jbs help do

The `do` block defines shell commands executed by a JUBE step.

In JUBE terms, JBS lowers `do` into JUBE [`step` sections](https://apps.fz-juelich.de/jsc/jube/docu/tutorial.html#step-dependencies). JBS imports the required data bindings for each step and reports collisions, missing imports, and unused imports.

## Syntax

```jbs
do <name>
        [after <step0>, <step1>, ...]
        [with <source>, <var> from <source2>, ...]
        [<key>=<int> ...]
{
        # shell commands
        # variables can be used as $<var> or ${<var>}
}
```

`[<key>=<int> ...]` configures JUBE step options. The currently allowed keys are:

- `max_async` must be an integer `>= 0`
- `procs` must be an integer `>= 0`
- `iterations` must be an integer `>= 1`

### `after`: step dependency declarations

`after` defines execution dependencies. A dependent step also inherits variables visible in predecessor steps. If the same visible name would come from different sources, JBS reports an error.

### `with`: import data bindings into the step

`with` imports table or scalar data bindings produced by top-level assignments or by imported modules.

Examples:

- `with cases`
- `with x from cases`
- `with (x, y) from cases`
- `with defaults.rows[x, y]`

Rules:

- variables are not visible unless imported through `with` or inherited through `after`
- importing a table source such as `with cases` exposes all of its columns
- importing multiple sources creates the expected JUBE product across those sources
- name collisions across imported or inherited variables are errors

### `${jube_...}`: useful JUBE variables inside `do`

Each workpackage executes the `do` body in its own work directory. Useful runtime variables include:

- `${jube_benchmark_name}`
- `${jube_benchmark_id}`
- `${jube_step_name}`
- `${jube_wp_id}`
- `${jube_wp_relpath}`
- `${jube_wp_abspath}`
- `${jube_benchmark_home}`

Use `$jube_benchmark_home` when you need files near the benchmark definition rather than files inside the workpackage directory.

## Example

```jbs
a = (1, 2)
b = ("a", "b")
base_cases = table(a = a, b = b)

d = ("x", "y")
extra_cases = table(d = d)

do step0
        with (a, b) from base_cases
{
        echo "${a} ${b}" > prepared.txt
}

do step1
        after step0
        with d from extra_cases
{
        test -f ../step0/work/prepared.txt
        echo "${a} ${b} ${d}"
}
```

In that example:

- `step1` waits for `step0`
- `a` and `b` come from inherited visibility through `after step0`
- `d` is added by the explicit `with d from extra_cases`
