# `jbs fwait`

Wait until one of several files changes or appears:

```bash
jbs fwait results/done.flag other/done.flag
```

The command prints the path that caused it to exit.

If a file already exists, the command exits after the next observed change to that path. If it does not exist, the command exits once the file appears.

`jbs fwait` uses filesystem notifications when available and also polls target metadata periodically. The polling path is required on network filesystems such as GPFS, where notifications from writes on other nodes may not be delivered to the waiting process.

Use `-e` to exit immediately if any watched file already exists:

```bash
jbs fwait -e results/done.flag other/done.flag
```
