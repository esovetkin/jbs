# jbs help do

The `do` block defines shell commands executed by a JUBE step.

In JUBE terms, JBS lowers `do` into JUBE [`step` sections](https://apps.fz-juelich.de/jsc/jube/docu/tutorial.html#step-dependencies). JBS imports the required data bindings for each step and reports collisions, missing imports, and unused imports.

## Syntax

```jbs
do <name>
        [after <step0>, <step1>, ...]
        [with <source>, <source2>[<col0>, <col1>, ...], ...]
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
% jbs do.jbs -o do.yaml
% jube-autorun do.yaml
...
  | stepname | all | open | wait | error | done |
  |----------|-----|------|------|-------|------|
  |    step0 |   2 |    0 |    0 |     0 |    2 |
  |    step1 |   4 |    0 |    0 |     0 |    4 |
...
```

### Submitting `sbatch` jobs with `do`

[This example](../examples/do_sbatch.jbs) creates sbatch scripts in the `write_sbatch` step. Then `runwait_sbatch` submits a job with `sbatch -W`, which waits for job completion.

The steps in JUBE are executed [serially](https://apps.fz-juelich.de/jsc/jube/docu/advanced.html#parallel-workpackages). The `procs` argument can be used to increase the number of jobs running in parallel (`procs=0` doesn't work as expected in JUBE).

Another way is to use the `done` file. However, there seems to be no support for this in JUBE YAML, only XML.
