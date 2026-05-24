# `rbind(...)`

## Arguments

- `*tables`: one or more table values.
- `tables = [...]` expands a list or tuple of table values into the positional arguments.
- All input tables must have the same column names.
- Column order may differ; the output uses the first table's column order.

## Returns

A new table containing all rows from the inputs, in argument order.

`rbind` preserves table projection identity within each input table, but treats
each input argument as an independent source. Projecting columns from the output
therefore returns the appended projection rows from every input.

## Example

```jbs
a = table(id = [1, 2], label = ["a", "b"])
b = table(label = ["c"], id = [3])
out = rbind(a, b)
out2 = rbind(tables = [a, b])
```
