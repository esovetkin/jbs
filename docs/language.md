# JBS Language (V1)

## Grammar

```ebnf
program       := stmt* EOF
stmt          := param_block | do_block | submit_block

param_block   := "param" IDENT with_clause? "{" param_stmt* final_expr "}"
param_stmt    := IDENT "=" expr NEWLINE
final_expr    := comb_expr NEWLINE

with_clause   := "with" with_item ("," with_item)*
with_item     := IDENT ("from" IDENT)?

do_block      := "do" IDENT after_clause? with_clause? raw_block
submit_block  := "submit" IDENT after_clause? with_clause? raw_block raw_block

after_clause  := "after" IDENT ("," IDENT)*
raw_block     := "{" RAW_TEXT "}"
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

In `param`:

- imported names initialize local scope.
- assignment to imported name is local rebinding (copy-on-write).

In `do`/`submit`:

- `with p2` uses whole parameter set.
- `with x from p2` generates a synthetic subset parameterset containing only selected variables.
- In mixed form (`with x from p2, p3`), `p3` is treated as whole-parameterset import.

## Lowering to JUBE YAML

### `param` lowering modes

1. Pure outer product (`*` only): template parameters with fixed separator `####`.
2. Any direct sum (`+` present): grouped/indexed representation:

```yaml
parameterset:
  - name: grouped
    parameter:
      - { name: i, type: int, mode: text, _: "0,1,2" }
      - { name: a, mode: python, _: "[1,2,1][$i]" }
      - { name: b, mode: python, _: "['x','y','z'][$i]" }
```

This preserves direct-sum row alignment under JUBE semantics.

### `do` lowering

- emits one `step`.
- `depend` is comma-separated from `after`.
- prepends:
  - `set -euo pipefail`
  - `cd "${jube_benchmark_home}"`

### `submit` lowering

- emits synthetic submit parameterset with `init_with: "platform.xml:systemParameter"`.
- maps built-in `jbs_*` globals to submit parameters (`queue`, `account`, `nodes`, ...).
- raw blocks map to:
  - first block: `env`
  - second block: `args_exec`
- emits submit step operations:
  - `${submit} --parsable ${submit_script} > run.jobid`
  - `echo "true" > success`

## Built-in Globals

- `jbs_systemname`
- `jbs_queue`
- `jbs_account`
- `jbs_timelimit`
- `jbs_outlogfile`
- `jbs_outerrfile`
- `jbs_gres`
- `jbs_threadspertask`
- `jbs_nnodes`
- `jbs_tasks`
- `jbs_executable`

Run `jbs` without arguments to print defaults and mapping.

## Diagnostics

All diagnostics include source location (`file:line:column`).

Key codes:

- `E020`: unknown imported parameterset.
- `E021`: unknown imported variable.
- `E036`: repeated identifier in combination expression.
- `E042`: conflicting key values during row merge.
- `E053`: reserved separator `####` appears in value.
- `E071`: invalid `submit` block arity (must be two raw blocks).
- `W101`: `+` length mismatch, cyclic broadcast applied.

## Known Limitations

- YAML emission only.
- no XML emission.
- no automatic `patternset` / `analyser` / `result` generation.
