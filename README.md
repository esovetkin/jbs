![pipeline](https://gitlab.jsc.fz-juelich.de/sdlaml/jbs/badges/main/pipeline.svg) ![coverage](https://gitlab.jsc.fz-juelich.de/sdlaml/jbs/badges/main/coverage.svg)

# JBS: A Language that Compiles to JUBE Configurations

Disclaimer: JBS is still in early development and may contain bugs. If you find one or want additional features or syntax changes, please open an issue or PR. At the moment, only YAML output is supported. XML output may be added once JBS stabilizes.

## Motivation and Quick Start

[JUBE](https://apps.fz-juelich.de/jsc/jube/docu/) configuration files can be tedious to write. They contain repetitive syntax, their structure is often non-local (you need to jump across sections to match names), and small YAML indentation mistakes can break runs. The goal of JBS is to simplify this workflow and help users create JUBE configurations faster and more safely. See [docs/motivation.md](docs/motivation.md) for more details.

Here is a small example. The following script runs `step` six times (without Slurm job submission) and creates a result table from parsed output.

```bash
% cat > taster.jbs

# Build a table of variable combinations (`*` is a Cartesian product, `+` is a direct sum).
# Variables ${a} and ${x} become visible whenever `cases` is imported via `with`.
cases = t(a = ("a", "b", "c")) * t(x = (1,2))

# The `do` sections define the shell code to run.
# The `submit` sections define an sbatch script to submit.
do step with cases {
        echo "Number: ${x}"  > ex_ofile
        echo "Letter: ${a}" >> ex_ofile
}

# The `analyse` sections define patterns to be searched
# and how they should be presented in the result table.
analyse step {
        # define which pattern is searched in which file
        # %d, %f, %w are shortcuts for JUBE pattern variables
        number = "Number: %d" in "ex_ofile"
        letter = "Letter: %w" in "ex_ofile"

        # the last expression defines result table columns
        (a as "name of a column", x, number, letter)
}
% jbs taster.jbs -o taster.yaml
% awk '!/^[[:space:]]*(#|$)/' taster.jbs
     10      52     321
% awk '!/^[[:space:]]*(#|$)/' taster.yaml
     59     133    1472
% jube-autorun taster.yaml
...
  | stepname | all | open | wait | error | done |
  |----------|-----|------|------|-------|------|
  |  ex_step |   6 |    0 |    0 |     0 |    6 |
...
name of a column,x,number,letter
a,1,1,a
a,2,2,a
b,1,1,b
b,2,2,b
c,1,1,c
c,2,2,c
```

In addition to compiling JUBE configuration files, JBS reports useful errors and warnings, such as unused variables, missing imports, variable-name collisions, and circular dependencies.

## Build and Test

```bash
# module load Go

# go test ./...
make test

# Compile into a single executable, `jbs`.
# go build -o jbs ./cmd/jbs
make
```

## Help and Syntax Overview

```bash
% jbs -h
Usage:

Compile with:
  jbs input.jbs -o output.yaml

Options:
  -o, --output   Output path (default: - for stdout)
  -c, --check    Parse+validate only

Read examples/help:
  jbs help [analyse|do|functions|globals|repl|submit|use]

Inspect embedded shared scripts:
  jbs embed [filename]

Inspect step parameter expansion:
  jbs printparam [-t pretty|csv] [-o <outputfile>] script.jbs
  defaults: -t pretty, -o -

Format jbs in place:
  jbs fmt [-s|--strict] script.jbs

Interactive mode:
  jbs
  jbs repl
```

A JBS program uses the following statement forms:
- `use`: import another JBS file
- top-level assignments via expressions and function calls
- `if`: select assignment and expression statements with a boolean condition
- `do`, `submit`: blocks that define execution steps
- `analyse`: blocks that define result analysis

Top-level assignments define reusable globals, consisting of scalars, lists/tuples, tables, and functions. `do` and `submit` execution blocks import visible variables explicitly through `with` and based on the point where the block appears. The `analyse` blocks inherit variables from the execution blocks.

Use `if condition { ... } else { ... }` to choose values before normal top-level declarations. `if` bodies may contain assignments, expression statements, and nested `if`; declarations and imports (`do`, `submit`, `analyse`, `use`) stay at module top level.

### `do <name> [with ...] [after ...] [<key>=<int> ...] { ... }`

`do` defines the step computation via a shell script using variables from parameter sets provided via `with` (see [Import Semantics](docs/language.md#import-semantics-with)). [Step dependencies](https://apps.fz-juelich.de/jsc/jube/docu/tutorial.html#step-dependencies) are defined by `after`, and dependent steps still inherit predecessor-visible variables through those dependencies. Circular dependencies are not allowed.

Read more in `jbs help do` or [docs/help_do.md](docs/help_do.md).

### `submit <name> [with ...] [after ...] [use ...] [<key>=<int> ...] { key = value ... }`

The `submit` block configures job-system settings, so it is less straightforward than `do`. `with` imports row-varying data. `submit ... use ...` imports scalar defaults from globals or module namespaces. JBS currently supports only Slurm job templates (see [slurm/platform.xml](https://github.com/FZJ-JSC/JUBE/blob/master/platform/slurm/platform.xml) and [slurm/submit.job.in](https://github.com/FZJ-JSC/JUBE/blob/master/platform/slurm/submit.job.in)).

Read more in `jbs help submit` or [docs/help_submit.md](docs/help_submit.md).

### `analyse <step_name> [with ...] { ... }`

`analyse` defines JUBE `analyser` and `result` sections. You must target an existing `do` or `submit` step. `analyse` inherits variables visible in that step and all predecessor steps.

```jbs
analyse <step_name>
        [with <scalar0>, <scalar1>, ...]
{
        value0 = expression in "file"
        value1 = "<pattern>" in "file"
        ...

        (value0 [as "column_name"], value1, ...)
}
```

Read more in `jbs help analyse` or [docs/help_analyse.md](docs/help_analyse.md).

### `use ...`

`use` imports reusable definitions from embedded modules and quoted local `.jbs` scripts.

```jbs
use jsc
use submit_defaults from jsc
use "./defaults.jbs" as local_defaults
use add from "./lib/math.jbs"
use "./lib/math.jbs" as math
```

Bare import names are for embedded modules only; local files must be quoted paths resolved from the importing file.

Read more in `jbs help use` or [docs/help_use.md](docs/help_use.md).

See [docs/language.md](docs/language.md) for the JBS grammar and semantics.

## REPL

`jbs` with no arguments starts the REPL.

```bash
% jbs
Type :help for commands, Ctrl+D to exit
jbs> add = function(a, b = 1) {
...>   a + b
...> }
jbs> add(41)
42
```

Use `:help <function_name>` or `?<function_name>` to view documentation for a specific function.

## Known Limitations

- The main idea behind JBS is to design a compact language while maintaining JUBE functionality. Hence, JBS is designed to compile JBS programs into JUBE YAML.

  If JBS becomes useful, the next step would be to implement `jbs run`, which would run benchmarks with the same functionality as JUBE.

- No XML generation.

  I want the JBS syntax to stabilize first. I chose YAML because I started writing JUBE configurations in YAML.

- Results are saved only as CSV/TSV.

  There is also a [database option](https://apps.fz-juelich.de/jsc/jube/docu/advanced.html#result-database), which should, in principle, require only an extra argument in `analyse`.

- Tags affect parameter sets.

  I still need to design a clean syntax for this in JBS.

- [Multiple benchmarks](https://apps.fz-juelich.de/jsc/jube/docu/advanced.html#multiple-benchmarks).

- Single-benchmark features: `substitutionset` and `fileset`.

  I have never used these, so I need examples to understand the functionality and decide the best way to include them in JBS.

- Non-Slurm `submit` support could be implemented as an additional argument.

Useful reference: [general JUBE structure](https://apps.fz-juelich.de/jsc/jube/docu/glossar.html#term-general_structure_yaml).

- [Statistic pattern values](https://apps.fz-juelich.de/jsc/jube/docu/advanced.html#statistic-pattern-values)

```jbs
analyse <stepname>
{
    p = ...

    (..., max(p))
}
```

In general, detecting multiple patterns is useful.
