# jbs help ls-analyse

`jbs ls-analyse <file.jbs|benchmark-dir>` lists analyse outputs from the latest
run directory without starting workpackages or running analyses.

```bash
jbs ls-analyse input.jbs
jbs ls-analyse bench
jbs ls-analyse -b small input.jbs
jbs ls-analyse -b small bench
```

In CSV mode, the command prints generated `analyse.csv` paths with row and column
counts. In SQLite mode, it prints `<database>:<table>` entries with row and
column counts.

When the input is a JBS file, the command parses the source to find the
benchmark output directory. When the input is a benchmark directory, the command
reads the latest persisted `manifest.json` directly and does not parse or
evaluate the source.

Use `-b` or `--benchmark` to list outputs for one configured benchmark
component. With directory input, this selects a matching component directory
below the benchmark root, or validates the selected name when the input already
points at one component directory.
If the latest run has no analyse outputs, the command succeeds without printing
a table.
