# jbs help submit

`submit` defines scheduler-facing execution fields and lowers to JUBE submit templates.

## Syntax

```jbs
submit <name>
        [after <step0>, <step1>, ...]
        [with <source>, <var> from <source2>, ...]
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

```
        # expression keys
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

        # raw block
        preprocess = {
                echo "before run"
        }

        measurement = ""
        starter = "srun"
        args_starter = ""
        executable = "/bin/bash"
        args_exec = "-lc hostname"

        # raw block
        postprocess = {
                echo "after run"
 ```


- `account`, `queue`, `nodes`, `tasks`, `threadspertask`, `timelimit`
- `outlogfile`, `outerrfile`, `gres`, `mail`, `notification`
- `measurement`, `starter`, `args_starter`, `executable`, `args_exec`
- `preprocess`, `postprocess`

## Notes

- Variables are visible only through `with` imports (plus inherited vars from `after`).
- `submit ... use ...` merges helper defaults with last-win policy.
- Missing/empty key warnings are emitted for important scheduler fields.

## Example

```jbs
nnodes = (1, 2)
case = ("ddp", "fsdp")
cases = comb(case + nnodes)

submit train
        with cases
{
        account = "myacct"
        queue = "batch"
        nodes = "${nnodes}"
        executable = "/bin/bash"
        args_exec = "-lc 'echo case=${case} nodes=${nnodes}'"
}
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
- `outerrfile` -> `#STDERRLOGFILE#` (platform variable name)
- `queue` -> `#QUEUE#`
- `gres` -> `#GRES#`
- `account` -> `#ACCOUNT_CONFIG#` via:
  - `account_slurm = "#SBATCH --account=$account"` if `account` is non-empty; otherwise empty

Other useful placeholders:
- `#BENCHNAME#` comes from JUBE internals:
  - `${jube_benchmark_name}_${jube_step_name}_${jube_wp_id}`
- `$jube_benchmark_home`, `$jube_wp_abspath`, and other [JUBE variables](https://apps.fz-juelich.de/jsc/jube/docu/glossar.html#term-jube_variables)

## Lookup: launch line replacement

In `submit.job.in`, the runtime line is:

```bash
#MEASUREMENT# #STARTER# #ARGS_STARTER# #EXECUTABLE# #ARGS_EXECUTABLE#
```

From `platform.xml:executesub`:

- `measurement` -> `#MEASUREMENT#`
- `starter` -> `#STARTER#`
- `args_starter` -> `#ARGS_STARTER#`
- `executable` -> `#EXECUTABLE#`
- `args_exec` -> `#ARGS_EXECUTABLE#`

The final command is built by token replacement in that order.

Example:

```jbs
submit run
{
        measurement = "time -p"
        starter = "srun"
        args_starter = ""
        executable = "/bin/bash"
        args_exec = "-lc 'python train.py --epochs 1'"
}
```

Resulting launch line in the generated `submit.job`:

```bash
time -p srun --mpi=pmix /bin/bash -lc 'python train.py --epochs 1'
```

## `preprocess` and `postprocess`

`preprocess` and `postprocess` are raw blocks inserted as-is into the template:

- `preprocess` -> `#PREPROCESS#`
- `postprocess` -> `#POSTPROCESS#`

Example:

```jbs
submit run
{
        preprocess = {
                module purge
                module load CUDA
        }

        executable = "/bin/bash"
        args_exec = "-lc 'hostname'"

        postprocess = {
                echo "finished"
        }
}
```

## Example: practical GPU submit block

XXX
