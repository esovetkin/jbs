# JBS Language

## Compiler Layout (Contributor Note)

The compiler is split by feature boundary to keep changes local and safer:

- `internal/parser/`
  - `parser.go`: parser entry point and top-level dispatch
  - `parser_blocks.go`, `parser_clauses.go`, `parser_with_items.go`: block/header parsing
  - `parser_bodies_param_let_analyse.go`, `parser_submit_fields.go`: block body parsing
  - `parser_expr.go`, `parser_comb_expr.go`: expression and combination parsing
  - `parser_scan.go`, `parser_toplevel.go`: scanner/trivia and top-level statement helpers
- `internal/sema/`
  - `analyze.go`: semantic orchestration
  - `compile_*.go`: `let`, `param`, `submit`, `analyse` compilation
  - `steps_*.go`, `step_visibility.go`: step validation and import planning
  - `refs_validate.go`: shell/expression reference scanning and usage warnings
  - `globals_resolve.go`, `imports_sources.go`: globals and source import materialization
- `internal/lower/`
  - `types.go`: JUBE YAML document model types
  - `to_jube_yaml.go`: lowering orchestration
  - `lower_*.go`: focused lowering stages (params, steps, subsets, analyse/result, names, shell rewrite, raw block normalization)

The intent is behavior-neutral modularity: parser, sema, and lowering logic stay in their original packages, but each file now maps to one feature area.

## Grammar

```ebnf
program       := stmt (sep stmt)* sep? EOF
sep           := (NEWLINE | ";")+
stmt          := use_stmt | global_assign | let_block | param_block | do_block | submit_block | analyse_block

use_stmt      := "use" (
                   IDENT
                 | STRING "as" IDENT
                 | ident_list "from" use_source
               )
ident_list    := IDENT ("," IDENT)*
use_source    := IDENT | STRING

global_assign := IDENT "=" expr

let_block     := "let" IDENT "{" let_stmt* "}"
let_stmt      := IDENT "=" expr

param_block   := "param" IDENT with_clause? "{" param_stmt* final_expr "}"
param_stmt    := IDENT "=" expr
final_expr    := comb_expr

with_clause   := "with" with_item ("," with_item)*
qualified_name := IDENT ("." IDENT)*
with_item     := qualified_name ("from" qualified_name)?
              | "(" qualified_name ("," qualified_name)+ ")" ("from" qualified_name)?

do_block      := "do" IDENT do_header_clause* raw_block
submit_block  := "submit" IDENT submit_header_clause* "{" submit_stmt* "}"

do_header_clause := after_clause | with_clause | step_opt_clause
submit_header_clause := after_clause | use_clause | with_clause | step_opt_clause

after_clause  := "after" IDENT ("," IDENT)*
use_clause    := "use" IDENT ("," IDENT)*
step_opt_clause := "max_async" "=" INT | "iterations" "=" INT
raw_block     := "{" RAW_TEXT "}"

submit_stmt   := submit_key "=" submit_value
submit_key    := "account" | "args_exec" | "args_starter" | "executable" |
                 "gres" | "mail" | "measurement" | "nodes" |
                 "notification" | "outlogfile" | "outerrfile" | "queue" |
                 "starter" | "tasks" | "threadspertask" | "timelimit" |
                 "preprocess" | "postprocess"
submit_value  := expr | raw_block

analyse_block := "analyse" IDENT with_clause? "{" analyse_stmt* analyse_tuple "}"
analyse_stmt  := IDENT "=" expr ("in" STRING)?
analyse_tuple := "(" analyse_col ("," analyse_col)* ","? ")"
analyse_col   := IDENT ("as" STRING)?
```

## Statement Separators

In structural blocks (`let`, `param`, `analyse`, `submit`) and top-level global assignments, statements can be separated by a newline or `;`.

Multiline expressions require explicit backslash-newline continuation (`\n`).
Implicit operator-based newline continuation is not supported.

Example:

```jbs
let p { a = 1; b = 2 }
param q { x = (1,2); y = ("a","b"); x + y; }
analyse step { n = "N: %d" in "out"; (n); }
```

Backslash continuation example:

```jbs
param p {
        x = 1 + \
            2 + 3
        x
}
```

## Step Options (`max_async`, `iterations`)

`do` and `submit` headers support optional step options:

- `max_async=<int>` with `int >= 0`
- `iterations=<int>` with `int >= 1`

These clauses can appear on one line or across multiple lines and can be interleaved with `after`, `with`, and submit-header `use`.

Example:

```jbs
do prep
        with p
        max_async=0 iterations=2
{
        echo prep
}

submit run
        after prep
        with p
        max_async=3 iterations=4
{
        args_exec = "-lc hostname"
}
```

