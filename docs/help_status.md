# jbs help status

`jbs status <file.jbs|benchmark-dir>` prints the status overview for the latest
run directory without starting workpackages or running analyses.

```bash
jbs status input.jbs
jbs status bench
jbs status -b small input.jbs
jbs status -b small bench
```

The table groups workpackage counts by the `do` step dependency tree and includes
`FINISHED`, `ERROR`, `BLOCKED`, `NOTSTARTED`, `RUNNING`, and `INTERRUPTED`.
If any workpackage has status `ERROR`, `jbs status` also prints the corresponding
absolute workpackage directory paths after the overview table.

When the input is a JBS file, the command parses the source to find the
benchmark output directory. When the input is a benchmark directory, the command
reads the latest persisted `manifest.json` directly and does not parse or
evaluate the source.

Use `-b` or `--benchmark` to inspect one configured benchmark component. With
directory input, this selects a matching component directory below the benchmark
root, or validates the selected name when the input already points at one
component directory.
