# jbs help use

The `use` statement imports reusable definitions from embedded or local `.jbs` scripts.

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

## Resolution rules

- `use <module>`:
  - first resolves embedded `shared/<module>.jbs`
  - if missing, resolves local `./<module>.jbs` from the directory where `jbs` is called
- `use "<path>.jbs" ...` resolves relative to the importing `.jbs` file directory (or absolute if given)
- quoted paths must end with `.jbs`

## Importable symbols

You can import:

- `let`
- `param`
- `do`
- `submit`
- top-level global assignments (by variable name)

`analyse` is not importable by symbol name.

## Step imports and dependency closure

When you import a `do`/`submit` step symbol, JBS also imports its required dependencies:

- transitive `after` steps
- referenced `with` sources
- referenced submit-header `use` let namespaces

This ensures that the final YAML file contains everything required.

## Submit defaults from a let namespace

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
- non-submit variables in a defaults namespace are retained as internal helper parameters (`_jk__<step>_<name>`) in the generated submit parameter set
- submit values referencing those helper variables are rewritten to the helper alias form
- explicit submit fields override imported defaults
- if multiple namespaces set the same submit key or helper variable name, JBS uses last-win precedence and emits warning `W072`

## `jbs embed`

```bash
jbs embed
jbs embed jsc
```

- `jbs embed` prints all embedded shared files
- `jbs embed <filename>` prints an embedded file's content

## Errors and collisions

Import name collisions are hard errors, including:

- imported symbol vs local symbol
- imported symbol vs imported symbol
- transitive imported dependency collisions
- alias collisions
