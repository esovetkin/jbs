# `jbs continue`

Resume the latest non-running run directory for a benchmark:

```bash
jbs continue input.jbs
```

Continue only works when the benchmark status is not `RUNNING` and the current source identity matches the run being resumed.

`jbs continue` also starts a run that was prepared with `jbs run --dry-run`, `jbs run -n`, or the `jbs -n input.jbs` shorthand.

Source identity includes every loaded `.jbs` source file's content and loader label. For file modules, the loader label is the cleaned absolute path used to load the file. It is not resolved through symbolic links.

Use the same path spelling for `jbs continue` that was used for `jbs run`. Continuing through a symlink or alternate absolute path can produce a different hash even when the source bytes are identical.
