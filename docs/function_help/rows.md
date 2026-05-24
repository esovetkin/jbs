# `rows(<table>)`

## Arguments

- `table`: one table value.

## Returns

A plain list of dictionaries. Each dictionary represents one physical table row.
Dictionary keys are string column names, and values are copied from the
corresponding row cells.

The list does not preserve hidden table projection metadata. Passing it back to
`table(...)` creates a new row-oriented table.

For a table with zero rows, `rows` returns an empty list. Passing that list back
to `table(...)` creates an empty table with no columns.

## Example

```jbs
cases = table(x = [1, 2], y = ["a", "b"])
rows(cases)
rows(table = cases)
# [dict(x = 1, y = "a"), dict(x = 2, y = "b")]

table(rows(cases))
# same visible rows as cases, but rebuilt as a new row-oriented table
```
