# jbs help ls-analyse

`jbs ls-analyse <file.jbs>` lists analyse outputs from the latest run directory
without starting workpackages or running analyses.

```bash
jbs ls-analyse input.jbs
jbs ls-analyse -b small input.jbs
```

In CSV mode, the command prints generated `analyse.csv` paths with row and column
counts. In SQLite mode, it prints `<database>:<table>` entries with row and
column counts.

Use `-b` or `--benchmark` to list outputs for one configured benchmark component.
If the latest run has no analyse outputs, the command succeeds without printing
a table.

