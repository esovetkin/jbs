# jbs help analyse

The `analyse` block targets an existing `do` or `submit` step and maps matched patterns and step-visible variables into a result table. The resulting table is saved as `result/result_<step_name>.dat`.

In JUBE terms, JBS lowers `analyse` into JUBE `patternset`, `analyser` and `result` sections. The overall workflow matches the [JUBE tutorial example](https://apps.fz-juelich.de/jsc/jube/docu/tutorial.html#creating-a-result-table) for creating a result table.

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
- `with` in `analyse` can import only from `let` namespaces (useful for reusable pattern strings).
- Extraction expressions must evaluate to a string.
- Left-hand extraction aliases become available as variables in the final tuple.
- The tuple in the final line is required and defines the columns in the result table.
- `as "..."` sets a custom column heading. If it is omitted, the variable name is used as the column heading.

## Example

```jbs
param p
{
        a = ("a", ) * 3
        i = [1,2,3]
        x = i / 2
        a + i + x
}

do write_number
        with p
{
        echo "Word: ${a}" > en
        echo "Number: ${i}" >> en
        echo "Zahl: ${x}" > de
}

analyse write_number
{
        alpha = "Word: %w" in "en"
        en = "Number: %d" in "en"
        de = "Zahl: %f" in "de"

        (a, i, x, en as "Number", de as "Zahl", alpha as "Letter")
}
```

Running JUBE on that example produces:
```bash
% jbs printparam example.jbs
| p.a | p.i | p.x | step             |
|-----|-----|-----|------------------|
| a   | 1   | 0.5 | do: write_number |
| a   | 2   | 1.0 | do: write_number |
| a   | 3   | 1.5 | do: write_number |
% jbs example.jbs -o example.yaml
% jube-autorun example.yaml
...
a,i,x,Number,Zahl,Letter
a,1,0.5,1,0.5,a
a,2,1,2,1.0,a
a,3,1.5,3,1.5,a
```
