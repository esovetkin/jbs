# jbs help let

The `let` block defines a namespace of reusable scalar variables.

`let` values can be imported with `with` in `param`, `do`, `submit`, and `analyse`.

## Syntax

```jbs
let <name>
{
        <var0> = <expr>
        <var1> = <expr>
        ...
}
```

## Rules

- `let` values must be scalar.
- Allowed value kinds: string, int, float, bool.
- `shell("...")` and `python("...")` are allowed as scalar string-producing assignments.
- Tuple/list values are not allowed in `let`.
- Import variables with `with` and use unqualified names.

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

## Step Imports from `let`

`do`/`submit` support all `with` forms for let namespaces:

```jbs
let l
{
        # ensure there are no new lines, otherwise JUBE cannot handle it
        systemname = shell("hostname | tr -d '\n'")
        queue = "batch"
}

do s1
        with l
{
        echo ${systemname} ${queue}
}

do s2
        with systemname from l
{
        echo ${systemname}
}

do s3
        with (systemname, queue) from l
{
        echo ${systemname} ${queue}
}
```

For step imports from `let`, jbs generates synthetic YAML `parameterset` entries and adds them to `step.use`.

## Notes

- In `param`, `with <let_name>` imports all let variables into local scope.
- In `analyse`, `with` imports are allowed only from `let` namespaces and imported let variables must be strings.
- `W310`/`W311` also apply to variables defined in `let` blocks.
