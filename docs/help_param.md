# jbs help param

`jbs param <file.jbs>` prints the expanded step parameter table without creating
a run directory or starting workpackages.

```bash
jbs param input.jbs
jbs param -b small input.jbs
jbs param -t csv input.jbs
jbs param -o params.txt input.jbs
```

Options:

- `-t pretty` or `--type pretty` prints the default aligned table.
- `-t csv` or `--type csv` prints CSV output.
- `-o <path>` or `--output <path>` writes output to a file.
- `-b <name>` or `--benchmark <name>` prints parameters for one configured benchmark component.

The default output type is `pretty`, and the default output path is stdout.

When `jbs_benchmarks` is configured and no benchmark is selected, `jbs param`
prints rows for all configured components and includes a `benchmark` column.
Selection with `-b` uses the same target and dependency filtering as `jbs run`
and `jbs tree`.
