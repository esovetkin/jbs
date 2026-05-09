# `table(...)`

## Arguments

- One or more named column arguments such as `id = values`.
- Or one positional dictionary argument: `table(dict_value)`.
- Column values may be scalar, list, or tuple values.
- Scalar values are treated as one-row columns and broadcast when needed.
- Shorter non-empty columns are cyclically broadcast to the longest column.
- Non-divisible broadcasts emit `W101`.
- Empty columns can only be used when all columns are empty.
- Dictionary keys must be valid string column names such as `x`, `system_name`, or `ns.value`.
- Duplicate column names and mixed positional/named arguments are rejected.
- `t(...)` is an alias of `table(...)`.

## Returns

A table value with columns in argument order.

## Example

```jbs
cases = table(id = (1,2), label = ("a","b"))
short = t(id = (1,2), label = ("a","b"))
from_dict = table(dict(id = range(5), label = range(10)))
```
