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
- `use "<path>.jbs" ...` always resolves a local filesystem path
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

When you import a `do`/`submit` step symbol, jbs also imports its required dependencies:

- transitive `after` steps
- referenced `with` sources
- referenced submit-header `use` let namespaces

This ensures one final YAML file has everything needed.

## Submit defaults from let namespace

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

- submit header supports only one `use` clause
- non-submit keys in defaults namespace are ignored with a warning
- explicit submit fields override imported defaults

## `jbs embed`

```bash
jbs embed
jbs embed jsc
```

- `jbs embed` prints all embedded shared files
- `jbs embed <filename>` prints embedded file content

## Errors and collisions

Import name collisions are hard errors, including:

- imported symbol vs local symbol
- imported symbol vs imported symbol
- transitive imported dependency collisions
- alias collisions
