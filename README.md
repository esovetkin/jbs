# JBS: A Language that Compiles to JUBE Configurations

Disclaimer: This language is still in early development, and there are likely bugs. If you find one or want additional features or syntax changes, please open an issue or PR. At the moment, only YAML output is supported. XML output may be added once JBS stabilizes.

## Motivation and Quick Start

[JUBE](https://apps.fz-juelich.de/jsc/jube/docu/) configuration files can be tedious to write. They contain repetitive syntax, their structure is often non-local (you need to jump across sections to match names), and small YAML indentation mistakes can break runs. The goal of JBS is to simplify this workflow and help users create JUBE configurations quickly.

Here is a small example. The following script runs `ex_step` six times (without Slurm job submission) and creates a result table from parsed output.

```bash
% cat > taster.jbs
param ex_parset {
        x = (1, 2)
        a = ("a", "b", "c")

        # `a + x` is like Python's zip
        # `a * x` is an outer product
        # `(a + b) * c` also works
        # the last expression "returns" the parameter set
        a * x
}

# the `do` section runs on a login node
# the `submit` section submits a Slurm job
do ex_step with ex_parset {
        echo "Number: ${x}"  > ex_ofile
        echo "Letter: ${a}" >> ex_ofile
}

analyse ex_step {
        # define which pattern is searched in which file
        # %d, %f, %w are shortcuts for JUBE pattern variables
        number = "Number: %d" in "ex_ofile"
        letter = "Letter: %w" in "ex_ofile"

        # the last expression defines result table columns
        (a as "name of a column", x, number, letter)
}
% jbs taster.jbs -o taster.yaml
% awk '!/^[[:space:]]*(#|$)/' taster.jbs | wc
     14      57     352
% awk '!/^[[:space:]]*(#|$)/' taster.yaml | wc
     59     133    1432
% jube-autorun taster.yaml
...
```

In addition to compiling JUBE configuration files, JBS also reports useful errors and warnings, such as unused variables, missing imports, variable name collisions, and circular dependencies.

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
  jbs help [globals|param|do|submit|let|analyse]

Inspect step parameter expansion:
  jbs printparam [-t pretty|csv] [-o <outputfile>] script.jbs

Format jbs in place:
  jbs fmt script.jbs
```

`printparam` defaults are `-t pretty` and `-o -` (stdout).

```bash
# pretty markdown table to stdout
jbs printparam script.jbs

# csv output to file
jbs printparam -t csv -o params.csv script.jbs
```

In `param`, `let`, `analyse`, `submit`, and top-level global assignments, statements can be separated by either newlines or `;`.

### `param <name> [with ...] { ... }`

Defines a parameter set by declaring variables and ending with a combination expression. `with` imports variables or full parameter sets from other parameter sets. The last expression defines how parameters are combined (see [Combination Algebra](docs/language.md#combination-algebra)). Variables used there can then be referenced in `do` and `submit` statements.

See `jbs help param` or [docs/help_param.md](docs/help_param.md).

### `do <name> [with ...] [after ...] { ... }`

`do` uses parameter sets provided via `with` (see [Import Semantics](docs/language.md#import-semantics-with)) and executes shell commands in the block. `after` defines [step dependencies](https://apps.fz-juelich.de/jsc/jube/docu/tutorial.html#step-dependencies). Circular dependencies are not allowed.

See `jbs help do` or [docs/help_do.md](docs/help_do.md).

### `submit <name> [with ...] [after ...] { key = value ... }`

The `submit` block configures the job-system settings, so it is less straightforward than `do`. JBS currently relies on [slurm/platform.xml](https://github.com/FZJ-JSC/JUBE/blob/master/platform/slurm/platform.xml) and [slurm/submit.job.in](https://github.com/FZJ-JSC/JUBE/blob/master/platform/slurm/submit.job.in).

See `jbs help submit` or [docs/help_submit.md](docs/help_submit.md).

### `let <namespace> { name = "regex-with-%d/%f/%w" ... }`

`let` defines namespaced variables that can be reused across the script. In `analyse`, pattern expressions can reference let variables (for example `p.number`) or inline strings. Placeholder shortcuts (`%d`, `%f`, `%w`) follow JUBE pattern conventions. See lowering details [here](docs/language.md#let--analyse-lowering).

See `jbs help let` or [docs/help_let.md](docs/help_let.md).

### `analyse <step_name> { ... }`

`analyse` defines JUBE `analyser` and `result` sections. You must target an existing `do` or `submit` step. `analyse` inherits variables visible in that step. Extraction assignments use either let references (`namespace.variable`) or inline string expressions before `in "file"`. The final tuple defines output columns.

```jbs
analyse <step_name> {
        value = expression in "file"
        ...

        (value [as "column_name"], ...)
}
```

See `jbs help analyse` or [docs/help_analyse.md](docs/help_analyse.md).

See [docs/language.md](docs/language.md) for grammar and semantics.

![](docs/logo.png "Go gopher kicking JUBE into exascale")
