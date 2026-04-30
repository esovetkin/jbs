# `product(...)`

## Arguments

- One or more positional table arguments.
- Duplicate column names are rejected.

## Returns

A table containing the Cartesian product of all input rows. Column order follows argument order.

## Example

```jbs
cases = product(t(x = (1,2)), t(y = ("a","b")))
len(cases) == 4
```
