# jbs help submit

`submit` in JBS is a thin layer over JUBE's Slurm template flow:

1. JBS writes a synthetic JUBE `parameterset` with `init_with: "platform.xml:systemParameter"`.
2. JUBE applies `platform.xml:executesub` substitutions into `submit.job.in`.
3. The resulting `submit.job` is submitted with `sbatch`.

If you only care about `#SBATCH` lines and the launch command, jump to:
- "Lookup: submit keys -> #SBATCH headers"
- "Lookup: launch line replacement"

## `submit` syntax in JBS

```jbs
submit <name>
        [after <step0>, <step1>, ...]
        [with <paramset>, <var> from <paramset2>, ...]
{
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
        starter = "srun"
        args_starter = ""
        executable = "/bin/bash"
        args_exec = "-lc hostname"
        measurement = ""
        mail = "me@example.org"
        notification = "END,FAIL"

        # raw-block keys
        preprocess = {
                echo "before run"
        }
        postprocess = {
                echo "after run"
        }
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
- `errlogfile` -> `#STDERRLOGFILE#` (platform variable name)
- `queue` -> `#QUEUE#`
- `gres` -> `#GRES#`
- `account` -> `#ACCOUNT_CONFIG#` via:
  - `account_slurm = "#SBATCH --account=$account" if account is non-empty, else empty`

Other useful placeholders:
- `#BENCHNAME#` comes from JUBE internals:
  - `${jube_benchmark_name}_${jube_step_name}_${jube_wp_id}`

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

The final command is built by simple token replacement in that order.

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
        executable = "/bin/bash"
        args_exec = "-lc 'hostname'"

        preprocess = {
                module purge
                module load CUDA
        }

        postprocess = {
                echo "finished"
        }
}
```

## Example: Practical GPU submit block

```jbs
param cases
{
        nnodes = (1, 2)
        case = ("ddp", "fsdp")
        case + nnodes
}

submit train
        with cases
{
        account = "atmlaml"
        queue = "develbooster"
        nodes = nnodes
        tasks = nnodes
        threadspertask = 48
        gres = "gpu:4"
        timelimit = "00:15:00"
        outlogfile = "job.out"
        outerrfile = "job.err"

        starter = "srun"
        executable = "/bin/bash"
        args_exec = "-lc 'python -u train.py --case ${case}'"

        preprocess = {
                export NCCL_SOCKET_IFNAME=ib0
                export GLOO_SOCKET_IFNAME=ib0
        }
}
```
