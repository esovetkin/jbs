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
