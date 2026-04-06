# jbs help param

The `param` block defines a parameter set.
Inside a `param` block:

1. You assign variables.
2. The last line is a combination expression using `+` and `*`.
3. Variables used in the final expression become available to `do` and `submit`.

```jbs
param <name> [with ...]
{
        var0 = ...
        var1 = ...
        var0 + var1   # last expression
}
```

You can separate statements with a newline or `;`:

```jbs
param quick
{
        a = (1, 2); b = ("x", "y"); a + b;
}
```

For multiline expressions, use explicit backslash-newline continuation (`\n`):

```jbs
param multiline
{
        x = 1 + \
            2 + 3
        x
}
```

Implicit continuation from operators on the next line is not supported.

## 1) Basic param

```jbs
param cases
{
        nnodes = (1, 2)
        case = ("ddp", "fsdp")

        case + nnodes
}
```

- `+` is a direct sum (zip-like).
- Result rows: `(case="ddp", nnodes=1)`, `(case="fsdp", nnodes=2)`.

## 2) `*` outer product

```jbs
param grid
{
        model = ("small", "base")
        lr = (1e-3, 5e-4, 1e-4)

        model * lr
}
```

- `*` is a Cartesian product.
- Result rows: `2 x 3 = 6` combinations.

## 3) Precedence and parentheses

```jbs
param combos2
{
        a = (1, 2)
        b = ("x", "y")
        c = ("L", "R")

        (a + b) * c
}
```

## 4) Broadcasting with non-matching lengths

```jbs
param warn_example
{
        x = (1, 2)
        y = ("a", "b", "c")

        x + y
}
```

- Lengths `2` and `3` do not match.
- Cyclic broadcasting is applied to length `3`: `(1, "a"), (2, "b"), (1, "c")`.
- No warning is emitted when lengths are divisible:

```jbs
param no_warn_example
{
        x = (1, 2)
        y = ("a", "b", "c", "d")

        # 2 broadcasts into 4 cleanly (4 % 2 == 0), so no W101.
        x + y
}
```

## 5) Importing from other parameter sets (`with`)

Import an entire parameter set:

```jbs
param base
{
        x = (1, 2, 3)
        y = ("a", "b", "c")
        x + y
}

param derived with base
{
        tag = ("cpu", "gpu", "tpu")
        x + tag
}
```

Import selected variables:

```jbs
param derived2 with x from base
{
        phase = ("train", "valid", "test")
        x + phase
}
```

Import multiple selected variables:

```jbs
param derived3 with (x, y) from base
{
        z = ("u", "v", "w")
        (x + y) + z
}
```

If the same visible variable name is imported from different sources in one `param` block, compilation fails with `E214`.

Import from a module alias namespace:

```jbs
use "./lib.jbs" as lib

param derived4 with lib.base
{
        z = ("u", "v", "w")
        z
}
```

Qualified `from` is also supported:

```jbs
use "./lib.jbs" as lib

param derived5 with x from lib.base
{
        y = (1, 2, 3)
        x + y
}
```

## 6) `python()` / `shell()` in `param`

`python()` and `shell()` are allowed as standalone assignment values:

```jbs
param envinfo
{
        queue = python("__import__('os').environ.get('JUBE_QUEUE', 'devel')")
        host = shell("hostname | tr -d '\n'")

        queue + host
}
```

Use them as complete assignment values, not inside tuple/list elements.
