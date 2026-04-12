# jbs help use

The `use` statement imports reusable definitions from embedded or local `.jbs` scripts.

`use` is JBS-only syntax. During compilation, all imported content is merged into one generated JUBE YAML file.

## Syntax

```jbs
# Bare module import (embedded has priority over local ./<name>.jbs)
use <module>

# Path import with alias
use "<path>.jbs" as <alias>

# Selective symbol import
use <name> from <module>
use <name0>, <name1> from <module>
use <name> from "<path>.jbs"
```

After importing a module, `with` also supports namespace-qualified references:

```jbs
use test_lib

do s
        with x from test_lib.p
{
        echo ${x}
}
```

Resolution rules:
- `use <module>`:
  - first resolves embedded `shared/<module>.jbs`
  - if missing, resolves local `./<module>.jbs` from the directory where `jbs` is invoked
- `use "<path>.jbs" ...` resolves relative to the importing `.jbs` file directory (or absolute if given)
- quoted paths must end with `.jbs`

Importable symbols:
- `let`
- `param`
- `do`
- `submit`
- top-level global assignments (by variable name)

When you import a `do`/`submit` step symbol, JBS also imports its required dependencies:

- transitive `after` steps
- referenced `with` sources
- referenced submit-header `use` let namespaces

This ensures the final YAML file includes everything required.

`analyse` is not importable by symbol name.

Name collisions during import are hard errors, including:

- imported symbol vs local symbol
- imported symbol vs imported symbol
- transitive imported dependency collisions
- alias collisions

## Example: `submit` defaults from a let namespace

```jbs
use submit_defaults from jsc

submit run
        use submit_defaults
        with params
{
        nodes = "${nnodes}"   # explicit field overrides defaults
        args_exec = "-lc hostname"
}
```

Rules:

- submit headers can contain one or more `use` clauses
- non-submit variables from defaults namespaces are retained as internal helper parameters (`_jk__<step>_<name>`) in the generated submit parameter set
- submit values that reference those helper variables are rewritten to helper aliases
- explicit submit fields override imported defaults
- if multiple namespaces define the same submit key or helper variable name, JBS applies last-wins precedence and emits warning `W072`

## `jbs embed`

```bash
jbs embed
jbs embed jsc
```

- `jbs embed` prints all embedded shared files
- `jbs embed <filename>` prints the content of the embedded file
