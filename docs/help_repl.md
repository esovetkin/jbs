# `jbs repl`

`jbs repl` starts an interactive shell for building and checking JBS snippets incrementally.

`jbs` with no arguments is equivalent to `jbs repl`.

## Prompt and multiline behavior

- Primary prompt: `jbs> `
- Continuation prompt: `...> `

Input is committed only when it is complete. Completion waits for:

- balanced `{}`
- closed single/double quotes
- no trailing backslash continuation

So multiline blocks work naturally:

```jbs
jbs> do run {
...>   echo hi
...> }
```

The same applies to multiline function literals:

```jbs
jbs> add = function(a, b = 1) {
...>   a + b
...> }
jbs> add
<function>
jbs> add(41)
42
```

Bare expression lines are part of normal top-level source. They are evaluated in the current module-aware session scope and then appended to the accepted session source.

You can inspect a global variable by entering its bare name:

```jbs
jbs> x = range(10)
jbs> x
[0, 1, 2, ...]
```

Namespace imports work the same way:

```jbs
jbs> use jsc
jbs> jsc.systemname
# prints the imported value of jsc.systemname
```

Imported functions behave the same way:

```jbs
jbs> use "./lib.jbs" as lib
jbs> lib.add(1, 2)
3
```

Returned closures also persist once bound into accepted session source:

```jbs
jbs> make_adder = function(delta) {
...>   function(x) {
...>     x + delta
...>   }
...> }
jbs> add2 = make_adder(2)
jbs> add2(3)
5
```

Top-level expression continuation is line-oriented, like file mode:

- `1 + \` followed by the next line continues the same expression chunk
- open `(` or `[` alone do not keep the prompt in continuation mode

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

- bare `use jsc`-style imports resolve embedded modules only
- `use "./lib.jbs" as lib` resolves relative to the REPL working directory (`<cwd>` where `jbs repl` was started)
- `use value from "./lib.jbs"` follows the same REPL-relative rule for the in-memory entry module
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
- assignments, imports, and blocks do not print values automatically
- bare expression line output is ignored by normal file compilation; only REPL prints it
- function values print as `<function>`