## Expressions

Supported assignment expressions:

- scalar literals: string/int/float/bool
- tuples/lists
- identifiers
- unary `+`, `-`
- binary `+`, `-`, `*`, `/`, `%`
- comparison operators
- `and`, `or`
- conditional expression: `a if cond else b`
- mode declarations:
  - `shell("...")`
  - `python("...")`

Mode declarations lower to JUBE parameter mode fields:

```yaml
- name: some_param
  mode: shell
  _: "..."
```

```yaml
- name: some_param
  mode: python
  _: '...'
```

Unsupported syntax (diagnostics emitted):

- function calls
- dict literals
- import statements

## Combination Algebra

- `A * B`: Cartesian product.
- `A + B`: direct sum (zip).
- Operator precedence: `*` before `+`.
- Parentheses are supported.

`+` broadcasting behavior:

- If lengths match, a normal zip is used.
- Otherwise, cyclic broadcasting is applied to `max(len(left), len(right))`.
- Warning `W101` is emitted at the `+` operator span only when the shorter length does not divide the longer one.

Repeated identifier use in a single combination expression is rejected (`E036`).

Examples:

```jbs
param ex_zip {
        x = (1, 2)
        y = ("a", "b")

        # direct sum (zip), equal lengths
        # yields [(x=1, y="a"), (x=2, y="b")]
        x + y
}

param ex_product {
        x = (1, 2)
        y = ("a", "b", "c")

        # Cartesian product
        # yields [
        #   (x=1, y="a"), (x=1, y="b"), (x=1, y="c"),
        #   (x=2, y="a"), (x=2, y="b"), (x=2, y="c")
        # ]
        x * y
}

param ex_precedence {
        x = (1, 2)
        y = ("a", "b")
        z = ("L", "R")

        # '*' binds before '+': x + (y * z)
        # y * z yields [
        #   (y="a", z="L"), (y="a", z="R"),
        #   (y="b", z="L"), (y="b", z="R")
        # ]
        # then x is broadcast to length 4:
        # yields [
        #   (x=1, y="a", z="L"), (x=2, y="a", z="R"),
        #   (x=1, y="b", z="L"), (x=2, y="b", z="R")
        # ]
        x + y * z
}

param ex_parentheses {
        x = (1, 2)
        y = ("a", "b")
        z = ("L", "R")

        # parentheses change grouping: (x + y) * z
        # x + y yields [(x=1, y="a"), (x=2, y="b")]
        # outer product with z yields [
        #   (x=1, y="a", z="L"), (x=1, y="a", z="R"),
        #   (x=2, y="b", z="L"), (x=2, y="b", z="R")
        # ]
        (x + y) * z
}

param ex_broadcast_warn {
        x = (1, 2)
        y = ("a", "b", "c")

        # non-matching lengths: 2 + 3
        # cyclic broadcast to length 3:
        # yields [(x=1, y="a"), (x=2, y="b"), (x=1, y="c")]
        # emits W101 because 3 % 2 != 0
        x + y
}

param ex_broadcast_no_warn_divisible {
        x = (1, 2)
        y = ("a", "b", "c", "d")

        # non-matching lengths: 2 + 4
        # cyclic broadcast to length 4:
        # yields [
        #   (x=1, y="a"), (x=2, y="b"),
        #   (x=1, y="c"), (x=2, y="d")
        # ]
        # no W101 because 4 % 2 == 0
        x + y
}

param ex_scalar_like {
        x = [1, 2, 3]
        c = "const"

        # c behaves like length-1 and is broadcast in '+'
        x + c
}
```

## `let` Namespaces

`let` defines a namespace of reusable values.

```jbs
let p {
        number = "Number: %d"
        letter = "Letter: %w"
        retries = 3
}
```

Import variables with `with`.

`param` can import a full let namespace into local scope:

```jbs
param cases with p {
        x = (1, 2)
        y = (number, letter)
        x + y
}
```

Tuple/list values are rejected in `let` (`E403`).
Nested tuples/lists are rejected (`E305`) in `param`, submit expression fields, and `analyse` helper assignments.

## Import Semantics (`with`)

Supported forms:

- `with p2, p3`
- `with x from p2, y, z from p3`
- mixed form: `with x from p2, p3`
- tuple form: `with (x, y) from p2`
- mixed tuple form: `with (x, y) from p2, p3`
- qualified source form: `with lib.p2`
- qualified `from` form: `with x from lib.p2`
- let import forms in `param`:
  - `with l`
  - `with x from l`
  - `with (x, y) from l`

In `do`/`submit`:

