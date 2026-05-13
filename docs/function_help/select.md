# `select(...)`

## Arguments

- `table`: a table value.
- `selector...`: one or more identifier or qualified-identifier column names.
- Named arguments are rejected.
- Column names must exist in the input table.
- Table index projection uses string selectors, for example `grid["id", "replica"]`.

## Returns

A table containing selected columns in selector order.

## Example

```jbs
grid = product(t(id = (1,2)), t(replica = (0,1)))
view = select(grid, id, replica)
```
