# JBS Language

## Canonical Core Syntax

JBS has six canonical top-level statement forms:

- `use`
- top-level assignment
- top-level expression statement
- `do`
- `submit`
- `analyse`

Top-level expression statements remain legal in files. They are evaluated, but normal file-mode YAML output ignores their display result. In practice, they are mainly useful in the REPL and for quick local inspection while editing a file.

Example:

```jbs
use "./lib/math.jbs" as math

jbs_name = "bench"
cases = table(id = id, label = label)

do run
        with cases
{
        echo "${id} ${label}"
}

submit train
        with cases
{
        account = "myacct"
        executable = "/bin/bash"
        args_exec = "-lc hostname"
}

analyse run {
        parsed = "Value: %d" in "out.log"
        (parsed)
}
```

## Grammar

```ebnf
program       := trivia? stmt (sep stmt)* sep? EOF
sep           := (NEWLINE | ";" | comment)+
trivia        := (NEWLINE | comment)*
opt_comment   := comment?
comment       := "#" COMMENT_TEXT

stmt          := use_stmt
               | global_assign
               | expr_stmt
               | do_block
               | submit_block
               | analyse_block

use_stmt      := "use" (
                   IDENT
                 | STRING "as" IDENT
                 | ident_list "from" use_source
               )
ident_list    := IDENT ("," IDENT)*
use_source    := IDENT | STRING

global_assign := IDENT assign_op expr opt_comment
expr_stmt     := expr opt_comment
assign_op     := "=" | "+=" | "-=" | "*=" | "/=" | "%="

qualified_name := IDENT ("." IDENT)*

with_clause   := "with" with_item ("," with_item)*
with_item     := qualified_name ("from" qualified_name)? ("as" IDENT)?
               | "(" qualified_name ("," qualified_name)+ ")" ("from" qualified_name)?
               | qualified_name "[" qualified_name ("," qualified_name)* "]" ("as" IDENT)?
               | "(" qualified_name ("," qualified_name)+ ")" "in" qualified_name

after_clause  := "after" IDENT ("," IDENT)*
use_clause    := "use" IDENT ("," IDENT)*
step_opt_clause := IDENT "=" INT

do_block      := "do" IDENT do_header_item* raw_block
submit_block  := "submit" IDENT submit_header_item* "{" submit_item* "}"
analyse_block := "analyse" IDENT with_clause? analyse_header_item* "{" analyse_item* analyse_tuple opt_comment "}"

do_header_item      := do_header_clause opt_comment | NEWLINE | comment
submit_header_item  := submit_header_clause opt_comment | NEWLINE | comment
analyse_header_item := NEWLINE | comment

do_header_clause     := after_clause | with_clause | step_opt_clause
submit_header_clause := after_clause | use_clause | with_clause | step_opt_clause

raw_block      := "{" RAW_TEXT "}"

submit_item    := submit_stmt | sep
submit_stmt    := submit_key assign_op submit_value opt_comment
submit_value   := expr | raw_block

analyse_item   := analyse_stmt | sep
analyse_stmt   := IDENT assign_op expr ("in" STRING)? opt_comment
analyse_tuple  := "(" analyse_col ("," analyse_col)* ","? ")"
analyse_col    := IDENT ("as" STRING)?
```

Numeric literals use the usual `int`, `float`, and scientific-notation forms. Unary `+` and `-` are parsed as operators, not as part of the number token.

## Statement Separators And Comments

- Top-level statements are separated by a newline or `;`.
- Multiline top-level expressions require explicit backslash-newline continuation.
- JBS uses `#` line comments.
- Comments are preserved by `jbs fmt` around top-level statements and block headers.

Example:

```jbs
use jsc
jsc.systemname
x = (1, 2)
x
```

## Top-Level Assignments

Top-level assignments define reusable global values. A global may hold:

- scalar data
- tuple or list data
- table data built by `table(...)`, `t(...)`, `zip(...)`, `product(...)`, `select(...)`, or `read_csv(...)`
- function values
- imported values projected into the current module

Built-in benchmark globals are also ordinary top-level assignments:

- `jbs_name`
- `jbs_outpath`
- `jbs_comment`

Rules:

- globals are introduced and updated by top-level assignment
- top-level assignments execute in source order
- `name = expr` creates or overwrites the current global value
- `+=`, `-=`, `*=`, `/=`, and `%=` read the current value, compute the operator result, and overwrite the global
- a compound assignment before the first value for that name reports an unknown-variable error
- a global may read only values that are already visible at that point in the file
- `do`, `submit`, and `analyse` blocks use a snapshot of globals visible where the block appears
- module exports use final global values after the module has executed
- `jbs_name` and `jbs_outpath` must evaluate to plain strings without `shell(...)` or `python(...)` mode

