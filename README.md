# JBS

JBS is a small benchmark scripting tool inspired by [JUBE](https://apps.fz-juelich.de/jsc/jube/docu/index.html). A `.jbs` file defines parameter data, executable `do` steps and `analyse` pattern extraction blocks.

The direct runner creates a benchmark directory, expands workpackages from `with` tables, runs step workpackages with dependency ordering, and writes per-workpackage stdout, stderr, status, and exit code files.

## Quick Start

```jbs
x = (1, 2)
a = ("a", "b", "c")

# The `do` sections define shell code to run.
# The `$x` and `$a` variables receive values from the Cartesian product of the tuples x and a.
do step with a, x {
        echo "Number: ${x}"  > ex_ofile
        echo "Letter: ${a}" >> ex_ofile
}

# The `analyse` sections define patterns to search for
# and how they should be presented in the result table.
analyse step {
        # define which pattern is searched for in which file
        # %d, %f, %w are shortcuts for common capture patterns
        number = "Number: %d" in "ex_ofile"
        letter = "Letter: %w" in "ex_ofile"

        # the final expression defines result table columns
        (a as "name of a column", x, number, letter)
}
```

Run it:

```bash
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

Run from source:

```bash
ml Go
go run gitlab.jsc.fz-juelich.de/sdlaml/jbs@<tag> taster.jbs
```

Resume an interrupted benchmark:

```bash
jbs continue taster.jbs
```

Inspect parameter expansion without running jobs:

```bash
jbs printparam taster.jbs
jbs printparam -t csv -o params.csv taster.jbs
```

Validate or format a script:

```bash
jbs --check taster.jbs
jbs fmt taster.jbs
```

## Commands

```text
jbs <file.jbs> [--no-strict]      run a benchmark
jbs run <file.jbs> [--no-strict]  run a benchmark
jbs continue <file.jbs>           resume an interrupted benchmark
jbs --check <file.jbs>            parse and validate only
jbs printparam [opts] <file>      print expanded step parameters
jbs fmt [-s|--strict] <file>      format a script in place
jbs help [topic]                  show built-in help
jbs repl                          start the REPL
```

Help topics:

```bash
jbs help analyse
jbs help do
jbs help functions
jbs help globals
jbs help repl
jbs help use
```

## Language

Top-level assignments define scalar values, lists, tuples, tables, and functions. `do` blocks execute shell code once per workpackage. `analyse` blocks extract values from files created by a step and write `analyse.csv` for that step, or SQLite tables when `jbs_database` is set. `use` imports values or step declarations from another `.jbs` file.

`print(...)` writes explicit JBS output to command stdout. In `jbs run`, those lines appear before benchmark work starts; shell output from `run.sh` stays in each workpackage `stdout` file.

`with` imports row-varying data into a step. Multiple sources are combined using the table algebra provided by functions such as `table`, `product`, `select`, and `zip`.

`after` declares step dependencies. A dependent step can inherit visible variables from predecessor steps. The runner respects the dependency tree and concurrency limits.

`nproc` limits concurrent workpackages:

```jbs
jbs_nproc = 8
jbs_database = "results.sqlite"

do compile nproc 4 {
        make
}
```

`jbs_nproc = 0` and `do ... nproc 0` both mean "use the number of available CPUs".

`jbs_database = ""` keeps the default per-step `analyse.csv` files. A non-empty value writes all analyse outputs into one SQLite database, with one table per analysed step and run. Table names use `<benchmark_name>_<run_id>_<step_name>`, for example `bench_000000_run`, so later runs accumulate new tables instead of overwriting old ones. Relative database paths are resolved from the directory where `jbs run` is executed; absolute paths are accepted.

See [docs/language.md](docs/language.md) for details.

## Comparison to JUBE

- no submit templates. it's up to a user to define sbatch (or any other queue system) scripts

- jbs, however, provides

- it's a language, not a configuration file

  benchmark parameter space can be done programmatically