- `with p2` uses a whole parameter set.
- `with x from p2` generates a synthetic subset parameter set containing only selected variables.
- `with (x, y) from p2` generates one subset parameter set for selected variables.
- `after` implies parameter inheritance from dependency steps.
- If `after` already provides a variable from the same source parameter set, explicit `with` re-import of that variable is ignored.
- If explicit `with` targets a whole parameter set after inheritance, only non-inherited variables from that parameter set are imported.
- If the same variable name is inherited/imported from different parameter sets, compilation fails.
- Inherited imports also carry source-row context from their source parameter set.
- When a dependent step imports additional variables from the same source, JBS refines that inherited source-row context instead of creating an independent Cartesian dimension.
- This source-row context propagation is transitive across `after` chains (for example, `step0 -> step1 -> step2`).

In `param`:

- `with` can import from `param` and `let` sources.
- If the same visible variable name is imported from different sources in one `param` block, compilation fails with `E214`.
- Importing the same variable name repeatedly from the same source is allowed.

In `analyse`:

- `with` imports are allowed only from `let` namespaces.
- Imported let variables in `analyse` must be strings (`E422`).
- `with` imports from `param` are rejected (`E420`).

In submit headers:

- `submit ... use <name>` is special and accepts only `let` namespaces.
- Multiple submit-header `use` clauses are allowed and merged in order.
- Later `use` namespaces override earlier ones for the same submit key (last-win).
- Collisions across different `use` namespaces for the same submit key emit warning `W072`.
- Using a `param` source in submit-header `use` is rejected (`E071`).

Example:

```jbs
let defaults {
  queue = "batch"
}

let gpu_defaults {
  queue = "devel"
  gres = "gpu:4"
}

submit run
  use defaults
  use gpu_defaults
{
  args_exec = "-lc hostname"
}
```

`queue` resolves to `devel` (from `gpu_defaults`) and emits `W072` because both namespaces define `queue`.

## Lowering to JUBE YAML

### `param` lowering

All parameter sets lower to indexed representation:

```yaml
# jbs source:
# param grouped {
#         a = (1, 2)
#         b = ("x", "y", "z")
#         a + b
# }
parameterset:
  - name: grouped
    parameter:
      - { name: _ji_grouped, type: int, mode: text, _: "0,1,2" }
      - { name: a, mode: python, _: "[1,2,1][$_ji_grouped]" }
      - { name: b, mode: python, _: "['x','y','z'][$_ji_grouped]" }
```

### `do` lowering

- emits one `step` entry.
- sets `depend` as a comma-separated list from `after`.
- keeps raw block content as the step command body.

### `submit` lowering

- emits a synthetic submit parameter set with `init_with: "platform.xml:systemParameter"`.
- emits submit keys explicitly set in the block.
- if an imported `param`/`let` variable name collides with a submit key (for example `nodes`), the imported variable is renamed to `_ja__<name>` in generated YAML.
- submit keys keep their original names.
- for collided names, jbs rewrites `$name`/`${name}` references in:
  - submit raw blocks (`preprocess`, `postprocess`)
  - string-valued explicit submit expressions (for example `nodes = "${nodes}"`)
- auto-injects `tasks` when missing:
  - if `nodes` is set/resolved, `tasks` is set to the same value.
  - otherwise `tasks` is set to `$nodes`.
- `preprocess` and `postprocess` are raw-block keys.
- no implicit preamble is injected into `do`/`submit` raw blocks.
- emits submit step operations:
  - `${submit} --parsable ${submit_script} > run.jobid`
  - `echo "true" > success`

### `let` + `analyse` lowering

`let` and `analyse` compile to JUBE `patternset`, `analyser`, and `result`.

Placeholder expansion in extraction expressions:

- `%d` -> `$jube_pat_int` (inferred type `int`)
- `%f` -> `$jube_pat_fp` (inferred type `float`)
- `%w` -> `$jube_pat_wrd` (inferred type `string`)
- `%%` -> literal `%`
- `%s` is rejected (`E402`)

Example:

```jbs
let p {
  number = "Number: %d"
  letter = "Letter: %w"
}

analyse write with p {
  p0 = number in "en"
  p1 = letter in "en"
  # `as "letter"` sets the output column name for `p1`;
  # columns for `a` and `p0` keep their original names.
  (a, p0, p1 as "letter")
}
```

Lowering shape:

