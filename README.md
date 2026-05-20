![pipeline](https://gitlab.jsc.fz-juelich.de/sdlaml/jbs/badges/main/pipeline.svg) ![coverage](https://gitlab.jsc.fz-juelich.de/sdlaml/jbs/badges/main/coverage.svg)

# JBS

JBS is a small benchmark scripting tool inspired by [JUBE](https://apps.fz-juelich.de/jsc/jube/docu/index.html). JBS lets you define parameter sets, run Bash scripts for them, parse their output, and accumulate results in tables.

JBS defines a domain-specific language with syntax inspired by Python, R, and awk.

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
        # Define which pattern to search for and which file to read.
        # %d, %f, and %w are shortcuts for common capture patterns.
        number = "Number: %d" in "ex_ofile"
        letter = "Letter: %w" in "ex_ofile"

        # The final expression defines result table columns.
        (a as "name of a column", x, number, letter)
}
# Run the benchmark.
% jbs taster.jbs
 100% |████████████████████████████████| (6/6, 30 it/s) 0R|0E

| step     | FINISHED | ERROR | BLOCKED | NOTSTARTED | RUNNING | INTERRUPTED |
|----------|----------|-------|---------|------------|---------|-------------|
| └── step |        6 |     0 |       0 |          0 |       0 |           0 |
|----------|----------|-------|---------|------------|---------|-------------|
| total:   |        6 |     0 |       0 |          0 |       0 |           0 |

| analysis                              | nrows | ncols |
|---------------------------------------|-------|-------|
| jbs_benchmark/000000/step/analyse.csv |     6 |     5 |
% cat jbs_benchmark/000000/step/analyse.csv
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
go run gitlab.jsc.fz-juelich.de/sdlaml/jbs@latest taster.jbs
```

Or you can grab the [compiled binary](https://gitlab.jsc.fz-juelich.de/sdlaml/jbs/-/jobs/artifacts/main/raw/jbs?job=release).

## Usage

`jbs` is a single executable that checks and compiles a script, then runs or continues its execution steps. JBS also includes a REPL interpreter, invoked by calling `jbs` without arguments (see [docs/help_repl.md](docs/help_repl.md)).

```bash
% jbs -h
Usage:

Read examples/help:
  jbs help [analyse|archive|continue|do|fwait|globals|ls-analyse|param|repl|status|tree|use]

Run:
  jbs <file.jbs> [-n|--dry-run] [-w|--weak] [-l|--limit <n>] [--no-strict] [-b|--benchmark <name>]
  jbs run <file.jbs> [-n|--dry-run] [-w|--weak] [-l|--limit <n>] [--no-strict] [-b|--benchmark <name>]
  jbs continue <file.jbs> [-b|--benchmark <name>]

Check syntax:
  jbs -c|--check <file.jbs>

Print status of the latest run:
  jbs status <file.jbs|benchmark-dir> [-b|--benchmark <name>]

List generated analyse tables:
  jbs ls-analyse <file.jbs|benchmark-dir> [-b|--benchmark <name>]

Options:
  -n, --dry-run  Create the run directory without starting workpackages
  -w, --weak     Generate analyse outputs even when some workpackages fail
  -l, --limit <n>
                 Create and run only the first n selected DAG branches
  -b, --benchmark <name>
                 Run, continue, or inspect one configured benchmark component
  --no-strict   Do not add set -euo pipefail to generated run.sh
  -c, --check   Parse syntax only; do not evaluate expressions or imports

Profiling:
  --cpuprof[=<file>]
                 Write a CPU pprof profile; default: cpu.pprof
  --memprof[=<file>]
                 Write a heap pprof profile at command exit; default: mem.pprof

Archive benchmark directory:
  jbs archive <file.jbs|benchmark-dir>

Wait for files:
  jbs fwait [-e] <path> [path...]

Inspect job dependencies:
  jbs tree <file.jbs> [-b|--benchmark <name>]

Inspect step parameter expansion:
  jbs param [-t pretty|csv] [-o <outputfile>] <file.jbs>
  defaults: -t pretty, -o - (stdout)

Interactive mode:
  jbs
  jbs repl
```

`jbs run` exits with code 0 when all jobs finish successfully and with code 1 if any workpackage fails. `jbs run` workpackages inherit environment, where `jbs` was started. Pass `--limit <n>` or `-l <n>` to create and run only the first `n` selected DAG branches; a branch is a target workpackage plus the dependency workpackages it needs. Configured benchmark components are limited independently. `jbs status` prints the latest run status (see [docs/help_status.md](docs/help_status.md)), `jbs tree` prints the planned job tree (see [docs/help_tree.md](docs/help_tree.md)), and `jbs ls-analyse` lists generated analyse outputs (see [docs/help_ls_analyse.md](docs/help_ls_analyse.md)). `jbs fwait` waits for files to appear or change (see [docs/help_fwait.md](docs/help_fwait.md)). `jbs archive` can clean up generated workpackage directories (see [docs/help_archive.md](docs/help_archive.md)). `jbs param` lets you inspect steps and the parameter sets they use (see [docs/help_param.md](docs/help_param.md)).

`--cpuprof` and `--memprof` profile the JBS process. They are useful for debugging parsing, semantic analysis, planning, scheduling, and CLI overhead. They do not profile shell commands launched by `do` blocks. Inspect profiles with `go tool pprof`:

```sh
go tool pprof -http=:8080 ./jbs cpu.pprof
go tool pprof -http=:8080 ./jbs mem.pprof
```

## Variable Types, Tables, and Parameter Spaces

The `jbs` language defines several data types and operations on them.

[Scalar values](docs/language.md#scalars) (`string`, `int`, `float`, `bool`) are the only values that can be exported as variables in execution steps.

[Lists and tuples](docs/language.md#lists--tuples) combine scalar values and support several vector arithmetic operations.

Lists and tuples can be combined in [tables](docs/language.md#tables). Tables support slicing, which lets you take subsets of parameters. Tables can also be imported from CSV/TSV files (see [`?read_csv`](docs/function_help/read_csv.md) in the REPL). JBS also supports [dictionaries](docs/language.md#dictionaries).

`jbs` supports [loops](docs/language.md#for-while-loops), [conditional statements](docs/language.md#ifelse), and [functions](docs/language.md#functions).

Defined variables can be imported into `do` sections. Their corresponding scalar values are then set as variables in each workpackage job.

## `do` blocks: workpackages

`do` blocks define execution steps.

```jbs
do <step_name>
        [with <scalar/list/table/dict>, ... ]
        [after <dependency_step>]
        [nproc <max_parallel_workpackages>]
        [fsub "<template_file>" {
                "<regex>": <replacement_expr>,
        }]
{
        # shell code executed in each workpackage directory
        echo "${parameter}" > output.txt
}
```

`with` imports values used in the execution block. Scalars run once, lists and tuples run once per element, tables and dictionaries expose columns, and table projections use quoted column names, such as `with cases["x"]`.

The `after` keyword declares step dependencies. A dependent step can inherit visible variables from predecessor steps. The runner respects the dependency tree and concurrency limits.

`nproc` controls how many workpackages can run simultaneously for one execution step. `jbs_nproc` controls the total number of simultaneous workpackages across all execution steps.

`fsub` copies a template file into each workpackage directory, preserves its regular permission bits, and applies regular-expression substitutions with the step's visible variables before `run.sh` starts. Pattern keys support the same `%d`, `%f`, `%w`, and `%%` shortcuts used by `analyse` patterns.

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

`analyse` blocks define pattern matches in files generated across workpackages. Each `analyse` block targets a specific `do` step and executes in the workpackage's directory. By default, analysis runs only after all scheduled workpackages for the component finish successfully. Pass `--weak` or `-w` to generate analyse outputs even when some workpackages fail.

```jbs
analyse <step_name>
        [with <scalar_value>, ...]
	{
	        # Match a pattern inside a workpackage output file.
	        <pattern_name> = "<pattern>" in "<file>"
	        <pattern_name> = "<pattern>" in re"<file-regex>"

	        # The final tuple defines result columns.
	        (<variable_name> as "column 0", <pattern_name> as "column 1", "<pattern>" in re"<file-regex>" as "column 2")
	}
	```

Each `analyse` block inherits variables from all dependent execution steps. An `analyse` block consists of optional pattern assignments and a final tuple that defines the resulting table structure. The final tuple may include step-visible variables, extraction aliases, or direct pattern expressions written as `<pattern_expr> in "<file>"` or `<pattern_expr> in re"<file-regex>"`. If a direct pattern omits `as "<column name>"`, the evaluated pattern string is used as the column name. Pattern expressions are regular expressions. File targets are exact relative paths when written as strings; use `re"..."` to match all regular files whose workpackage-relative path matches a Go regular expression. Regex file targets add a `<column>.file` result column containing the matching relative filename. JBS uses Go regular expressions based on RE2 syntax, not PCRE, so lookahead, lookbehind, and backreferences are not supported. For convenience, JBS includes `%d`, `%f`, and `%w` shortcuts for integer, float, and word captures (see [this example](docs/help_analyse.md#example)).

Multiple matches produce multiple rows in the resulting table. Patterns support multiple capture groups, which produce multiple suffixed columns in the resulting table.

In CSV mode, JBS writes the step's `analyse.csv` in the corresponding directory. JBS also supports SQLite mode, which writes all analysis tables to a single file (use [`jbs_database`](docs/help_globals.md#jbs_database-write-results-to-a-sqlite-database)). SQLite output preserves `%d` captures as `INTEGER` columns and `%f` captures as `REAL` columns.

See more in [docs/help_analyse.md](docs/help_analyse.md) or `jbs help analyse`.

For the full JBS language reference, see [docs/language.md](docs/language.md).

## Comparison to JUBE

JBS was inspired by JUBE. For example, it produces a similar directory structure. However, it differs in several ways:

- JBS is a language, not a configuration file. This makes the syntax more compact and enables more flexible compile-time and runtime error analysis. JBS also aims to simplify sophisticated parameter-space definitions and their use across dependent execution steps.

- JBS currently has no tag support, because parameter spaces are already programmatically configurable.

- Unlike JUBE, JBS analysis captures all groups and all matches in a file and represents them in a table. There is no built-in support for [summary statistics](https://apps.fz-juelich.de/jsc/jube/docu/advanced.html#statistic-pattern-values) as JUBE provides; users can process analysis tables further as needed.

- JBS does not include submit templates. Users define their own submission scripts, while JBS provides several ways to wait for submitted jobs (see [examples/do_sbatch.jbs](examples/do_sbatch.jbs)).

- JBS greedily executes workpackages across the DAG while respecting global and local concurrency limits and job dependencies. JUBE executes jobs step by step. By default, JBS uses as many CPUs as are available.

- In `--weak` mode, JBS can generate analysis tables even when some jobs fail. JUBE requires all steps to complete successfully.