Example:

```jbs
jbs_name = "demo"
jbs_outpath = "results"

sizes = (1, 2, 4)
labels = ("small", "medium", "large")
cases = table(label = labels, size = sizes)

seed0 = 1
seed0 += 1
seed1 = seed0 + 1

cases = table(size = sizes)
do first with cases { echo ${size} }
cases = table(size = (8, 16))
do second with cases { echo ${size} }
```

## Top-Level Expression Statements

Bare expression lines are valid top-level statements.

They:

- are parsed and evaluated
- do not create globals or steps
- produce REPL output in interactive mode
- are ignored by normal file-mode YAML generation

Example:

```jbs
use jsc
jsc.systemname
names()
```

## `use`

`use` imports reusable definitions from embedded modules and quoted local `.jbs` modules.

```jbs
use jsc
use "./defaults.jbs" as defaults
use queue, account from "./defaults.jbs"
use add from "./lib/math.jbs"
```

Rules:

- `use <module>` resolves embedded `shared/<module>.jbs` only
- installed bare modules are not implemented yet, so bare names currently mean embedded modules only
- `use "<path>.jbs" as alias` resolves the path relative to the importing file
- selective imports use `from` and may target either an embedded bare module name or a quoted path
- local files must always be imported by quoted path such as `use "./defaults.jbs" as defaults` or `use queue from "./defaults.jbs"`
- chained quoted imports resolve from each importer's own base directory, not from the process working directory
- namespace imports expose members as `alias.name`
- selective imports project chosen members into local scope
- importing a `do` or `submit` symbol also pulls in its required step dependencies
- `analyse` blocks are not importable by symbol name

Migration rule: bare import names are for embedded modules; local files must be quoted paths.

## Expressions

Supported expression forms include:

- string, int, float, and bool literals
- tuples and lists
- identifiers and qualified identifiers
- unary `+`, `-`, `!`
- binary `+`, `-`, `*`, `/`, `%`
- logical `&`, `|`
- comparisons
- conditional `a if cond else b`
- function literals and call expressions
- mode expressions: `shell(...)`, `python(...)`

Builtins:

- `tuple(...)`
- `list(...)`
- `int(...)`
- `float(...)`
- `str(...)`
- `range(...)`
- `rev(...)`
- `table(...)` / `t(...)`
- `zip(...)`
- `product(...)`
- `select(...)`
- `len(...)`
- `names()`, `names(value)`
- `read_csv(path)`
- `filter(values, mask)`
- `map(function_value, values)`
- `reduce(function_value, values)`
- `all(value)`
- `any(value)`

### Function Values

Functions are first-class values.

```jbs
make_adder = function(delta) {
        function(x) {
                x + delta
        }
}

add2 = make_adder(2)
add2(3)
```

Rules:

- functions can be assigned, returned, passed, stored, and imported
- nested functions capture outer locals lexically
- local assignments inside a function body stay local to that function
- local assignments may still use `=` or compound operators; that mutability does not extend to the top level
- function-valued globals are valid in expression contexts
- function-valued globals are not valid `with` sources, `submit ... use ...` sources, or `analyse with ...` imports

### Table Values

JBS now uses explicit table operations instead of operator-overloaded `comb` algebra.

### `table(...)` and `t(...)`

`table(...)` constructs one table from named columns. `t(...)` is a short alias with identical semantics.

- column names come from named arguments
- all columns must have the same length
- `table(...)`/`t(...)` do not broadcast columns
- positional arguments are rejected

Example:

```jbs
ids = (1, 2)
labels = ("a", "b")
cases = t(id = ids, label = labels)
```

### `zip(...)`

`zip(...)` combines existing tables row-by-row.

- every argument must be a table value
- row counts must match exactly
- duplicate column names are rejected
- `zip(...)` does not broadcast rows

Example:

```jbs
env = zip(
        table(host = ("h0", "h1")),
        table(port = (8080, 8081)),
)
```

### `product(...)`

`product(...)` builds the Cartesian product of one or more tables.

- every argument must be a table value
- duplicate column names are rejected
- column order follows argument order

Example:

```jbs
x = (1, 2)
y = ("a", "b", "c")
cases = product(table(x = x), table(y = y))
```

### `select(...)`

`select(...)` projects a subset of columns from a table.

- the first argument must be a table value
- the remaining arguments are identifier selectors
- selector order is preserved

Example:

