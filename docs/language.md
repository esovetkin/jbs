# JBS Language

JBS files are evaluated top to bottom. They contain global assignments, imports, `do` blocks, `analyse` blocks, and top-level expression statements.

## Program Shape

```text
program        := statement*
statement      := assignment | use_stmt | do_block | analyse_block
                | if_stmt | for_stmt | while_stmt | break_stmt | continue_stmt
                | expr_stmt
assignment     := IDENT assign_op expr
assign_op      := "=" | "+=" | "-=" | "*=" | "/=" | "%="
use_stmt       := "use" import_items "from" STRING
if_stmt        := "if" expr block elif_branch* else_branch?
elif_branch    := "elif" expr block
else_branch    := "else" block
for_stmt       := "for" IDENT "in" expr block
while_stmt     := "while" expr block
block          := "{" statement* "}"
do_block       := "do" IDENT header_item* "{" raw_body "}"
header_item    := after_clause | with_clause | nproc_clause | fsub_clause
fsub_clause    := "fsub" STRING "{" (STRING ":" expr ("," STRING ":" expr)* ","?)? "}"
analyse_block  := "analyse" IDENT analyse_header_item* "{" analyse_body "}"
expr           := literal | name | call | index | member | unary | binary
                | conditional | function | list | tuple | dict
dict           := "{" (expr ":" expr ("," expr ":" expr)* ","?)? "}"
```

Top-level expression statements are evaluated. In files they are mainly useful for validation and quick inspection; in the REPL their values are printed.

`print(...)` writes explicit output using the same value rendering as the REPL. During `jbs run file.jbs`, print output is written to command stdout before benchmark work starts; shell stdout from `run.sh` is still captured in workpackage `stdout` files.

## Compile-Time And Runtime

Global assignments, imports, top-level expressions, functions called from those expressions, `read_csv(...)`, `print(...)`, `shell(...)`, and `env(...)` are evaluated before benchmark work starts. `shell(...)` runs in the source file's directory, captures stdout as a string, and can export currently assigned scalar JBS variables referenced as shell variables. `env(...)` reads the environment of the running `jbs` process.

`do` block bodies run later as generated `run.sh` scripts inside workpackage directories. Their stdout and stderr are stored in each workpackage's output files.

## Built-In Globals

`jbs_name` names the benchmark directory. It defaults to `jbs_benchmark` and must evaluate to a string.

`jbs_benchmarks` optionally splits one script into named benchmark components. It defaults to `{}`. When non-empty, it must be a dictionary from component names to analyse block names.

`jbs_nproc` is the global concurrency limit. It defaults to `0`. A value of `0` means the number of available CPUs.

`jbs_database` is the analyse SQLite database path. It defaults to `""`, which keeps per-step `analyse.csv` files. A non-empty relative path is resolved from the directory where `jbs run` is executed; absolute paths are accepted. SQLite analyse table names use `<benchmark_name>_<run_id>_<step_name>` for single benchmarks and `<benchmark_name>_<component>_<run_id>_<step_name>` for benchmark components.

```jbs
jbs_name = "sweep"
jbs_benchmarks = {}
jbs_nproc = 8
jbs_database = "results.sqlite"
```

## Values

JBS supports:

- `int`, `float`, `str`, `bool`, and `null`
- lists: `[1, 2, 3]`
- tuples: `(1, 2, 3)`
- dictionaries: `{"name": "case-a", 1: "one"}` or `dict(name = "case-a")`
- tables, created with `table(...)` or `t(...)`
- functions: `function(x) { x + 1 }`

Tuple syntax requires a comma for one item: `(1,)`.

## Dictionaries

Dictionaries map string, int, or bool keys to arbitrary JBS values.

```jbs
d = dict(name = "case-a", threads = 8)
same = {"name": "case-a", 1: [1, 2, 3], true: "enabled"}
```

Literal keys are expressions, so a bare identifier is looked up first:

```jbs
key = "name"
{"name": 1} == {key: 1}
```

Use index syntax for required keys and `get(...)` for optional keys:

```jbs
d["name"]
get(d, "missing", "fallback")
```

`update(d, key = value, ...)` and `left + right` return new dictionaries. Right-hand values replace existing keys and duplicate literal keys keep their first insertion position.

```jbs
base = dict(a = 1, b = 2)
next = base + dict(b = 3, c = 4)
next == dict(a = 1, b = 3, c = 4)
```

`for key in d { ... }` iterates keys only, in insertion order.

## Tables

Tables are named columns with equal row counts.

```jbs
cases = table(x = (1, 2), label = ("a", "b"))
```

Useful table operations:

- `product(a, b)` or `a * b` computes a Cartesian product.
- `zip(a, b)` or `a + b` combines rows by position.
- `select(table, col1, col2)` projects columns.
- `names(value)` lists visible names or table columns.

## Control Flow

