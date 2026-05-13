# `dict(...)`

## Arguments

- `key = value, ...`: one or more named arguments.
- Argument names become string keys.
- Values may be any JBS value.
- Or one positional table argument: `dict(table_value)`.
- `dict(**entries)` expands a dictionary into named entries.
- `dict()` with no source and no entries is an error.

Literal syntax is also available. Literal keys are expressions and must evaluate to string, int, or bool.

## Returns

One dictionary value.

## Example

```jbs
d = dict(name = "case-a", threads = 8)
same = {"name": "case-a", 1: "one", true: "enabled"}
patch = {"mode": "fast"}
expanded = dict(**patch)
d["name"] == "case-a"

cases = table(x = [1, 2], y = ["a", "b"])
columns = dict(cases)
columns == dict(x = [1, 2], y = ["a", "b"])
```

Duplicate keys are allowed; the last value wins.

`dict(table_value)` creates one dictionary entry per table column. Values are lists containing the column values in row order.