```jbs
grid = product(table(id = (1, 2)), table(replica = (0, 1)))
view = select(grid, id, replica)
```

### `read_csv(...)`

`read_csv(...)` reads CSV or TSV data and returns one table value.

- the first row is the header row
- header names must be unique valid table column names
- relative paths resolve from the source file that contains the call
- type inference is per column across the full file

Example:

```jbs
cases = read_csv("./cases.csv")
names(cases)
len(cases)
```

## Import Semantics (`with`)

`with` makes data bindings visible to `do`, `submit`, and `analyse`.

Supported forms:

- `with cases`
- `with cases, more_cases`
- `with cases[x]`
- `with lib.cases[x]`
- `with cases[x, y]`
- `with cases[x, y], env[host]`

Rules for `do` and `submit`:

- variables are visible only through explicit `with` imports or inherited `after` dependencies
- importing a full table source exposes all of its columns
- importing selected variables with `with source[col0, col1]` generates a synthetic subset parameter set during lowering
- conflicting visible names from different sources are errors
- `after` also carries inherited visible bindings from dependency steps, including names already inherited by those predecessors

Rules for `analyse`:

- `analyse with ...` is scalar-only
- imported values must be string data bindings
- function-valued globals are rejected

## `do`

`do` defines the shell body for a JUBE step.

```jbs
do <name>
        [after <step0>, <step1>, ...]
        [with <source>, <source2>[<col0>, <col1>, ...], ...]
        [<key>=<int> ...]
{
        # shell commands
}
```

Allowed step header options:

- `max_async=<int>` with `int >= 0`
- `procs=<int>` with `int >= 0`
- `iterations=<int>` with `int >= 1`

`after` declares execution dependencies. A dependent step also inherits variables visible in predecessor steps.

## `submit`

`submit` defines scheduler-facing fields and lowers to JUBE submit templates.

```jbs
submit <name>
        [after <step0>, <step1>, ...]
        [with <source>, <source2>[<col0>, <col1>, ...], ...]
        [use <name0>, <name1>, ...]
        [<key>=<int> ...]
{
        <field> = <expr>
        <raw_field> = {
                # raw shell block
        }
}
```

Current JBS lowering targets Slurm-oriented submit templates.

Notes:

- `with` imports row-varying data used by the submit body
- `submit ... use ...` imports scalar defaults from a scalar global or from a module namespace
- later `use` sources win on key collisions and emit warning `W072`
- raw submit keys are `preprocess` and `postprocess`

Common submit keys include:

- `account`
- `queue`
- `nodes`
- `tasks`
- `threadspertask`
- `timelimit`
- `measurement`
- `starter`
- `args_starter`
- `executable`
- `args_exec`
- `outlogfile`
- `outerrfile`
- `gres`
- `mail`
- `notification`
- `preprocess`
- `postprocess`

## `analyse`

`analyse` targets an existing `do` or `submit` step and lowers to JUBE `patternset`, `analyser`, and `result` sections.

```jbs
analyse <step_name>
        [with <scalar0>, <scalar1>, ...]
{
        helper = <expr>

        p0 = <pattern_expr> in "<file>"
        p1 = <pattern_expr> in "<file>"

        (p0, p1 as "Title")
}
```

Rules:

- the target must be an existing `do` or `submit` step
- helper assignments omit `in "<file>"`
- extraction assignments use `expr in "<file>"`
- extraction expressions must evaluate to strings
- `%d`, `%f`, `%w`, and `%%` are supported in extraction patterns
- the final tuple is required and defines result-table columns
- step-visible variables are available automatically

## Formatter (`jbs fmt`)

`jbs fmt [-s|--strict] <file.jbs>` rewrites a script in place using canonical layout.

Default `fmt` is syntax-only. `-s` / `--strict` also runs import expansion and semantic validation before writing.

Formatting rules:

- global assignments stay in assignment form; the formatter does not invent block syntax
- block headers are emitted as `do <name>`, `submit <name>`, or `analyse <step>`
- `after`, `with`, `use`, and step options are emitted on continuation lines
- braces use canonical block layout
- comments are preserved where possible
- output always ends with a trailing newline

If invalid syntax is present, formatting fails with the parser diagnostic rather than rewriting it into something implicit.

## Diagnostics

All diagnostics include source locations.

Relevant semantic diagnostics for top-level bindings:

- `E100`: unknown variable, including a forward reference or a compound assignment before the first value
- `E301` / `E302` / `E303`: invalid `jbs_name` or `jbs_outpath`
- `E304` / `E305`: invalid scalar or nested list/tuple global value

The full catalog is documented in [docs/diagnostics.md](diagnostics.md).
