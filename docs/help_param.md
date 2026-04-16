# jbs help param

The `param` block defines variables and how their values are combined.

In JUBE terms, JBS compiles each `param` block into a JUBE [`parameterset`](https://apps.fz-juelich.de/jsc/jube/docu/tutorial.html#parameter-space-creation), and then adds it to step [`use`](https://apps.fz-juelich.de/jsc/jube/docu/tutorial.html#step-dependencies) clauses where it is imported.

## Syntax

```jbs
param <name> [with ...]
{
        var0 = ...
        var1 = ...

        # Final expression defines how variables are combined
        var0 + var1
}
```

Inside a `param` block:

- assignments define variable values
- the final expression must evaluate to a comb value
- legacy final form uses `+` and `*` to define the parameter-space combination
- expression final form is also allowed, for example `comb(...)`
- using the same variable multiple times in the final expression is not allowed
- variables used in the final expression become available to `do` and `submit` (when imported with `with`)

### First-Class `comb` Values

`comb` values are row/column parameter-space objects built from combination algebra.

```jbs
param p
{
        x = (1, 2)
        y = ("a", "b", "c")
        m = comb(x * y)

        mx = m.x
        mxy = m[x, y]

        m
}
```

Notes:

- `m.x` selects one column as a sequence
- `m[x]` and `m[x,y]` project a comb object by columns
- `m[]` is invalid
- unknown columns produce an error

### Comb Aliasing with `as`

You can rename one comb branch explicitly with `as`:

```jbs
param p
{
        x = (1, 2, 3)
        y = ("a", "b", "c")
        a = comb(x + y)

        # valid: rename x-branch to z
        b = (x as z) + a
        b
}
```

Rules:

- `comb(a + a)` is invalid (duplicate output name `a`)
- `comb(a + a as b)` is valid
- `comb(a * range(2))` is invalid (unnamed non-identifier leaf)
- `comb(a * range(2) as b)` is valid
- alias on a comb-valued operand is invalid, for example `(a as t)` when `a` is comb

### Variable Types

Supported value types:

- scalar
  - string, for example `"hello"`
  - int, for example `9007199254740993`
  - float, for example `0.1`, `.1`, `1e-5`, `1E4`
  - bool, for example `true`, `True`, `TRUE`, `false`, `False`, `FALSE`
- tuples, for example `(0,1,2)`
- lists, for example `[0,1,2]`
- kernel functions used in `param` assignments, for example:
  - `range(...)`, `rev(...)`
  - `comb(...)`, `len(...)`, `filter(...)`, `all(...)`, `any(...)`
- mode declarations with `shell(...)` and `python(...)`

Example:

```jbs
queue = python("__import__('os').environ.get('JUBE_QUEUE', 'devel')")
# Removing the trailing newline is important for JUBE
host = shell("hostname | tr -d '\n'")
```

`shell` and `python` lower to JUBE `mode` fields. They should be used as standalone assignment values, not inside tuple/list literals.

### Tuple vs List Behavior

In legacy final combination algebra, tuples and lists behave the same.

They differ in assignment-level arithmetic:

- `[0,1,2] * 2` -> `[0,2,4]` (vector-style arithmetic)
- `(0,1,2) * 2` -> `(0,1,2,0,1,2)` (tuple repetition)

Use `tuple()` and `list()` to convert between representations.

Supported operators:

- assignment: `=`, `+=`, `-=`, `*=`, `/=`, `%=`
- expression: `+`, `-`, `*`, `/`, `%`

## Example

```jbs
param p0
{
    # (1,2,1,2)
    a = (1,2) * 2

    # [0.5, 1, 0.5, 1]
    a = list(a) / 2

    # [3.5, 3, 1.5, 1]
    a += rev(range(4))

    b = ("a", "b")
    # ("a", "b", "c")
    b += ("c",)

    # `+` is direct sum (zip-like)
    a + b
}

# Values of `a` and `b`
# (with a warning for non-matching lengths)
# | p0.a | p0.b |
# |------|------|
# | 3.5  | a    |
# | 3.0  | b    |
# | 1.5  | c    |
# | 1.0  | a    |

param p1
    # Importing `b` from p0 here would cause a collision with local `b`
    with a from p0
{
    # `;` can separate statements on one line
    # `*` is Cartesian product
    b = ("x", "y"); a * b
}

# | p1.a | p1.b |
# |------|------|
# | 3.5  | x    |
# | 3.5  | y    |
# | 3.0  | x    |
# | 3.0  | y    |
# | 1.5  | x    |
# | 1.5  | y    |
# | 1.0  | x    |
# | 1.0  | y    |

param p2
    with p0
{
    c = (true, false)

    # Operations on an entire imported parameter space are allowed
    p0 + c
}

# | p2.a | p2.b | p2.c  |
# |------|------|-------|
# | 3.5  | a    | true  |
# | 3.0  | b    | false |
# | 1.5  | c    | true  |
# | 1.0  | a    | false |
```

You can import multiple variables with:

```jbs
with (x, y) from base
```

Importing duplicate visible names causes a compilation error. You can also import from modules:

```jbs
use "./lib.jbs" as lib

param derived4
    with lib.base
{
    ...
}
```

Operator precedence and parentheses work as usual, for example `(a + b) * c`.

### Extra `with` Forms

The following `with` forms are also valid in `param` headers:

```jbs
with p[x, y]
with (x, y) in p
with p[x, y] as a
```

- `with p[x,y]` and `with (x,y) in p` are equivalent
- `with p[x,y] as a` also binds `a` as a comb alias

### `len`, `filter`, `all`, `any`

```jbs
param p
{
        x = (1, 2, 3, 4)
        m = comb(x)

        even = filter(m, m.x % 2 == 0)
        n = len(even)
        has_even = any(even.x)
        all_small = all(m.x <= 4)

        even
}
```

Behavior summary:

- `len` accepts list, tuple, string, and comb
- `filter` accepts list, tuple, or comb as first argument
- `filter` mask supports scalar or vector inputs with broadcast rules
- `all` and `any` reduce scalar/sequence truthiness to one boolean

### Multiline Continuation

Use backslash-newline for multiline expressions:

```jbs
# Implicit continuation after operators is not supported
x = 1 + \
    2 + 3
```

### Unused Variable Warnings

JBS warns about imported-but-unused variables and local variables that do not contribute to the final expression.

```jbs
param p0
{
        a = (1, 2)
        x = "unused"
        b = ("u", "v")
        # Warning: x does not contribute to the final expression
        a + b
}

param p1
{
        x = "hello "
        y = x + "world"
        b = ("a", y)
        # No warning: usage is transitive through y
        b
}
```
