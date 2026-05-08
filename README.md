![pipeline](https://gitlab.jsc.fz-juelich.de/sdlaml/jbs/badges/main/pipeline.svg) ![coverage](https://gitlab.jsc.fz-juelich.de/sdlaml/jbs/badges/main/coverage.svg)

# JBS

JBS is a small benchmark scripting tool inspired by [JUBE](https://apps.fz-juelich.de/jsc/jube/docu/index.html). JBS lets you define parameter sets, run Bash scripts for them, parse their output, and accumulate results in tables.

JBS defines a domain-specific language inspired by Python, R, and awk syntax.

## Quick Start

```bash
% cat taster.jbs
x = (1, 2)
a = ("a", "b", "c")

# The `do` sections define shell code to execute.
# The `$x` and `$a` variables receive values from the Cartesian product of tuples x and a.
do step with a, x {
        echo "Number: ${x}"  > ex_ofile
        echo "Letter: ${a}" >> ex_ofile
}

# The `analyse` sections define patterns to search for
# and how they should be presented in the result table.
analyse step {
        # Define which pattern is searched for in which file.
        # %d, %f, %w are shortcuts for common capture patterns
        number = "Number: %d" in "ex_ofile"
        letter = "Letter: %w" in "ex_ofile"

        # The final expression defines result table columns.
        (a as "name of a column", x, number, letter)
}
# Run the benchmark.
% jbs taster.jbs
 100% |████████████████████████████████| (6/6, 31 it/s) 0R|0E

step/analyse.csv
run_id,name of a column,x,number,letter
000000,a,1,1,a
000001,a,2,2,a
000002,b,1,1,b
000003,b,2,2,b
000004,c,1,1,c
000005,c,2,2,c
```

## Installation

`jbs` can be installed with `go install`:

```bash
# module load Go
go install gitlab.jsc.fz-juelich.de/sdlaml/jbs@latest
# Add "$(go env GOPATH)/bin" to PATH if needed.
```

Or `jbs` can be run directly with `go run`:

```bash
# module load Go
go run gitlab.jsc.fz-juelich.de/sdlaml/jbs@<tag> taster.jbs
```

## Usage

`jbs` is a single executable that checks and compiles a script, then runs or continues its execution steps. JBS also includes a REPL interpreter, which is invoked by calling `jbs` without arguments.

```bash
% jbs -h
Usage:

Run:
  jbs input.jbs [-n|--dry-run] [--no-strict] [-b|--benchmark <name>]
  jbs run input.jbs [-n|--dry-run] [--no-strict] [-b|--benchmark <name>]
  jbs continue input.jbs [-b|--benchmark <name>]

Archive:
  jbs archive input.jbs

Wait for files:
  jbs fwait [-e] <path> [path...]

Options:
  -n, --dry-run  Create the run directory without starting workpackages
  -b, --benchmark <name>
                 Run or continue one configured benchmark component
  --no-strict   Do not add set -euo pipefail to generated run.sh
  -c, --check   Parse+validate only

Read examples/help:
  jbs help [analyse|archive|continue|do|fwait|globals|repl|use]

Inspect step parameter expansion:
  jbs printparam [-t pretty|csv] [-o <outputfile>] script.jbs
  defaults: -t pretty, -o -

Format jbs in place:
  jbs fmt [-s|--strict] script.jbs

Interactive mode:
  jbs
  jbs repl
```

`jbs run` exits with code 0 when all jobs finish successfully and with code 1 if any workpackage fails. `jbs fwait` waits for files to appear or change via [fsnotify](https://docs.kernel.org/filesystems/inotify.html), which is useful for barrier jobs (see [docs/help_fwait.md](docs/help_fwait.md)). `jbs archive` can clean up generated workpackage directories (see [docs/help_archive.md](docs/help_archive.md)). `jbs printparam` lets you inspect steps and the parameter sets they use. `jbs fmt` applies canonical formatting to `jbs` files in place.

## Variable Types, Tables, and Parameter Spaces

The `jbs` language defines several data types and operations on those types.

[Scalar values](docs/language.md#scalars) (`string`, `int`, `float`, `bool`) are the only values that can be exported as variables in execution steps.

[Lists and tuples](docs/language.md#lists--tuples) combine scalar values and support several vector arithmetic operations.

Lists and tuples can be combined in [tables](docs/language.md#tables). Tables support slicing, which lets you take subsets of parameters. Tables can also be imported from CSV/TSV files (see [`?read_csv`](docs/function_help/read_csv.md) in the REPL). JBS also supports [dictionaries](docs/language.md#dictionaries), which can be converted to and from tables.

`jbs` supports [loops, conditional statements](docs/language.md#control-flow), and [functions](docs/language.md#functions).

Defined variables can be imported into `do` sections, and the corresponding scalar values are set as variables in each workpackage job.

## `do` blocks: workpackages

`do` blocks define execution steps.

```jbs
do <step_name>
        [after <dependency_step>]
        [with <table_or_value>[<column>, ...]]
        [nproc <max_parallel_workpackages>]
        [fsub "<template_file>" {
                "<regex>": <replacement_expr>,
        }]
{
        # shell code executed in each workpackage directory
        echo "${parameter}" > output.txt
}
```

`with` imports the variables used in the execution block.

The `after` keyword declares step dependencies. A dependent step can inherit visible variables from predecessor steps. The runner respects the dependency tree and concurrency limits.

`nproc` controls how many workpackages can run simultaneously for one execution step. `jbs_nproc` controls the total number of simultaneous workpackages across all execution steps.

`fsub` copies a template file into each workpackage directory and applies regular-expression substitutions using the step's visible variables before `run.sh` starts.

Executing the script produces the following directory structure:

```text
<jbs_name>/
  000000/
    manifest.json
    status
    <step>/
      analyse.csv              # only if the step has an analyse block and CSV mode is used
      000000/
        run.sh
        status
        stdout
        stderr
        exitcode               # after execution
        <dependency symlinks>
        <fsub output files>
```

Generated workpackage `run.sh` files use `set -euo pipefail` by default. Pass `--no-strict` to `jbs run` or the `jbs input.jbs` shorthand to omit it for newly created run directories.

See more in [docs/help_do.md](docs/help_do.md) or `jbs help do`.

## Analysis

`analyse` blocks define pattern matches in files generated across different workpackages. Analysis runs only after all scheduled workpackages for the component finish successfully. Each `analyse` block targets a specific `do` step and executes in the workpackage's directory.

```jbs
analyse <step_name>
        [with <scalar_value>, ...]
{
        # Match a pattern inside a workpackage output file.
        <pattern_name> = "<pattern>" in "<file>"

        # The final tuple defines result columns.
        (<variable_name> as "column 0", <pattern_name> as "column 1")
}
```

Each `analyse` block inherits variables from all dependent execution steps. An `analyse` block consists of pattern assignments and a final tuple that defines the resulting table structure.

Multiple matches produce multiple rows in the resulting table.

Patterns are regular expressions. JBS uses Go regular expressions based on RE2 syntax, not PCRE, so lookahead, lookbehind, and backreferences are not supported. For convenience, JBS includes `%d`, `%f`, and `%w` shortcuts for integer, float, and word captures (see [this example](docs/help_analyse.md#example)). Patterns support multiple capture groups, which produce multiple suffixed columns in the resulting table.

In CSV mode, JBS rewrites the step's `analyse.csv` in the corresponding directory. JBS also supports SQLite mode, which writes all analysis tables to a single file (use [`jbs_database`](docs/help_globals.md#jbs_database-write-results-to-a-sqlite-database)).

See more in [docs/help_analyse.md](docs/help_analyse.md) or `jbs help analyse`.

For the full JBS language reference, see [docs/language.md](docs/language.md).

## Comparison to JUBE

JBS was inspired by JUBE. For example, it produces a similar directory structure. However, it differs in several ways:

- JBS is a language, not a configuration file. This makes the syntax more compact and enables more flexible compile-time and runtime error analysis. JBS also aims to simplify sophisticated parameter-space definitions and their use across dependent execution steps.

- JBS currently has no tag support, because parameter spaces are already programmatically configurable.

- Unlike JUBE, JBS analysis captures all groups and all matches in a file and represents them in a table. There is no built-in support for [summary statistics](https://apps.fz-juelich.de/jsc/jube/docu/advanced.html#statistic-pattern-values) like in JUBE; users can process analysis tables further as needed.

- JBS does not include submit templates. Users define their own submission scripts, while JBS provides several ways to wait for submitted jobs (XXXlink to examplesXXX).

- JBS greedily executes workpackages across the DAG while respecting global and local concurrency limits and job dependencies. JUBE executes jobs step by step. By default, JBS uses as many CPUs as are available.
