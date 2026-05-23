# jbs help param

`jbs param <file.jbs>` prints the expanded step parameter table without creating
a run directory or starting workpackages.

```bash
jbs param input.jbs
jbs param -b small input.jbs
jbs param --limit 1 input.jbs
jbs param -b small --limit 1 input.jbs
jbs param -t csv input.jbs
jbs param -o params.txt input.jbs
```

Options:

- `-t pretty` or `--type pretty` prints the default aligned table.
- `-t csv` or `--type csv` prints CSV output.
- `-o <path>` or `--output <path>` writes output to a file.
- `-l <n>` or `--limit <n>` prints parameters only for the workpackages that would be created by `jbs run --limit <n>`.
- `-b <name>` or `--benchmark <name>` prints parameters for one configured benchmark component.

The default output type is `pretty`, and the default output path is stdout.

When `jbs_benchmarks` is configured and no benchmark is selected, `jbs param`
prints rows for all configured components and includes a `benchmark` column.
Selection with `-b` uses the same target and dependency filtering as `jbs run`
and `jbs tree`.

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
% jbs tree example.jbs
| step          |  # |
|---------------|----|
| └── step0     |  6 |
|     └── step1 | 12 |
|---------------|----|
| total:        | 18 |
```
