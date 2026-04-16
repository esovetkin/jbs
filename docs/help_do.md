# jbs help do

The `do` block defines shell commands executed by a JUBE step.

In JUBE terms, JBS lowers `do` into JUBE [`step` sections](https://apps.fz-juelich.de/jsc/jube/docu/tutorial.html#step-dependencies). JBS imports the correct parameter-space slices required for each step and raises errors for name collisions, plus warnings for missing or unused imports.

## Syntax

```jbs
do <name>
        [after <step0>, <step1>, ...]
        [with <source>, <var> from <source2>, ...]
        [<key>=<int> ...]
{
        # shell commands
        # ...
        # variables can be used as $<var> or ${<var>} as usual
}
```

`[<key>=<int> ...]` lets you set JUBE options for the [step_tag](https://apps.fz-juelich.de/jsc/jube/docu/glossar.html#term-step_tag). The currently allowed keys are:
- `max_async` must be an integer `>= 0`
- `procs` must be an integer `>= 0`
- `iterations` must be an integer `>= 1`

### `after`: step dependency declarations

The `after` keyword defines [execution dependencies](https://apps.fz-juelich.de/jsc/jube/docu/tutorial.html#step-dependencies). A dependent step also inherits variables visible in predecessor steps. For example, in `do step1 after step0`, variables imported in `step0` are visible in `step1`. JBS imports only variables that are not already inherited.

If the same variable name would come from a different parameter set (either inherited or explicitly imported with `with`), JBS raises an error. Dependency cycles are not permitted.

### `with`: import parameters to be used in the step

The `with` keyword lets you import entire parameter sets, slices of parameter sets (defined by `param`), or individual variables from namespaces (defined by `let`). You can import variables from different parameter sets; in JUBE this resolves to a Cartesian product across the imported sets. If you need different behavior, define a new parameter set with the desired partial imports.

Any name collision results in an error. JBS raises warnings for forgotten imports (a variable is used in the shell body but its namespace was not imported in the step or inherited via dependencies) and for unused imports (a variable is imported but not used in the shell body).

- Variables are not visible unless imported through `with`.
- Importing a comb source (`with cases`) exposes all its columns.
- `with a, b` imports named global sources and combines them for the step.
- `after` keeps step dependency/inheritance behavior.

### `${jube_...}`: useful JUBE variables inside `do`

At runtime, each workpackage executes the `do` body in a work directory, whose path is available as `$jube_wp_abspath`. If you need to access files next to your `.jbs` file (or the generated JUBE file), use `$jube_benchmark_home` to navigate to the benchmark configuration location. Read more in the [JUBE variables glossary](https://apps.fz-juelich.de/jsc/jube/docu/glossar.html#term-jube_variables).

```jbs
do inspect
{
        echo "benchmark: ${jube_benchmark_name}"
        echo "benchmark_id: ${jube_benchmark_id}"
        echo "step: ${jube_step_name}"
        echo "wp_id: ${jube_wp_id}"
        echo "wp_relpath: ${jube_wp_relpath}"
        echo "wp_abspath: ${jube_wp_abspath}"
}
```

Each workpackage runs in its own directory (`$jube_wp_abspath`).
Use `$jube_benchmark_home` for files near your benchmark definition.

## Example

```jbs
a = (1, 2)
b = ("a", "b")
c = (true, false)
p0 = comb((a + b) * c)

d = ("x", "y")

do step0
        with (a, c) from p0
{
        echo "${a} ${c}" > prepared.txt
}

do step1
        after step0    # step1 starts only after step0
        with p0        # effectively, only b is imported, a, c inherited from step0
        with d from p1 # `with p0, p1` is also valid syntax
{
        # dependency links are provided by JUBE
        test -f ../step0/work/prepared.txt

        # a and c come from inheritance; b is added by explicit import
        echo "${a} ${b} ${c} ${d}"
}
```

Running JUBE on that example produces:
```bash
% jbs printparam example.jbs
XXX
% jbs example.jbs -o example.yaml
% jube-autorun example.yaml
...
XXX
  | stepname | all | open | wait | error | done |
  |----------|-----|------|------|-------|------|
  |    step0 |   4 |    0 |    0 |     0 |    4 |
  |    step1 |   8 |    0 |    0 |     0 |    8 |
...
```

### XXX `sbatch` jobs submission with `do`

The `-W` or `--wait` options make `sbatch` wait until the job terminates and return the submitted job's exit code. Therefore, you can submit jobs inside `do` like this:

```jbs
# XXX write a self-contained example
do step0 {
        ...
        procs = 4
        sbatch -W \
            --output=job.out --error=job.err \
            --account=... --nodes=1
            XXX
}
```


Keep in mind that JUBE `do` steps run [sequentially](https://apps.fz-juelich.de/jsc/jube/docu/advanced.html#parallel-workpackages), so you need to use `procs` to allow multiple jobs to be submitted.

XXX what happens when `sbatch -W` is killed. The job continues to run, but how should the benchmark be resumed? To avoid issues with unfinished steps caused by a killed process (or a lost SSH connection), you might consider submitting a job with `donefile` (next example) or using `submit`; see `jbs help submit` or [help_submit.md](help_submit.md).

### XXX `sbatch` jobs submission with `do` and `donefile`

XXX might require another keyword option for `do`
