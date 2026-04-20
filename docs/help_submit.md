# jbs help submit

`submit` defines scheduler-facing execution fields and lowers to JUBE submit templates.

## Syntax

```jbs
submit <name>
        [after <step0>, <step1>, ...]
        [with <source>, <source2>[<col0>, <col1>, ...], ...]
        [use <name0>, <name1>, ...]
        [<key>=<int> ...]
{
        <field> = <expr>
        <raw_field> = {
                # raw shell block
        }
}
```

Allowed step header keys:

- `max_async` (>= 0)
- `procs` (>= 0)
- `iterations` (>= 1)

## Common submit fields

```jbs
        account = "myacct"
        queue = "batch"
        nodes = 2
        tasks = 8
        threadspertask = 6
        timelimit = "00:15:00"
        outlogfile = "job.out"
        outerrfile = "job.err"
        gres = "gpu:4"
        mail = "me@example.org"
        notification = "END,FAIL"

        preprocess = {
                echo "before run"
        }

        measurement = ""
        starter = "srun"
        args_starter = ""
        executable = "/bin/bash"
        args_exec = "-lc hostname"

        postprocess = {
                echo "after run"
        }
```

## Notes

- `with` imports row-varying data used by the submit body
- `after` still carries predecessor-visible variables into dependent submit steps; there is no separate `inherit` clause
- `submit ... use ...` imports scalar defaults from a scalar global or from a module namespace
- later `use` sources win on collisions and emit warning `W072`
- raw submit keys are `preprocess` and `postprocess`
- missing or empty important scheduler keys emit warnings

## Example

```jbs
nnodes = (1, 2)
case = ("ddp", "fsdp")
cases = table(case = case, nnodes = nnodes)

use "./submit_defaults.jbs" as defaults

submit train
        use defaults
        with cases
{
        nodes = "${nnodes}"
        executable = "/bin/bash"
        args_exec = "-lc 'echo case=${case} nodes=${nnodes}'"
}
```

`submit_defaults.jbs` can export scalar defaults such as:

```jbs
account = "myacct"
queue = "batch"
starter = "srun"
```

## Lookup: submit keys -> `#SBATCH` headers

From `submit.job.in`:

```bash
#SBATCH --mail-user=#NOTIFY_EMAIL#
#SBATCH --mail-type=#NOTIFICATION_TYPE#
#SBATCH --nodes=#NODES#
#SBATCH --ntasks=#TASKS#
#SBATCH --cpus-per-task=#NTHREADS#
#SBATCH --time=#TIME_LIMIT#
#SBATCH --output=#STDOUTLOGFILE#
#SBATCH --error=#STDERRLOGFILE#
#SBATCH --partition=#QUEUE#
#SBATCH --gres=#GRES#
#ACCOUNT_CONFIG#
```

From `platform.xml:executesub`, the replacements are:

- `mail` -> `#NOTIFY_EMAIL#`
- `notification` -> `#NOTIFICATION_TYPE#`
- `nodes` -> `#NODES#`
- `tasks` -> `#TASKS#`
- `threadspertask` -> `#NTHREADS#`
- `timelimit` -> `#TIME_LIMIT#`
- `outlogfile` -> `#STDOUTLOGFILE#`
- `outerrfile` -> `#STDERRLOGFILE#`
- `queue` -> `#QUEUE#`
- `gres` -> `#GRES#`
- `account` -> `#ACCOUNT_CONFIG#`

## Lookup: launch line replacement

In `submit.job.in`, the runtime line is:

```bash
#MEASUREMENT# #STARTER# #ARGS_STARTER# #EXECUTABLE# #ARGS_EXECUTABLE#
```

The replacements are:

- `measurement` -> `#MEASUREMENT#`
- `starter` -> `#STARTER#`
- `args_starter` -> `#ARGS_STARTER#`
- `executable` -> `#EXECUTABLE#`
- `args_exec` -> `#ARGS_EXECUTABLE#`
