# JBS

JBS is a small benchmark scripting tool. A `.jbs` file defines parameter data, executable `do` steps, optional `analyse` extraction blocks, and imports from other `.jbs` files.

The direct runner creates a benchmark directory, expands workpackages from `with` tables, runs step workpackages with dependency ordering, and writes per-workpackage stdout, stderr, status, and exit code files.

## Quick Start

```jbs
jbs_name = "taster"

cases = table(x = (1, 2), label = ("a", "b"))

do run with cases {
        echo "x=${x}" > out.txt
        echo "label=${label}" >> out.txt
}

analyse run {
        x = "x=(%d)" in "out.txt"
        label = "label=(%w)" in "out.txt"
        (x, label)
}
```

Run it:

```bash
jbs taster.jbs
# equivalent:
jbs run taster.jbs
```

Resume an interrupted benchmark:

```bash
jbs continue taster.jbs
```

Inspect parameter expansion without running jobs:

```bash
jbs printparam taster.jbs
jbs printparam -t csv -o params.csv taster.jbs
```

Validate or format a script:

```bash
jbs --check taster.jbs
jbs fmt taster.jbs
```

## Commands

```text
jbs <file.jbs>                 run a benchmark
jbs run <file.jbs>             run a benchmark
jbs continue <file.jbs>        resume an interrupted benchmark
jbs --check <file.jbs>         parse and validate only
jbs printparam [opts] <file>   print expanded step parameters
jbs fmt [-s|--strict] <file>   format a script in place
jbs help [topic]               show built-in help
jbs repl                       start the REPL
```

Help topics:

```bash
jbs help analyse
jbs help do
jbs help functions
jbs help globals
jbs help repl
jbs help use
```

## Language

Top-level assignments define scalar values, lists, tuples, tables, and functions. `do` blocks execute shell code once per workpackage. `analyse` blocks extract values from files created by a step and write `analyse.csv` for that step. `use` imports values or step declarations from another `.jbs` file.

`with` imports row-varying data into a step. Multiple sources are combined using the table algebra provided by functions such as `table`, `product`, `select`, and `zip`.

`after` declares step dependencies. A dependent step can inherit visible variables from predecessor steps. The runner respects the dependency tree and concurrency limits.

`nproc` limits concurrent workpackages:

```jbs
jbs_nproc = 8

do compile nproc 4 {
        make
}
```

`jbs_nproc = 0` and `do ... nproc 0` both mean "use the number of available CPUs".

See [docs/language.md](docs/language.md) for details.
