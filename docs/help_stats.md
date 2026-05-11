# jbs help stats

`jbs stats <file.jbs>` prints the status overview for the latest run directory
without starting workpackages or running analyses.

```bash
jbs stats input.jbs
jbs stats -b small input.jbs
```

The table groups workpackage counts by the `do` step dependency tree and includes
`FINISHED`, `ERROR`, `BLOCKED`, `NOTSTARTED`, `RUNNING`, and `INTERRUPTED`.
If any workpackage has status `ERROR`, `jbs stats` also prints the corresponding
workpackage directory paths after the overview table.

Use `-b` or `--benchmark` to inspect one configured benchmark component.
