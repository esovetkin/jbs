# `jbs archive`

Pack the benchmark output directory for a JBS file or an existing benchmark
directory:

```bash
jbs archive input.jbs
jbs archive bench
```

For JBS-file input, the archive is written in the current directory as
`<input-stem>.tar.gz`. For example, `jbs archive sweep.jbs` writes
`sweep.tar.gz`.

For benchmark-directory input, the archive name is derived from the directory
name. For example, `jbs archive bench_out` writes `bench_out.tar.gz`. To archive
one component from a `jbs_benchmarks` output tree, pass that component directory
directly, for example `jbs archive bench/small`.

Directory input does not parse or evaluate the source file.

Archive entries are stored as:

```text
<timestamp>/<jbs_name>/<run_id>/...
```

For `jbs_benchmarks` component output, archive entries preserve the component directories:

```text
<timestamp>/<jbs_name>/<component>/<run_id>/...
```

Repeated archive calls preserve previous snapshots in the existing archive and add a new timestamp directory. A successful archive removes the original benchmark output directory after the archive file has been written atomically.

Archiving is rejected if any run in the benchmark directory, including component run directories, is marked `RUNNING`. Dependency links are stored as symbolic links. External SQLite analyse databases are not included unless the database file is inside the benchmark output directory.
