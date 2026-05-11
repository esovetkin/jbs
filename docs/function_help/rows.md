# `rows(<table>)`

## Arguments

- One table value.
- Named arguments are not accepted.

## Returns

A list of dictionaries. Each dictionary represents one table row. Dictionary
keys are string column names, and values are copied from the corresponding row
cells.

For a table with zero rows, `rows` returns an empty list.

## Example

```jbs
cases = table(x = [1, 2], y = ["a", "b"])
rows(cases)
# [dict(x = 1, y = "a"), dict(x = 2, y = "b")]
```
