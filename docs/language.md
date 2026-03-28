# JBS Language (V1)

## Grammar

```ebnf
program       := stmt* EOF
stmt          := global_assign | param_block | do_block | submit_block

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
```

## Expressions

Assignment expressions support:

- scalar literals: string/int/float/bool
- tuples/lists/dicts
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
      - { name: i, type: int, mode: text, _: "0,1,2" }
      - { name: a, mode: python, _: "[1,2,1][$i]" }
      - { name: b, mode: python, _: "['x','y','z'][$i]" }
```

This keeps direct-sum alignment and outer-product expansion explicitly coordinated by `i`.

### `do` lowering

- emits one `step`.
- `depend` is comma-separated from `after`.
- prepends:
  - `set -euo pipefail`
  - `cd "${jube_benchmark_home}"`

### `submit` lowering

- emits synthetic submit parameterset with `init_with: "platform.xml:systemParameter"`.
- emits only submit keys explicitly set in the block.
- `preprocess` and `postprocess` are raw-block keys.
  - `preprocess` gets the standard preamble (`set -euo pipefail`, `cd "${jube_benchmark_home}"`).
  - `postprocess` is emitted as written.
- expression keys support scalar/container values and `shell("...")` / `python("...")`.
- emits submit step operations:
  - `${submit} --parsable ${submit_script} > run.jobid`
  - `echo "true" > success`

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
- `W101`: `+` length mismatch, cyclic broadcast applied.

## Known Limitations

- YAML emission only.
- no XML emission.
- no automatic `patternset` / `analyser` / `result` generation.
