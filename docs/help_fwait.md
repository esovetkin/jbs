# `jbs fwait`

Wait until one of several files changes or appears:

```bash
jbs fwait results/done.flag other/done.flag
```

The command prints the path that caused it to exit.

If a file already exists, the command exits after the next observed change to that path. If it does not exist, the command exits once the file appears.

Use `-e` to exit immediately if any watched file already exists:

```bash
jbs fwait -e results/done.flag other/done.flag
```
