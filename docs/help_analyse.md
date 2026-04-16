# jbs help analyse

The `analyse` block targets an existing `do` or `submit` step and maps matched patterns and step-visible variables into a result table. The resulting table is saved as `result/result_<step_name>.dat`.

In JUBE terms, JBS lowers `analyse` into JUBE `patternset`, `analyser` and `result` sections. The overall workflow matches the [JUBE tutorial example](https://apps.fz-juelich.de/jsc/jube/docu/tutorial.html#creating-a-result-table) for creating a result table.

## Syntax

```jbs
analyse <step_name>
        [with <scalar0>, <scalar1>, ...]
{
        # helper assignment (compile-time expression binding)
        helper = <expr>

        p0 = <pattern_expr> in "<file>"
        p1 = <pattern_expr> in "<file>"
        ...

        (p0, p1 as "Title",  ...)
}
```

- `<step_name>` must be a declared `do` or `submit` block.
- Extraction expressions must evaluate to a string.
- Extraction variable become available as variables in the final tuple.
- The tuple in the final line is required and defines the columns in the result table.
- `as "..."` sets a custom column heading. If it is omitted, the variable name is used as the column heading.
- `%d`, `%f`, `%w`, `%%` are supported in pattern strings.
- `with` in `analyse` is scalar-only.

## Example

```jbs
a = ("a", ) * 3
i = [1,2,3]
x = i / 2
cases = comb(a + i + x)

do s with cases
{
    echo "Word: ${a}" > en
    echo "Number: ${i}" >> en
    echo "Zahl: ${x}" > de
}

pat_number = "Number: %d"

analyse s with pat_number {
        n = pat_number in "out.log"
        (a, x, i, n as "parsed_number")
}
```

Running JUBE on that example produces:
```bash
% jbs printparam example.jbs
XXX
% jbs example.jbs -o example.yaml
% jbs example.jbs -o example.yaml
XXX
```
