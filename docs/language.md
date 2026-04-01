# JBS Language

## Grammar

```ebnf
program       := stmt (sep stmt)* sep? EOF
sep           := (NEWLINE | ";")+
stmt          := global_assign | let_block | param_block | do_block | submit_block | analyse_block

global_assign := IDENT "=" expr

let_block     := "let" IDENT "{" let_stmt* "}"
let_stmt      := IDENT "=" expr

param_block   := "param" IDENT with_clause? "{" param_stmt* final_expr "}"
param_stmt    := IDENT "=" expr
final_expr    := comb_expr

with_clause   := "with" with_item ("," with_item)*
with_item     := IDENT ("from" IDENT)?
              | "(" IDENT ("," IDENT)+ ")" ("from" IDENT)?

do_block      := "do" IDENT after_clause? with_clause? raw_block
submit_block  := "submit" IDENT after_clause? with_clause? "{" submit_stmt* "}"

after_clause  := "after" IDENT ("," IDENT)*
raw_block     := "{" RAW_TEXT "}"

submit_stmt   := submit_key "=" submit_value
submit_key    := "account" | "args_exec" | "args_starter" | "executable" |
                 "gres" | "mail" | "measurement" | "nodes" |
                 "notification" | "outlogfile" | "outerrfile" | "queue" |
                 "starter" | "tasks" | "threadspertask" | "timelimit" |
                 "preprocess" | "postprocess"
submit_value  := expr | raw_block

analyse_block := "analyse" IDENT "{" analyse_stmt* analyse_tuple "}"
analyse_stmt  := IDENT "=" expr ("in" STRING)?
analyse_tuple := "(" analyse_col ("," analyse_col)* ","? ")"
analyse_col   := (IDENT | IDENT "." IDENT) ("as" STRING)?
```

## Statement Separators

In structural blocks (`let`, `param`, `analyse`, `submit`) and top-level global assignments, statements can be separated by newline or `;`.

Multiline expressions require explicit backslash-newline continuation (`\\\n`).
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

## Expressions

Supported assignment expressions:

- scalar literals: string/int/float/bool
- tuples/lists
- identifiers
- qualified identifiers: `namespace.variable`
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
- imports

## Combination Algebra

- `A * B`: Cartesian product.
- `A + B`: direct sum (zip).
- operator precedence: `*` before `+`.
- parentheses are supported.

`+` broadcasting behavior:

- if lengths match: normal zip.
- else: cyclic broadcast to `max(len(left), len(right))`.
- warning `W101` is emitted at the `+` operator span only when the shorter length does not divide the longer one.

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

`namespace.variable` can be used in `param`, `submit`, and `analyse` expressions.

`param` can import a full let namespace into local scope:

```jbs
param cases with p {
        x = (1, 2)
        y = (number, letter)
        x + y
}
```

Nested tuples/lists are rejected (`E305`) in `let`, `param`, submit expression fields, and analyse helper assignments.

## Import Semantics (`with`)

Supported forms:

- `with p2, p3`
- `with x from p2, y, z from p3`
- mixed form: `with x from p2, p3`
- tuple form: `with (x, y) from p2`
- mixed tuple form: `with (x, y) from p2, p3`
- let import forms in `param`:
  - `with l`
  - `with x from l`
  - `with (x, y) from l`

In `do`/`submit`:

- `with p2` uses a whole parameter set.
- `with x from p2` generates a synthetic subset parameter set containing only selected variables.
- `with (x, y) from p2` generates one subset parameter set for selected variables.
- `after` implies parameter inheritance from dependency steps.
- if `after` already provides a variable from the same source parameter set, explicit `with` re-import of that variable is ignored.
- if explicit `with` targets a whole parameter set after inheritance, only non-inherited variables from that parameter set are imported.
- if the same variable name is inherited/imported from different parameter sets, compilation fails.
- inherited imports also carry source-row context from their source parameter set.
- when a dependent step imports additional variables from the same source, jbs refines that inherited source-row context instead of creating an independent Cartesian dimension.
- this source-row context propagation is transitive across `after` chains (for example, `step0 -> step1 -> step2`).

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

Compact jbs source for the indexed YAML example above:

```jbs
param grouped {
        a = (1, 2)
        b = ("x", "y", "z")
        a + b
}
```

### `do` lowering

- emits one `step` entry.
- sets `depend` as a comma-separated list from `after`.
- keeps raw block content as the step command body.

### `submit` lowering

- emits a synthetic submit parameter set with `init_with: "platform.xml:systemParameter"`.
- emits only submit keys explicitly set in the block.
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

analyse write {
  p0 = p.number in "en"
  p1 = p.letter in "en"
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

Rules:

- globals can be assigned only at the top level.
- unknown globals are compile errors (`E300`).
- `jbs_name` and `jbs_outpath` must be plain string literals.

Examples:

```jbs
jbs_name = "demo"
jbs_outpath = "results"
```

Invalid examples:

```jbs
jbs_name = python("x")   # E303
jbs_outpath = 12          # E302
unknown_name = "x"       # E300
```

Run `jbs help globals` to print defaults and mappings.

## Formatter (`jbs fmt`)

`jbs fmt <file.jbs>` rewrites a script in place using canonical layout.

Rules:

- one blank line between top-level statements.
- global assignments are emitted as `name = value`.
- block header on the first line (`param|do|submit|let|analyse <name>`).
- `after` and `with` clauses are emitted on dedicated continuation lines with 8 spaces.
- opening brace `{` is on its own line.
- block body indentation is normalized to 8 spaces.
- closing brace `}` is at column 1.
- output always ends with a trailing newline.

Submit formatting constraints:

- expression fields stay `key = expr`.
- raw fields stay `key = { ... }`.
- formatter does not change submit key semantics.

## Diagnostics

All diagnostics include source location (`file:line:column`).

Key codes:

- `E020`: unknown imported source in `with`.
- `E021`: unknown imported variable.
- `E022`: ambiguous `with` source (name matches both `param` and `let`).
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
- `E410`: unknown analyse target step.
- `E412`: analyse extraction expression does not evaluate to string.
- `E413`: analyse extraction alias collides with a step-visible variable.
- `E414`: duplicate analyse variable name.
- `E415`: unknown symbol in analyse result tuple.
- `E416`: malformed analyse assignment syntax.
- `E417`: analyse block missing or malformed final tuple.
- `W101`: `+` length mismatch with non-divisible lengths; cyclic broadcast applied.
- `W300`: top-level global reassigned; last value wins.
- `W310`: exposed param variable is never referenced in any `do`/`submit` body via `$name` or `${name}`.
- `W311`: step references `$name`/`${name}` for a known param variable, but the corresponding paramset is not imported via `with`.
- `W320`: analyse helper variable shadows a step-visible variable.

For `W310`/`W311`, reference scanning applies to:

- `do` block body text.
- submit raw blocks (`preprocess`, `postprocess`).
- string literals in expression-valued submit keys.

## Known Limitations

- YAML emission only.
- No XML emission.
