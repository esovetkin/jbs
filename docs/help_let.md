# jbs help let

The `let` block defines a namespace of scalar variables, and can be imported using `with` in `param`, `do`, `submit`, and `analyse` sections.

In JUBE terms, JBS lowers `let` into JUBE [`parameterset` sections](), and ensures that they are added in [`use` step attributes]() in every step they are imported.

## Syntax

```jbs
let <name>
{
        <var0> = <expr>
        <var1> = <expr>
        ...
}
```

- `let` values must be scalar.
- Allowed value kinds: string, int, float, bool.
- `shell("...")` and `python("...")` are allowed as scalar string-producing assignments.
- Tuple/list values are not allowed in `let`.
- Import variables with `with` and use unqualified names.
- `let` expressions can use globals and earlier assignments in the same `let` block.
- `let` expressions cannot implicitly read variables from other `let` namespaces.
- Assignment operators: `=`, `+=`, `-=`, `*=`, `/=`, `%=`.
- In `param`, `with <let_name>` imports all let variables into local scope.
- In `analyse`, `with` imports are allowed only from `let` namespaces, and imported let variables must be strings.
- `W310` and `W311` also apply to variables defined in `let` blocks.

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
        with p
{
        n = number in "out.log"
        w = letter in "out.log"
        (n, w)
}
```
