# `jbs continue`

Resume the latest non-running run directory for a benchmark:

```bash
jbs continue input.jbs
jbs continue -b small input.jbs
```

Continue only works when the benchmark status is not `RUNNING` and the current source identity matches the run being resumed.

`jbs continue` also starts a run that was prepared with `jbs run --dry-run`, `jbs run -n`, or the `jbs -n input.jbs` shorthand. It can retry workpackages previously marked `ERROR`, `INTERRUPTED`, or `BLOCKED`; blocked work starts once its dependencies finish successfully.

When `jbs_benchmarks` is configured, `jbs continue input.jbs` resumes every configured component. Use `-b` or `--benchmark` to resume only one component. A selected component must exist in `jbs_benchmarks`.

Source identity includes every loaded `.jbs` source file's content and loader label, plus the contents of any `fsub` templates used by the selected benchmark. For file modules, the loader label is the cleaned absolute path used to load the file. It is not resolved through symbolic links.

Use the same path spelling for `jbs continue` that was used for `jbs run`. Continuing through a symlink or alternate absolute path can produce a different hash even when the source bytes are identical.

`jbs continue` resumes interrupted work when the benchmark is not already marked `RUNNING` and the source identity hash matches. The hash includes the contents and loader labels of all loaded `.jbs` files plus the contents of any `fsub` templates used by the selected benchmark. File labels are the cleaned absolute paths used by the loader, so continuing through a symlink or alternate absolute path can fail even if the file contents are identical.
