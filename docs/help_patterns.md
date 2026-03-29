# jbs help patterns

The `patterns` block defines named extraction rules (regular expressions) that are later used by `analyse`.

In JUBE terms, each pattern is a `pattern_tag` entry used during analyser/result creation.

References:
- JUBE glossary (`pattern_tag`): https://apps.fz-juelich.de/jsc/jube/docu/glossar.html#term-pattern_tag
- JUBE tutorial (creating a result table): https://apps.fz-juelich.de/jsc/jube/docu/tutorial.html#creating-a-result-table

## Syntax

```jbs
patterns <group_name>
{
        <pattern_name> = "<regex>"
        ...
}
```

You use these patterns from `analyse` via `<group_name>.<pattern_name>`.

```jbs
analyse <step_name>
{
        p0 = <group_name>.<pattern_name> in "<file>"

        (p0)
}
```

## Placeholders in jbs patterns

jbs supports shortcuts that map to predefined JUBE pattern variables:

- `%d` -> `$jube_pat_int` (integer)
- `%f` -> `$jube_pat_fp` (floating point)
- `%w` -> `$jube_pat_wrd` (word)
- `%%` -> literal `%`

This keeps pattern definitions compact while still lowering to standard JUBE patterns.

## Example 1: Extract integer and word

```jbs
param p
{
        x = (1, 2)
        name = ("alpha", "beta")
        x + name
}

do write
        with p
{
        echo "Number: ${x}"  > out.txt
        echo "Name: ${name}" >> out.txt
}

patterns basic
{
        number = "Number: %d"
        who = "Name: %w"
}

analyse write
{
        n = basic.number in "out.txt"
        w = basic.who in "out.txt"

        (x, name, n as "parsed_number", w as "parsed_name")
}
```

What this does:
- `patterns basic` defines two extractors.
- `analyse write` applies them to `out.txt`.
- The final tuple defines result columns.

## Example 2: Multiple files, different patterns

```jbs
param cases
{
        id = (1, 2, 3)
        id
}

do produce
        with cases
{
        echo "EN value: ${id}" > en.log
        echo "DE wert: ${id}" > de.log
}

patterns p
{
        en_value = "EN value: %d"
        de_wert = "DE wert: %d"
}

analyse produce
{
        en = p.en_value in "en.log"
        de = p.de_wert in "de.log"

        (id, en as "english", de as "german")
}
```

## Example 3: Floating-point parsing

```jbs
do bench
{
        echo "Runtime: 12.73" > run.log
}

patterns perf
{
        runtime = "Runtime: %f"
}

analyse bench
{
        t = perf.runtime in "run.log"
        (t as "runtime_seconds")
}
```
