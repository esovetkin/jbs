# jbs help do

The `do` block defines shell commands that are executed by a JUBE step.

At runtime, each workpackage executes the `do` body in its own JUBE work directory. In practice, this is the directory pointed to by `$jube_wp_abspath` (see the JUBE variables glossary: https://apps.fz-juelich.de/jsc/jube/docu/glossar.html#term-jube_variables).

If you need to access files next to your `.jbs` (or generated JUBE) file, use `$jube_benchmark_home` to jump back to the benchmark definition location instead of working only inside the workpackage directory.

Related JUBE docs:
- JUBE variables glossary: https://apps.fz-juelich.de/jsc/jube/docu/glossar.html#term-jube_variables
- JUBE directory structure: https://apps.fz-juelich.de/jsc/jube/docu/glossar.html#term-directory_structure

## Syntax

```jbs
do <name>
        [after <step0>, <step1>, ...]
        [with <paramset>, <var> from <paramset2>, ...]
{
        # shell commands
}
```

## Variable inheritance with `after`

`after` is not only an execution dependency. The dependent step also inherits variables that were visible in the predecessor step.

- If `step1` has `after step0`, variables imported in `step0` are visible in `step1`.
- If `step1` also uses `with ... from <same_paramset>`, jbs imports only variables that are not already inherited.
- If the same variable name would come from different parameter sets (inherited vs explicit `with`), jbs raises an error.
- jbs also preserves source-row constraints from inherited imports:
  - direct-sum pairing (`+`) is not broken in downstream steps
  - outer-product expansions (`*`) remain valid where expected
- row constraints propagate transitively across chains such as `step0 -> step1 -> step2`.

Example:

```jbs
param pm0
{
        a = (1, 2)
        b = ("x", "y")
        c = (true, false)
        a * b * c
}

do step0
        with (a, b) from pm0
{
        echo "${a} ${b}"
}

do step1
        after step0
        with (b, c) from pm0
{
        # a and b come from inheritance; c is added from explicit import
        echo "${a} ${b} ${c}"
}
```

## Example 1: Basic `do` with one parameter set

```jbs
param cases
{
        case = ("ddp", "fsdp")
        case
}

do run_case
        with cases
{
        echo "running ${case}"
        hostname
}
```

What happens:
- JUBE creates one workpackage per row in `cases`.
- The `do` script runs once per workpackage.
- `${case}` is substituted from the imported parameter set.

## Example 2: `after` dependency

```jbs
do prepare
{
        echo "prepare inputs" > prepared.txt
}

do train
        after prepare
{
        test -f ../prepare/work/prepared.txt
        echo "train"
}
```

What happens:
- `train` starts only after `prepare` workpackages are complete.
- Dependency links are provided by JUBE according to its workpackage directory structure.

## Example 3: Use both workpackage-local and benchmark-home paths

```jbs
param p
{
        name = ("a", "b")
        name
}

do build
        with p
{
        echo "WP dir: ${jube_wp_abspath}"
        echo "Benchmark home: ${jube_benchmark_home}"

        # Read input from benchmark location
        cp "${jube_benchmark_home}/configs/${name}.cfg" ./input.cfg

        # Write output into this workpackage
        echo "done ${name}" > result.txt
}
```

Why this is useful:
- Keep immutable/shared input in `$jube_benchmark_home`.
- Keep per-run artifacts in the workpackage (`$jube_wp_abspath`).
- See other [JUBE variables](https://apps.fz-juelich.de/jsc/jube/docu/glossar.html#term-jube_variables)

## Example 4: Import selected variables with `with ... from ...`

```jbs
param matrix
{
        case = ("ddp", "fsdp")
        nnodes = (1, 2)
        case * nnodes
}

do smoke
        with (case, nnodes) from matrix
{
        echo "case=${case} nodes=${nnodes}"
}
```

This imports only the listed variables from `matrix` into the step.

## Example 5: Useful JUBE variables inside `do`

All of these are documented in the JUBE glossary linked above.

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

## Practical guidelines

- Use `do` for commands that should run directly in a JUBE workpackage.
- Use `after` to enforce ordering between steps.
- Use `with` to import exactly the variables needed by the step.
- Use `$jube_benchmark_home` when you need stable paths to source/config files.
- Use `$jube_wp_abspath` for workpackage-local outputs and debugging.