`if`, `for`, and `while` can compute globals before declarations.

```jbs
mode = "small"
if mode == "small" {
        cases = table(x = range(2))
} elif mode == "large" {
        cases = table(x = range(10))
} else {
        cases = table(x = range(1))
}

values = ()
for x in range(3) {
        values += (x,)
}
```

`do`, `analyse`, and `use` declarations are top-level only. They are not allowed inside control-flow bodies.

## Imports

Imports load another `.jbs` file and merge selected declarations into the current program.

```jbs
use cases from "./params.jbs"
use "./steps.jbs" as steps
```

Namespaced imports are referenced with dot syntax:

```jbs
do run with steps.cases {
        echo "${x}"
}
```

Importing a `do` step also imports the dependencies required by its `after` chain.

## `do`

`do` blocks define shell code to execute. Each block runs once for every row visible through its `with` data.

```jbs
do run
        with cases
        nproc 4
{
        echo "x=${x}" > out.txt
}
```

Header clauses:

- `with source` imports all columns from a data source.
- `with source[a,b]` imports selected columns.
- `after step` waits for another step and inherits that step's visible variables.
- `fsub "path" { "regex": expr }` copies a template into each workpackage directory and applies substitutions before `run.sh` starts.
- `nproc N` limits concurrent workpackages for this step.

`nproc 0` means the number of available CPUs. The effective step concurrency is limited by both `jbs_nproc` and the step's own `nproc`.

`fsub` template paths are resolved relative to the `.jbs` file that defines the `do` block. The destination filename is the template basename. Substitution rules are regular expressions applied in declaration order. A rule with no capture groups replaces the full match with a scalar replacement value. A rule with capture groups replaces each captured group and requires a tuple/list with the same number of scalar values. Rules must match at least once for every workpackage; multiple matches are all replaced and reported as warnings. Dry-run materializes substituted files, and `jbs continue` resumes those prepared files after checking the stored template hashes.

## `analyse`

An `analyse` block belongs to one step. Pattern assignments search files inside each workpackage directory.

```jbs
analyse run {
        x = "x=(%d)" in "out.txt"
        label = "label=(%w)" in "out.txt"
        (x as "value", label)
}
```

Pattern shortcuts:

- `%d` captures an integer.
- `%f` captures a floating-point value.
- `%w` captures a word.
- `%%` matches a literal percent character.

Plain regular expressions are also allowed. If a pattern has multiple capture groups, result columns are suffixed with `.0`, `.1`, and so on. Multiple matches in one file produce multiple rows. Generated CSV files and SQLite tables include `run_id`.

## Running

`jbs run file.jbs` and `jbs file.jbs` create a benchmark directory named from `jbs_name`. Each run uses the next numeric run id:

```text
benchmark/
  000000/
    status
    step/
      analyse.csv
      000000/
        run.sh
        status
        stdout
        stderr
        exitcode
```

`jbs_benchmarks` defaults to `{}`. When empty, the single-directory layout above is used. When non-empty, it must be a dictionary mapping component names to analyse block names:

```jbs
jbs_benchmarks = {
        "small": ["run_small", "summary"],
        "large": "run_large",
}
```

Each component is written below `<jbs_name>/<component>/` and runs only the steps needed by its requested analyse blocks:

```text
benchmark/
  small/
    000000/
      status
      step/
        ...
  large/
    000000/
      status
      step/
        ...
```

Without `--benchmark`, `jbs run` runs every configured component. `jbs run -b small file.jbs` and `jbs continue -b small file.jbs` operate on one configured component. Selecting a benchmark is an error when `jbs_benchmarks` is empty or the selected name is not configured.

If `jbs_database` is non-empty, analyse results are written to that SQLite database instead of per-step `analyse.csv` files. The database contains one table per analysed step and run. Single-benchmark table names use `<benchmark_name>_<run_id>_<step_name>`, for example `bench_000000_run`. Component table names use `<benchmark_name>_<component>_<run_id>_<step_name>`, for example `bench_small_000000_run`. Later runs create new tables in the same database instead of overwriting previous runs. `jbs continue` rewrites the table for the original run, and command output prints only the current run's tables.

The top-level status file is written last during initial directory creation. This keeps incomplete initializations from being resumable.

`jbs continue file.jbs` resumes interrupted work when the benchmark is not already marked `RUNNING` and the source identity hash matches. With multiple configured components, it resumes all components; use `-b` to resume only one. The hash includes the contents and loader labels of all loaded `.jbs` files plus the contents of any `fsub` templates used by the selected benchmark. File labels are the cleaned absolute paths used by the loader, so continuing through a symlink or alternate absolute path can fail even if the file contents are identical.

Generated workpackage `run.sh` files use `set -euo pipefail` by default. Pass `--no-strict` to `jbs run` or the `jbs file.jbs` shorthand to omit it for newly created run directories.
