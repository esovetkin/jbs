# `zip(...)`

## Arguments

- One or more positional table arguments.
- All input tables must have the same row count.
- Duplicate column names are rejected.

## Returns

A table that merges rows by index. Column order follows argument order.

## Example

```jbs
env = zip(t(host = ("h0","h1")), t(port = (1,2)))
len(env) == 2
```
