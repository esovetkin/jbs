# jbs help analyse

The `analyse` block maps parsed values and step-visible variables into a result table.

In JUBE terms, JBS lowers `analyse` into JUBE `analyser` and `result` sections. The overall workflow matches the JUBE tutorial example for creating a result table.

Reference:
- JUBE tutorial: creating a result table
  https://apps.fz-juelich.de/jsc/jube/docu/tutorial.html#creating-a-result-table

## What `analyse` does

1. Targets one existing `do` or `submit` step.
2. Declares extraction assignments from files produced by that step.
3. Defines output columns in a final tuple expression.
4. Writes output to `result/result_<step_name>.dat`.

## Syntax

```jbs
analyse <step_name>
        [with <let_ns>, <var> from <let_ns2>, ...]
{
        # helper assignment (compile-time expression binding)
        helper = <expr>

        # extraction assignment
        alias0 = <expr> in "<file>"
        alias1 = <expr> in "<file>"

        (<col0>, <col1> as "Custom Name", ...)
}
```

Rules:
- `<step_name>` must be a declared `do` or `submit` block.
- `with` in `analyse` can import only from `let` namespaces.
- Imported `let` variables in `analyse` must be strings.
- Extraction expressions must evaluate to a string.
- Left-hand extraction aliases become available in the final tuple.
- The final tuple is required and defines result columns.
- `as "..."` sets a custom column heading.
- Statements can be separated by newlines or `;`.
- Assignment operators in `analyse` statements: `=`, `+=`, `-=`, `*=`, `/=`, `%=`.

Compact one-line example:

```jbs
analyse write with p { n = number in "out"; w = word in "out"; (n, w); }
```

## Minimal end-to-end example

```jbs
param p
{
        number = (1, 2, 4)
        number
}

do write_number
        with p
{
        echo "Number: ${number}" > en
        echo "Zahl: ${number}" > de
}

let pat
{
        number_en = "Number: %d"
        number_de = "Zahl: %d"
}

analyse write_number
        with pat
{
        en = number_en in "en"
        de = number_de in "de"

        (number, en as "Number", de as "Zahl")
}
```

This follows the same idea as the JUBE tutorial:
- produce files in a step
- parse files in an analyser
- define visible output columns in the result

## Example with multiple parsed values

```jbs
param cfg
{
        case = ("ddp", "fsdp")
        case
}

do run
        with cfg
{
        echo "Runtime: 12.7" > job.out
        echo "Final loss: 0.031" >> job.out
        echo "Case: ${case}" > meta.out
}

let p
{
        runtime = "Runtime: %f"
        loss = "Final loss: %f"
        case_name = "Case: %w"
}

analyse run
        with p
{
        rt = runtime in "job.out"
        ls = loss in "job.out"
        cs = case_name in "meta.out"

        (case, cs as "parsed_case", rt as "runtime_s", ls as "final_loss")
}
```

## Example with `submit`

`analyse` can target a `submit` step in exactly the same way.

```jbs
submit bench
{
        executable = "/bin/bash"
        args_exec = "-lc 'echo Runtime: 3.5 > job.out'"
}

let p
{
        runtime = "Runtime: %f"
}

analyse bench
        with p
{
        t = runtime in "job.out"
        (t as "runtime_seconds")
}
```

## How column naming works

```jbs
analyse write
        with p
{
        p0 = number_en in "en"
        p1 = number_de in "de"

        (number, p0, p1 as "German")
}
```

- `number` uses the column title `number`
- `p0` uses the column title `p0`
- `p1 as "German"` uses the column title `German`
