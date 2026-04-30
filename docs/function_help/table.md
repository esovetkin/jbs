# `table(...)`

## Arguments

- One or more named column arguments such as `id = values`.
- Column values may be scalar, list, or tuple values.
- Scalar values are treated as one-row columns.
- All columns must have equal length.
- Duplicate column names and positional arguments are rejected.
- `t(...)` is an alias of `table(...)`.

## Returns

A table value with columns in argument order.

## Example

```jbs
cases = table(id = (1,2), label = ("a","b"))
short = t(id = (1,2), label = ("a","b"))
```