```yaml
patternset:
  - name: p
    pattern:
      # From analyse 'write': alias 'p0' for pattern 'p.number'
      - name: _jp__p_number__write__p0
        type: int
        _: 'Number: $jube_pat_int'
      # From analyse 'write': alias 'p1' for pattern 'p.letter'
      - name: _jp__p_letter__write__p1
        type: string
        _: 'Letter: $jube_pat_wrd'

analyser:
  - name: analyser_write
    use: p
    analyse:
      - step: write
        file:
          - use: p
            _: en

result:
  use:
    - analyser_write
  table:
    - name: result_write
      style: csv
      column:
        - title: a
          _: a
        - title: p0
          _: _jp__p_number__write__p0
        - title: letter
          _: _jp__p_letter__write__p1
```

Inline extraction expressions in `analyse` create synthetic pattern groups of the form `_ja_<step>_<alias>`.

## Built-in Globals

- `jbs_name` (root `name`)
- `jbs_outpath` (root `outpath`)
- `jbs_comment` (root `comment`)

Rules:

- Globals can be assigned only at the top level.
- Unknown globals are compile errors (`E300`).
- `jbs_name` and `jbs_outpath` must be plain string literals.

Examples:

```jbs
jbs_name = "demo"
jbs_outpath = "results"
jbs_comment = "My benchmark note"
```

Invalid examples:

```jbs
jbs_name = python("x")   # E303
jbs_outpath = 12          # E302
unknown_name = "x"       # E300
```

Run `jbs help globals` to print defaults and mappings.

## Formatter (`jbs fmt`)

`jbs fmt [-s|--strict] <file.jbs>` rewrites a script in place using canonical layout.

Default `fmt` is syntax-only. Use `-s`/`--strict` to require import expansion and semantic validation before formatting.

Rules:

- One blank line between top-level statements.
- Global assignments are emitted as `name = value`.
- The block header is on the first line (`param|do|submit|let|analyse <name>`).
- `after` and `with` clauses are emitted on dedicated continuation lines with 8 spaces.
- The opening brace `{` is on its own line.
- Block body indentation is normalized to 8 spaces.
- The closing brace `}` is at column 1.
- Output always ends with a trailing newline.

Submit formatting constraints:

- Expression fields stay `key = expr`.
- Raw fields stay `key = { ... }`.
- The formatter does not change submit key semantics.

## Diagnostics

All diagnostics include source location (`file:line:column`).

Key codes:

- `E020`: unknown imported source in `with`.
- `E021`: unknown imported variable.
- `E022`: ambiguous `with` source (name matches both `param` and `let`).
- `E214`: conflicting variable imported from multiple sources in `with`.
- `E036`: repeated identifier in combination expression.
- `E042`: conflicting key values during row merge.
- `E072`: unknown submit key.
- `E073`: `preprocess`/`postprocess` require raw-block values.
- `E074`: non-raw submit keys cannot use raw-block values.
- `E075`: duplicate submit key.
- `E076`: malformed submit statement.
- `E300`: unknown global variable.
- `E301`: `jbs_name` must be a string literal.
- `E302`: `jbs_outpath` must be a string literal.
- `E303`: `jbs_name`/`jbs_outpath` cannot use `shell()`/`python()`.
- `E304`: unsupported global value kind (must be scalar).
- `E305`: nested tuple/list value is not allowed.
- `E400`: duplicate `let` block name.
- `E401`: duplicate variable name in a `let` block.
- `E402`: invalid placeholder in analyse extraction expression.
- `E403`: let variable must be scalar.
- `E410`: unknown analyse target step.
- `E412`: analyse extraction expression does not evaluate to string.
- `E413`: analyse extraction alias collides with a step-visible variable.
- `E414`: duplicate analyse variable name.
- `E415`: unknown symbol in analyse result tuple.
- `E416`: malformed analyse assignment syntax.
- `E417`: analyse block missing or malformed final tuple.
- `E420`: analyse with-clause imports from non-let source.
- `E422`: analyse with-clause imported let variable is not string-valued.
- `W101`: `+` length mismatch with non-divisible lengths; cyclic broadcast applied.
- `W300`: top-level global reassigned; last value wins.
- `W310`: exposed param variable is never referenced in any `do`/`submit` body via `$name` or `${name}`.
- `W311`: step references `$name`/`${name}` for a known param variable, but the corresponding paramset is not imported via `with`.
- `W312`: variable declared in a `param` block does not contribute to the final combination expression (directly or transitively).
- `W320`: analyse helper variable shadows a step-visible variable.
- `W072`: submit default key is defined in multiple submit-header `use` namespaces; later namespace wins.
- `W073`: submit key `account` or `queue` is missing or resolves to an empty string.
- `W074`: submit keys `executable` and `args_exec` are both missing or resolve to empty strings while `starter` is set to a non-empty value.

For `W310`/`W311`, reference scanning applies to:

- `do` block body text.
- submit raw blocks (`preprocess`, `postprocess`).
- string literals in expression-valued submit keys.
