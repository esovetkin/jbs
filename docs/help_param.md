# jbs help param

`jbs param <file.jbs>` prints the expanded step parameter table without creating
a run directory or starting workpackages.

```bash
jbs param input.jbs
jbs param -t csv input.jbs
jbs param -o params.txt input.jbs
```

Options:

- `-t pretty` or `--type pretty` prints the default aligned table.
- `-t csv` or `--type csv` prints CSV output.
- `-o <path>` or `--output <path>` writes output to a file.

The default output type is `pretty`, and the default output path is stdout.

