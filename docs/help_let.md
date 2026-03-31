# jbs help let

The `let` block defines a namespace of reusable variables.

You can reference values from a let namespace using `<namespace>.<variable>` in `param`, `submit`, and `analyse` expressions.

## Syntax

```jbs
let <name>
{
        <var0> = <expr>
        <var1> = <expr>
        ...
}
```

## Example

```jbs
let p
{
        number = "Number: %d"
        letter = "Letter: %w"
        retries = 3
}

param cases
        with p
{
        x = (1, 2)
        y = (number, letter)
        x + y
}

do write
        with cases
{
        echo ${x} ${y}
}

analyse write
{
        n = p.number in "out.log"
        w = "Word: %w" in "out.log"
        (n, w)
}
```

## Notes

- Flat tuples/lists are allowed in `let` assignments.
- Nested tuple/list values are rejected.
- In `param`, `with <let_name>` imports all let variables into local scope.
