# JBS Language

JBS files are evaluated top to bottom. They contain global assignments, imports, `do` blocks, `analyse` blocks, and top-level expression statements.

## Program Shape

```text
program       := statement*
statement     := assignment | use_stmt | do_block | analyse_block | expr_stmt
assignment    := IDENT assign_op expr
assign_op     := "=" | "+=" | "-=" | "*=" | "/=" | "%="
use_stmt      := "use" import_items "from" STRING
do_block      := "do" IDENT header_item* "{" raw_body "}"
analyse_block := "analyse" IDENT analyse_header_item* "{" analyse_body "}"
```

Top-level expression statements are evaluated. In files they are mainly useful for validation and quick inspection; in the REPL their values are printed.

`print(...)` writes explicit output using the same value rendering as the REPL. During `jbs run file.jbs`, print output is written to command stdout before benchmark work starts; shell stdout from `run.sh` is still captured in workpackage `stdout` files.

## Built-In Globals

`jbs_name` names the benchmark directory. It defaults to `jbs_benchmark` and must evaluate to a string.

`jbs_nproc` is the global concurrency limit. It defaults to `0`. A value of `0` means the number of available CPUs.

```jbs
jbs_name = "sweep"
jbs_nproc = 8
```

## Values

JBS supports:

- `int`, `float`, `str`, `bool`, and `null`
- lists: `[1, 2, 3]`
- tuples: `(1, 2, 3)`
- tables, created with `table(...)` or `t(...)`
- functions: `function(x) { x + 1 }`

Tuple syntax requires a comma for one item: `(1,)`.

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
- `nproc N` limits concurrent workpackages for this step.

`nproc 0` means the number of available CPUs. The effective step concurrency is limited by both `jbs_nproc` and the step's own `nproc`.

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

Plain regular expressions are also allowed. If a pattern has multiple capture groups, result columns are suffixed with `.0`, `.1`, and so on. Multiple matches in one file produce multiple rows. Generated CSV files include `run_id`.

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

The top-level status file is written last during initial directory creation. This keeps incomplete initializations from being resumable.

`jbs continue file.jbs` resumes interrupted work when the script hash matches and the benchmark is not already marked `RUNNING`.

Generated workpackage `run.sh` files use `set -euo pipefail` by default. Pass `--no-strict` to `jbs run` or the `jbs file.jbs` shorthand to omit it for newly created run directories.
