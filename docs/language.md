# JBS Language

## Canonical Core Syntax

JBS has six canonical top-level statement forms:

- `use`
- top-level assignment
- top-level expression statement
- `do`
- `submit`
- `analyse`

Legacy top-level `let` and `param` blocks are no longer part of the language. The parser reports `E067` with a migration hint when it encounters them.

Top-level expression statements remain legal in files. They are evaluated, but normal file-mode YAML output ignores their display result. In practice, they are mainly useful in the REPL and for quick local inspection while editing a file.

Example:

```jbs
use "./lib/math.jbs" as math

jbs_name = "bench"
cases = comb(id + label)

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

global_assign := IDENT "=" expr opt_comment
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
- `comb(...)` table data
- function values
- imported values projected into the current module

Built-in benchmark globals are also ordinary top-level assignments:

- `jbs_name`
- `jbs_outpath`
- `jbs_comment`

Rules:

- `jbs_name` and `jbs_outpath` must be plain string literals
- globals are introduced only by top-level assignment
- top-level bindings are immutable and must use plain `=`
- each top-level name may be defined once
- top-level evaluation is still dependency-based, so a global may read another global defined later in the file as long as that later name is defined exactly once
- top-level compound assignment reports `E307`
- duplicate top-level definitions report `E306`

Example:

```jbs
jbs_name = "demo"
jbs_outpath = "results"

sizes = (1, 2, 4)
labels = ("small", "medium", "large")
cases = comb(labels + sizes)

seed0 = 1
seed1 = seed0 + 1
seed2 = seed1 + 1
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

`use` imports reusable definitions from embedded or local `.jbs` modules.

```jbs
use jsc
use "./defaults.jbs" as defaults
use queue, account from "./defaults.jbs"
use add from "./lib/math.jbs"
```

Rules:

- `use <module>` resolves embedded `shared/<module>.jbs` first, then local `./<module>.jbs` relative to the current working directory
- `use "<path>.jbs" as alias` resolves the path relative to the importing file
- selective imports use `from` and may target either a bare module name or a quoted path
- namespace imports expose members as `alias.name`
- selective imports project chosen members into local scope
- importing a `do` or `submit` symbol also pulls in its required step dependencies
- `analyse` blocks are not importable by symbol name

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
- alias expressions `expr as IDENT` inside comb construction

Builtins:

- `tuple(...)`
- `list(...)`
- `int(...)`
- `float(...)`
- `str(...)`
- `range(...)`
- `rev(...)`
- `comb(...)`
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

### `comb(...)`

`comb(...)` evaluates combination algebra and returns a table-like value that can be imported into `do` and `submit`.

- `A * B` is Cartesian product
- `A + B` is zip-like direct sum with cyclic broadcasting
- `*` binds tighter than `+`
- duplicate output column names are rejected
- unnamed non-identifier leaves are rejected; use `as` to name them

Example:

```jbs
x = (1, 2)
y = ("a", "b", "c")
cases = comb(x * y)
```

### `read_csv(...)`

`read_csv(...)` reads CSV or TSV data and returns one `comb` value.

- the first row is the header row
- header names must be unique valid comb column names
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
- `with x from cases`
- `with x from lib.cases`
- `with (x, y) from cases`
- `with cases[x, y]`
- `with cases[x, y] as pair`
- `with (x, y) in cases`

Rules for `do` and `submit`:

- variables are visible only through explicit `with` imports or inherited `after` dependencies
- importing a full comb source exposes all of its columns
- importing individual variables generates a synthetic subset parameter set during lowering
- conflicting visible names from different sources are errors
- `after` also carries inherited visible bindings from dependency steps

Rules for `analyse`:

- `analyse with ...` is scalar-only
- imported values must be string data bindings
- function-valued globals are rejected

## `do`

`do` defines the shell body for a JUBE step.

```jbs
do <name>
        [after <step0>, <step1>, ...]
        [with <source>, <var> from <source2>, ...]
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
        [with <source>, <var> from <source2>, ...]
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

If removed legacy syntax such as top-level `let` or `param` is present, formatting fails with the parser diagnostic rather than rewriting it into something implicit.

## Diagnostics

All diagnostics include source locations.

Relevant parser/alignment diagnostics in this core language:

- `E067`: legacy top-level `let` / `param` block encountered

Relevant semantic diagnostics for top-level bindings:

- `E306`: duplicate top-level binding
- `E307`: top-level compound assignment is not allowed

The full catalog is documented in [docs/diagnostics.md](diagnostics.md).
