# `jbs archive`

Pack the benchmark output directory for a JBS file:

```bash
jbs archive input.jbs
```

The archive is written in the current directory as `<input-stem>.tar.gz`. For example, `jbs archive sweep.jbs` writes `sweep.tar.gz`.

Archive entries are stored as:

```text
<timestamp>/<jbs_name>/<run_id>/...
```

Repeated archive calls preserve previous snapshots in the existing archive and add a new timestamp directory. A successful archive removes the original benchmark output directory after the archive file has been written atomically.

Archiving is rejected if any run in the benchmark directory is marked `RUNNING`. Dependency links are stored as symbolic links. External SQLite analyse databases are not included unless the database file is inside the benchmark output directory.
