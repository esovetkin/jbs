# jbs help tree

`jbs tree <file.jbs>` prints the planned `do` dependency tree and the number of
workpackages generated for each step.

```bash
jbs tree input.jbs
jbs tree --limit 1 input.jbs
jbs tree -b small input.jbs
jbs tree -b small --limit 1 input.jbs
```

The command evaluates the JBS script and builds the runtime plan, but it does not
create a run directory and does not start workpackages. The `#` column contains
the number of workpackages for each displayed step, and `total:` is the total
number of workpackages in the selected benchmark.

Use `-b` or `--benchmark` to inspect one configured benchmark component.
Use `-l <n>` or `--limit <n>` to print the dependency tree for only the first
`n` selected DAG branches, using the same branch selection rules as
`jbs run --limit`.

## Example

```bash
% cat example.jbs
p0 = t(a = range(6), b = ("a", "b", "c")) * t(c=("x","y"))

do step0
        with p0["a"]
{
        echo "a=${a}"
}

do step1
        after step0
        with p0["b","c"]
{
        echo "a=${a}"
        echo "b=${b}"
        echo "c=${c}"
}
% jbs tree example.jbs
| step          |  # |
|---------------|----|
| └── step0     |  6 |
|     └── step1 | 12 |
|---------------|----|
| total:        | 18 |
% jbs param example.jbs
| p0.a | p0.b | p0.c | step      |
|------|------|------|-----------|
| 0    |      |      | do: step0 |
| 1    |      |      | do: step0 |
| 2    |      |      | do: step0 |
| 3    |      |      | do: step0 |
| 4    |      |      | do: step0 |
| 5    |      |      | do: step0 |
| 0    | a    | x    | do: step1 |
| 0    | a    | y    | do: step1 |
| 1    | b    | x    | do: step1 |
| 1    | b    | y    | do: step1 |
| 2    | c    | x    | do: step1 |
| 2    | c    | y    | do: step1 |
| 3    | a    | x    | do: step1 |
| 3    | a    | y    | do: step1 |
| 4    | b    | x    | do: step1 |
| 4    | b    | y    | do: step1 |
| 5    | c    | x    | do: step1 |
| 5    | c    | y    | do: step1 |
```
