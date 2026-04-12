# jbs help submit

`submit` in JBS is a thin layer over JUBE's Slurm template flow:

1. JBS writes a synthetic JUBE `parameterset` with `init_with: "platform.xml:systemParameter"`.
2. JUBE applies `platform.xml:executesub` substitutions to `submit.job.in`.
3. The resulting `submit.job` is submitted with `sbatch`.

If you only care about `#SBATCH` lines and the launch command, jump to:
- "Lookup: submit keys -> #SBATCH headers"
- "Lookup: launch line replacement"

## `submit` syntax in JBS

```jbs
submit <name>
        [after <step0>, <step1>, ...]
        [use <let_namespace0>, <let_namespace1>, ...]
        [with <paramset>, <var> from <paramset2>, ...]
        [<key>=<int> ...]
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
        }
}
```

Step options use generic `key=value` syntax. Allowed keys are:

- `max_async` must be an integer `>= 0`
- `procs` must be an integer `>= 0`
- `iterations` must be an integer `>= 1`

Example:

```jbs
submit run
        with p
        max_async=3 procs=2 iterations=2
{
        args_exec = "-lc hostname"
}
```

`submit ... use ...` is restricted to `let` namespaces. Using a `param` source in submit-header `use` is an error.

Multiple `use` clauses are allowed and merged in order:

```jbs
submit run
        use defaults
        use gpu_defaults
{
        args_exec = "-lc hostname"
}
```

This is equivalent to:

```jbs
submit run
        use defaults, gpu_defaults
{
        args_exec = "-lc hostname"
}
```

Defaults follow last-win precedence by key across `use` namespaces. If the same key (or helper variable name) is provided by multiple `use` namespaces, JBS emits warning `W072`.

Variables in submit-header `use` namespaces that are not submit keys are lowered as internal helper parameters (`_jk__<step>_<name>`). References to those variables inside submit values are rewritten to the helper aliases.

## Submit Expression Precedence

When JBS evaluates expression-valued submit fields (for example `nodes = mynodes`), identifier lookup follows this order:

1. globals
2. effective `with` imports
3. submit-header `use` variables in declaration order (last `use` wins)

Short form: `globals < with-imports < submit-header use`.

`use` can override names imported via `with` for submit expression evaluation.

```jbs
let d0 {
        mynodes = 1
}

let d1 {
        mynodes = 4
}

submit run
        with d0
        use d1
{
        nodes = mynodes
        args_exec = "-lc hostname"
}
```

In this example, `nodes` resolves to `4` from `d1`.

Explicit submit field assignments are not available as identifiers to later submit field expressions in the same block.

Inside `submit`, key assignments can be separated by a newline or `;`:

```jbs
submit run { queue = "batch"; account = "myacct"; args_exec = "-lc hostname"; }
```

Expression-valued submit keys support assignment operators:

- `=`
- `+=`, `-=`, `*=`, `/=`, `%=`

Rewrite rules follow normal desugaring, for example:

- `args_exec += " --flag"` -> `args_exec = args_exec + " --flag"`

Raw submit keys (`preprocess`, `postprocess`) must use `= { ... }`.

## Variable inheritance with `after`

`after` also carries variable visibility from predecessor steps.

- If `submit run` has `after prep`, variables visible in `prep` are inherited by `run`.
- If `run` also has `with ... from <same_paramset>`, JBS imports only variables that are not already inherited.
- If an inherited variable name collides with a variable of the same name from a different parameter set in explicit `with`, JBS raises an error.
- JBS preserves source-row constraints from inherited imports, so dependent `submit` steps do not introduce unintended Cartesian blow-ups.
- This source-row inheritance is transitive across multi-step chains.

Example:

```jbs
param pm0
{
        a = (1, 2)
        b = ("x", "y")
        c = (true, false)
        a * b * c
}

do prep
        with (a, b) from pm0
{
        echo "${a} ${b}" > vars.txt
}

submit run
        after prep
        with (b, c) from pm0
{
        executable = "/bin/bash"
        args_exec = "-lc 'echo ${a} ${b} ${c}'"
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

```jbs
use submit_defaults from jsc

param cases
{
        nnodes = (1, 2)
        case = ("ddp", "fsdp")
        case + nnodes
}

submit train
        use submit_defaults
        with cases
{
        account = "atmlaml"
        nodes = "${nnodes}"
        timelimit = "00:15:00"

        preprocess = {
                export NCCL_SOCKET_IFNAME=ib0
                export GLOO_SOCKET_IFNAME=ib0
        }

        executable = "/bin/bash"
        args_exec = "-lc 'python -u train.py --case ${case}'"
}
```
