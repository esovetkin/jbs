![pipeline](https://gitlab.jsc.fz-juelich.de/sdlaml/jbs/badges/main/pipeline.svg) ![coverage](https://gitlab.jsc.fz-juelich.de/sdlaml/jbs/badges/main/coverage.svg)

# JBS: A Language that Compiles to JUBE Configurations

Disclaimer: JBS is still in early development and may contain bugs. If you find one or want additional features or syntax changes, please open an issue or PR. At the moment, only YAML output is supported. XML output may be added once JBS stabilizes.

## Motivation and Quick Start

[JUBE](https://apps.fz-juelich.de/jsc/jube/docu/) configuration files can be tedious to write. They contain repetitive syntax, their structure is often non-local (you need to jump across sections to match names), and small YAML indentation mistakes can break runs. The goal of JBS is to simplify this workflow and help users create JUBE configurations faster and more safely. See [docs/motivation.md](docs/motivation.md) for more details.

Here is a small example. The following script runs `ex_step` six times (without Slurm job submission) and creates a result table from parsed output.

```bash
% cat > taster.jbs
x = (1, 2)
a = ("a", "b", "c")

# Build one explicit table and then take the Cartesian product.
# Variables ${a} and ${x} become visible whenever ex_parset is imported via `with`.
ex_parset = product(table(a = a), table(x = x))

# the `do` sections define the shell code to run
# the `submit` sections define a sbatch script to submit
do ex_step with ex_parset {
        echo "Number: ${x}"  > ex_ofile
        echo "Letter: ${a}" >> ex_ofile
}

# the `analyse` sections define patterns to be searched
# and how they should be presented in the result table
analyse ex_step {
        # define which pattern is searched in which file
        # %d, %f, %w are shortcuts for JUBE pattern variables
        number = "Number: %d" in "ex_ofile"
        letter = "Letter: %w" in "ex_ofile"

        # the last expression defines result table columns
        (a as "name of a column", x, number, letter)
}
% jbs taster.jbs -o taster.yaml
% jube-autorun taster.yaml
...
```

In addition to compiling JUBE configuration files, JBS reports useful errors and warnings, such as unused variables, missing imports, variable-name collisions, and circular dependencies (see more in [docs/diagnostics.md](docs/diagnostics.md)).

## Build and Test

```bash
# module load Go

# go test ./...
make test

# compiles into a single executable `jbs`
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
  jbs help [globals|functions|do|submit|analyse|repl|use]

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

A JBS program uses six canonical top-level statement forms:

- `use`
- top-level assignment
- top-level expression statement
- `do`
- `submit`
- `analyse`

Top-level assignments define reusable globals, including explicit table values and function values. They are single-assignment bindings: top-level code uses plain `=`, defines each name once, and introduces a new name instead of rebinding with `+=` or a later `=`. `do` and `submit` import visible data explicitly through `with`. `analyse` builds result tables from parsed files. Legacy top-level `let` and `param` blocks are no longer part of the language.

Top-level assignments define global variables. Use `table(...)`, `zip(...)`, `product(...)`, and `select(...)` to build parameter-space objects and import them in steps.

```jbs
x = (1, 2)
model = ("a", "b", "c")
cases = product(table(model = model), table(x = x))
seed0 = 1
seed1 = seed0 + 1
seed2 = seed1 + 1
```

Use `table(...)` for one table, `zip(...)` for row-wise merge, `product(...)` for Cartesian product, and `select(...)` for projection. Legacy `comb(...)` still works during migration, but it emits a deprecation warning.

Read more in `jbs help globals` or [docs/help_globals.md](docs/help_globals.md).

Functions are also ordinary top-level expression values:

```jbs
make_adder = function(delta) {
        function(x) {
                x + delta
        }
}

add2 = make_adder(2)
```

They can be passed around in expressions and imported from modules, but they are not valid `with` / `submit use` / `analyse with` data sources.

### `do <name> [with ...] [after ...] [<key>=<int> ...] { ... }`

`do` defines the step computation via a shell script with the variables from parameter sets provided via `with` (see [Import Semantics](docs/language.md#import-semantics-with)). [Step dependencies](https://apps.fz-juelich.de/jsc/jube/docu/tutorial.html#step-dependencies) are defined by `after`. Circular dependencies are not allowed.

Read more in `jbs help do` or [docs/help_do.md](docs/help_do.md).

### `submit <name> [with ...] [after ...] [use ...] [<key>=<int> ...] { key = value ... }`

The `submit` block configures job-system settings, so it is less straightforward than `do`. `with` imports row-varying data. `submit ... use ...` imports scalar defaults from globals or module namespaces. JBS currently supports only Slurm job templates (see [slurm/platform.xml](https://github.com/FZJ-JSC/JUBE/blob/master/platform/slurm/platform.xml) and [slurm/submit.job.in](https://github.com/FZJ-JSC/JUBE/blob/master/platform/slurm/submit.job.in)).

Read more in `jbs help submit` or [docs/help_submit.md](docs/help_submit.md).

### `analyse <step_name> [with ...] { ... }`

`analyse` defines JUBE `analyser` and `result` sections. You must target an existing `do` or `submit` step. `analyse` inherits variables visible in that step. Pattern variables are defined in extraction expressions or imported via `with` (scalar string bindings only).

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

`use` imports reusable definitions from embedded or local `.jbs` scripts.

```jbs
use jsc
use "./defaults.jbs" as local_defaults
use submit_defaults from jsc
use add from "./lib/math.jbs"
use "./lib/math.jbs" as math
```

Namespace imports expose function-valued globals as `math.add(...)`; selective imports project them into local expression scope as ordinary globals.

Read more in `jbs help use` or [docs/help_use.md](docs/help_use.md).

### REPL

`jbs` with no arguments starts the REPL. Multiline functions and closures work the same way they do in files:

```jbs
jbs> add = function(a, b = 1) {
...>   a + b
...> }
jbs> add(41)
42
```

Top-level expression statements are legal in files and in REPL. File-mode compilation ignores their display output, so they are mainly useful for REPL work and quick local checks.

See [docs/language.md](docs/language.md) for the JBS grammar and semantics.

## Known Limitations

- The main idea behind JBS is to design a compact language while maintaining JUBE functionality. Hence, JBS is designed as a compiler of JBS into JUBE YAML.

  If JBS becomes useful, the next step would be the implementation of the `jbs run` that replaces runs the benchmark with the same functionality as JUBE.

- No XML generation.

  I want JBS syntax to stabilize first. I chose YAML because I started writing JUBE in YAML first.

- Results are saved only as CSV/TSV.

  There is also [the database option](https://apps.fz-juelich.de/jsc/jube/docu/advanced.html#result-database), which should, in principle, be just an extra argument in `analyse`.

- Tags affect parameter sets.

  I still need to design a clean syntax for this in JBS.

- [Multiple benchmarks](https://apps.fz-juelich.de/jsc/jube/docu/advanced.html#multiple-benchmarks).

- Single-benchmark features: `substitutionset` and `fileset`.

  I have never used these, so I need examples to understand the functionality and decide the best way to include them in JBS.

- Non-Slurm `submit` support could be implemented as an additional argument.

Useful link: [general JUBE structure](https://apps.fz-juelich.de/jsc/jube/docu/glossar.html#term-general_structure_yaml).

- [statistic pattern values](https://apps.fz-juelich.de/jsc/jube/docu/advanced.html#statistic-pattern-values)

```jbs
analyse <stepname>
{
    p = ...

    (..., max(p))
}
```

in general detecting multiple patterns is useful
