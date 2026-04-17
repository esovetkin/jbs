# `jbs repl`

`jbs repl` starts an interactive shell for building and checking JBS snippets incrementally.

`jbs` with no arguments is equivalent to `jbs repl`.

## Prompt and multiline behavior

- Primary prompt: `jbs> `
- Continuation prompt: `...> `

Input is evaluated only when it is complete. Completion waits for:

- balanced `{}`, `()`, and `[]`
- closed single/double quotes
- no trailing backslash continuation

So multiline blocks work naturally:

```jbs
jbs> do run {
...>   echo hi
...> }
```

You can inspect a global variable by entering its bare name:

```jbs
jbs> a
[0, 1, 2, ...]
```

You can also evaluate a standalone expression directly:

```jbs
jbs> range(10)
[0, 1, 2, ...]
```

Standalone expression evaluation prints the result but does not append anything to the accepted session source.  
Assignments and blocks still go through the normal commit path.

## Using `use` in REPL

REPL supports the same `use` import flow as file mode.

```jbs
jbs> use submit_defaults from jsc
jbs> queue = "batch"
jbs> submit run
...>         use submit_defaults
...> {
...>         account = "myacct"
...>         executable = "/bin/bash"
...>         args_exec = "-lc hostname"
...> }
```

Path resolution:

- `use "./lib.jbs" as lib` resolves relative to the REPL working directory (`<cwd>` where `jbs repl` was started)
- nested quoted imports resolve relative to the importer module path

## Key bindings

- `Ctrl+R`: reverse history search (readline default)
- `Ctrl+C`: cancel current pending multiline input and return to prompt
- `Ctrl+D`: exit REPL

History is stored locally in `<cwd>/.jbs_history`, where `<cwd>` is the directory from which `jbs` was started.

## REPL commands

- `:help`: show command help
- `:show`: print current accepted session source
- `:check`: run parser/sema checks on accepted source
- `:yaml`: print lowered YAML for accepted source
- `:save <filename>`: write lowered YAML to file
- `:reset`: clear accepted source and pending input
- `:quit` or `:exit`: exit REPL

Notes:

- `:yaml` and `:save` use the same lowering path.
- `:save` writes atomically (temporary file + rename).
- `:save` paths are resolved relative to the REPL process working directory when given as relative paths.
- REPL output prints errors only; warnings are suppressed in interactive mode.
