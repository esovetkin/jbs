# jbs help analyse

The `analyse` block targets an existing `do` or `submit` step and maps matched patterns plus step-visible variables into a result table. The resulting table is saved as `result/result_<step_name>.dat`.

In JUBE terms, JBS lowers `analyse` into JUBE `patternset`, `analyser`, and `result` sections.

## Syntax

```jbs
analyse <step_name>
        [with <scalar0>, <scalar1>, ...]
{
        helper = <expr>

        p0 = <pattern_expr> in "<file>"
        p1 = <pattern_expr> in "<file>"

        (p0, p1 as "Title", ...)
}
```

Rules:

- `<step_name>` must be a declared `do` or `submit` block
- helper assignments omit `in "<file>"`
- extraction expressions must evaluate to strings
- extraction variables become available in the final tuple
- the final tuple is required and defines result-table columns
- `as "..."` sets a custom column heading
- `%d`, `%f`, `%w`, and `%%` are supported in extraction strings
- `with` in `analyse` is scalar-only and requires string data bindings

## Example

```jbs
a = ("a",) * 3
i = [1,2,3]
x = i / 2
cases = table(a = a, i = i, x = x)
pat_number = "Number: %d"

do s
        with cases
{
    echo "Word: ${a}" > en
    echo "Number: ${i}" >> en
    echo "Zahl: ${x}" > de
}

analyse s
        with pat_number
{
        n = pat_number in "en"
        (a, x, i, n as "parsed_number")
}
```

In that example:

- `a`, `x`, and `i` come from the target step visibility
- `pat_number` is imported explicitly through `analyse with ...`
- `n` is an extracted result value from file `en`
