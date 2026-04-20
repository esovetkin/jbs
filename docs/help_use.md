# jbs help use

The `use` statement imports reusable definitions from embedded or local `.jbs` scripts.

`use` is JBS-only syntax. During compilation, imported modules are resolved and analyzed together with the entry file before one YAML document is produced.

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
        with x from test_lib.cases
{
        echo ${x}
}
```

You can also use comb-style slicing on imported table symbols:

```jbs
use test_lib

do s0
        with test_lib.cases[x, y]
{
        echo ${x} ${y}
}
```

Resolution rules:

- `use <module>`:
  - first resolves embedded `shared/<module>.jbs`
  - if missing, resolves local `./<module>.jbs` from the directory where `jbs` is invoked
- `use "<path>.jbs" ...` resolves relative to the importing `.jbs` file directory, or absolute if given
- quoted paths must end with `.jbs`

Importable symbols:

- top-level global assignments, including function-valued globals
- `do` steps
- `submit` steps
- module namespaces created by namespace imports

When you import a `do` or `submit` symbol, JBS also imports its required dependencies:

- transitive `after` steps
- referenced `with` sources
- referenced submit-header `use` sources

`analyse` is not importable by symbol name.

Name collisions during import are hard errors, including:

- imported symbol vs local symbol
- imported symbol vs imported symbol
- transitive imported dependency collisions
- alias collisions

## Example: submit defaults from a module namespace

```jbs
use "./submit_defaults.jbs" as defaults

submit run
        use defaults
{
        executable = "/bin/bash"
        args_exec = "-lc hostname"
}
```

`submit_defaults.jbs` can export scalar defaults such as:

```jbs
account = "myacct"
queue = "batch"
starter = "srun"
```

Rules:

- submit headers can contain one or more `use` clauses
- non-submit variables from defaults namespaces are retained as internal helper parameters in the generated submit parameter set
- submit values that reference those helper variables are rewritten to helper aliases
- explicit submit fields override imported defaults
- if multiple sources define the same submit key or helper variable name, JBS applies last-wins precedence and emits warning `W072`

## `jbs embed`

```bash
jbs embed
jbs embed jsc
```

- `jbs embed` prints all embedded shared files
- `jbs embed <filename>` prints the content of the embedded file
