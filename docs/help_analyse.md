# jbs help analyse

The `analyse` block maps parsed values (from `patterns`) and step-visible variables into a result table.

In JUBE terms, jbs lowers `analyse` into JUBE `analyser` and `result` sections. The overall workflow matches the JUBE tutorial example for creating a result table.

Reference:
- JUBE tutorial: creating a result table
  https://apps.fz-juelich.de/jsc/jube/docu/tutorial.html#creating-a-result-table

## What `analyse` does

1. Targets one existing `do` or `submit` step.
2. Declares pattern extractions from files produced by that step.
3. Defines output columns in a final tuple expression.

## Syntax

```jbs
analyse <step_name>
{
        <alias0> = <pattern_group>.<pattern_name> in "<file>"
        <alias1> = <pattern_group>.<pattern_name> in "<file>"
        ...

        (<col0>, <col1> as "Custom Name", ...)
}
```

Rules:
- `<step_name>` must be a declared `do` or `submit` block.
- Left-hand aliases (`<alias0>`, `<alias1>`) are local names for parsed values.
- The final tuple is required and defines result columns.
- `as "..."` sets a custom column heading.

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

patterns pat
{
        number_en = "Number: %d"
        number_de = "Zahl: %d"
}

analyse write_number
{
        en = pat.number_en in "en"
        de = pat.number_de in "de"

        (number, en as "Number", de as "Zahl")
}
```

This corresponds to the same idea as the JUBE tutorial:
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

patterns p
{
        runtime = "Runtime: %f"
        loss = "Final loss: %f"
        case_name = "Case: %w"
}

analyse run
{
        rt = p.runtime in "job.out"
        ls = p.loss in "job.out"
        cs = p.case_name in "meta.out"

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

patterns p
{
        runtime = "Runtime: %f"
}

analyse bench
{
        t = p.runtime in "job.out"
        (t as "runtime_seconds")
}
```

## How column naming works

```jbs
analyse write
{
        p0 = p.number_en in "en"
        p1 = p.number_de in "de"

        (number, p0, p1 as "German")
}
```

- `number` column title is `number`
- `p0` column title is `p0`
- `p1 as "German"` column title is `German`
