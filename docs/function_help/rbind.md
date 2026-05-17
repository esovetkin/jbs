# `rbind(...)`

## Arguments

- `*tables`: one or more table values.
- `tables = [...]` expands a list or tuple of table values into the positional arguments.
- All input tables must have the same column names.
- Column order may differ; the output uses the first table's column order.

## Returns

A new table containing all rows from the inputs, in argument order.

## Example

```jbs
a = table(id = [1, 2], label = ["a", "b"])
b = table(label = ["c"], id = [3])
out = rbind(a, b)
out2 = rbind(tables = [a, b])
```
