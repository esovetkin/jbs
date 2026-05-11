# jbs help repl

The REPL evaluates JBS chunks interactively.

Start it with:

```bash
jbs
jbs repl
```

## Expressions

Top-level expressions print their values:

```text
jbs> x = 1
jbs> x + 2
3
```

`names()` is useful for inspecting the current scope:

```text
jbs> names()
["jbs_database", "jbs_name", "jbs_nproc", ...]
```

Use `print(...)` when you want explicit output without an additional expression echo:

```text
jbs> print("x", [1, 2, 3, 4])
x [1, 2, 3, ...]
```

`shell(...)` also works in the REPL and runs during chunk evaluation:

```text
jbs> shell("printf hi")
hi
```

## Multi-Line Input

The REPL continues reading while braces, brackets, parentheses, strings, or trailing line continuations remain open.

```text
jbs> f = function(x) {
...   x + 1
... }
```

## Declarations

`do`, `analyse`, and `use` declarations are accepted at top level.

```text
jbs> cases = table(x = (1, 2))
jbs> do run with cases {
...   echo "${x}"
... }
```

Control-flow bodies can contain assignments and expressions, but declarations remain top-level only.

## Commands

```text
:help             show REPL help
?                 list internal functions with focused help
?<function_name>  show help for an internal function
:show             print accepted source
:reset            clear accepted source
:save <filename>  write accepted source to a file
:quit             exit
```

`:save` writes the accepted JBS source, the same content shown by `:show`.
