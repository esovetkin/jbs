# JBS Language (V1)

## Grammar

```ebnf
program       := stmt* EOF
stmt          := global_assign | param_block | do_block | submit_block | patterns_block | analyse_block

global_assign := IDENT "=" expr NEWLINE

param_block   := "param" IDENT with_clause? "{" param_stmt* final_expr "}"
param_stmt    := IDENT "=" expr NEWLINE
final_expr    := comb_expr NEWLINE

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

patterns_block := "patterns" IDENT "{" pattern_stmt* "}"
pattern_stmt   := IDENT "=" STRING

analyse_block  := "analyse" IDENT "{" analyse_stmt* analyse_tuple "}"
analyse_stmt   := IDENT "=" IDENT "." IDENT "in" STRING
analyse_tuple  := "(" analyse_col ("," analyse_col)* ","? ")"
analyse_col    := IDENT ("as" STRING)?
```

## Expressions

Assignment expressions support:

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

Unsupported syntax (diagnostic emitted):

- function calls
- attribute access
- dict literals
- imports

## Combination Algebra

- `A * B`: cartesian product.
- `A + B`: direct sum (zip).
- precedence: `*` before `+`.
- parentheses supported.

`+` broadcasting behavior:

- if lengths match: normal zip.
- else: cyclic broadcast to `max(len(left), len(right))`.
- warning `W101` emitted at the `+` operator span.

Repeated identifier use in a single combination expression is rejected (`E036`).

## Import Semantics (`with`)

Supported forms:

- `with p2, p3`
- `with x from p2, y, z from p3`
- mixed form: `with x from p2, p3`
- tuple form: `with (x,y) from p2`
- mixed tuple form: `with (x,y) from p2, p3`

In `param`:

- imported names initialize local scope.
- assignment to imported name is local rebinding (copy-on-write).

In `do`/`submit`:

- `with p2` uses whole parameter set.
- `with x from p2` generates a synthetic subset parameterset containing only selected variables.
- `with (x,y) from p2` generates one synthetic subset parameterset for selected variables.
- In mixed form (`with x from p2, p3`), `p3` is treated as whole-parameterset import.
- In mixed tuple form (`with (x,y) from p2, p3`), `p3` is treated as whole-parameterset import.

## Lowering to JUBE YAML

### `param` lowering

All paramsets lower to indexed representation:

```yaml
parameterset:
  - name: grouped
    parameter:
      - { name: _jbs__idx_grouped, type: int, mode: text, _: "0,1,2" }
      - { name: a, mode: python, _: "[1,2,1][$_jbs__idx_grouped]" }
      - { name: b, mode: python, _: "['x','y','z'][$_jbs__idx_grouped]" }
```

This keeps direct-sum alignment and outer-product expansion explicitly coordinated by a context-specific index variable.

### `do` lowering

- emits one `step`.
- `depend` is comma-separated from `after`.
- normalizes raw block indentation and preserves only block content.

### `submit` lowering

- emits synthetic submit parameterset with `init_with: "platform.xml:systemParameter"`.
- emits only submit keys explicitly set in the block.
- `preprocess` and `postprocess` are raw-block keys.
  - both are emitted from normalized raw content only (no injected preamble).
- expression keys support scalar/container values and `shell("...")` / `python("...")`.
- emits submit step operations:
  - `${submit} --parsable ${submit_script} > run.jobid`
  - `echo "true" > success`

### `patterns` + `analyse` lowering

`patterns` and `analyse` compile to JUBE `patternset`, `analyser`, and `result`.

Placeholder expansion in `patterns` values:

- `%d` -> `$jube_pat_int` (inferred type `int`)
- `%f` -> `$jube_pat_fp` (inferred type `float`)
- `%w` -> `$jube_pat_wrd` (inferred type `string`)
- `%%` -> literal `%`
- `%s` is rejected (`E402`)

Example:

```jbs
patterns p {
  number = "Number: %d"
  letter = "Letter: %w"
}

analyse write {
  p0 = p.number in "en"
  p1 = p.letter in "en"
  (a, p0, p1 as "letter")
}
```

Lowering shape:

```yaml
patternset:
  - name: p
    pattern:
      # From analyse 'write': alias 'p0' for pattern 'p.number'
      - name: _jbs_pattern__p_number__write__p0
        type: int
        _: 'Number: $jube_pat_int'
      # From analyse 'write': alias 'p1' for pattern 'p.letter'
      - name: _jbs_pattern__p_letter__write__p1
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
          _: _jbs_pattern__p_number__write__p0
        - title: letter
          _: _jbs_pattern__p_letter__write__p1
```

## Built-in Globals

- `jbs_name` (root `name`)
- `jbs_outpath` (root `outpath`)

Rules:

- globals can be assigned only at top-level
- unknown globals are compile errors (`E300`)
- `jbs_name` and `jbs_outpath` must be plain string literals

Examples:

```jbs
jbs_name = "demo"
jbs_outpath = "results"
```

Invalid examples:

```jbs
jbs_name = python("x")   # E303
jbs_outpath = 12         # E302
unknown_name = "x"       # E300
```

Run `jbs help globals` to print defaults and mapping.

## Formatter (`jbs fmt`)

`jbs fmt <file.jbs>` rewrites a script in place using canonical layout.

Rules:

- one blank line between top-level statements
- global assignments emitted as `name = value`
- block header on first line (`param|do|submit <name>`)
- `after` and `with` clauses emitted on dedicated continuation lines with 8 spaces
- opening brace `{` on its own line
- block body indentation normalized to 8 spaces
- closing brace `}` at column 1
- output always ends with a trailing newline

Submit formatting constraints:

- expression fields stay `key = expr`
- raw fields stay `key = { ... }`
- formatter does not change submit key semantics

## Diagnostics

All diagnostics include source location (`file:line:column`).

Key codes:

- `E020`: unknown imported parameterset.
- `E021`: unknown imported variable.
- `E036`: repeated identifier in combination expression.
- `E042`: conflicting key values during row merge.
- `E072`: unknown submit key.
- `E073`: `preprocess`/`postprocess` require raw-block values.
- `E074`: non-raw submit keys cannot use raw-block values.
- `E075`: duplicate submit key.
- `E076`: malformed submit statement.
- `E400`: duplicate patterns block name.
- `E401`: duplicate pattern name in patterns block.
- `E402`: invalid pattern placeholder.
- `E410`: unknown analyse target step.
- `E411`: unknown pattern group in analyse assignment.
- `E412`: unknown pattern name in analyse assignment.
- `E413`: analyse alias collides with step-visible variable.
- `E414`: duplicate analyse alias.
- `E415`: unknown symbol in analyse result tuple.
- `E416`: malformed analyse assignment syntax.
- `E417`: analyse block missing final tuple.
- `W101`: `+` length mismatch, cyclic broadcast applied.
- `W310`: exposed param variable is never referenced in any `do`/`submit` body via `$name` or `${name}`.
- `W311`: step references `$name`/`${name}` for a known param variable but the corresponding paramset is not imported via `with`.

For `W310`/`W311`, reference scanning applies to:

- `do` block body text
- submit raw blocks (`preprocess`, `postprocess`)
- string literals in expression-valued submit keys (all keys, not only `args_exec`)

## Known Limitations

- YAML emission only.
- no XML emission.
