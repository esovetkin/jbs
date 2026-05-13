# `rows(<table>)`

## Arguments

- One table value.
- Named arguments are not accepted.

## Returns

A list of dictionaries. Each dictionary represents one table row. Dictionary
keys are string column names, and values are copied from the corresponding row
cells.

For a table with zero rows, `rows` returns an empty list.
That list can still be passed back to `table(...)` to recreate the original
zero-row table shape.

## Example

```jbs
cases = table(x = [1, 2], y = ["a", "b"])
rows(cases)
# [dict(x = 1, y = "a"), dict(x = 2, y = "b")]

table(rows(cases))
# same table as cases
```
