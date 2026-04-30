# `select(...)`

## Arguments

- `table`: a table value.
- `selector...`: one or more identifier or qualified-identifier column selectors.
- Named arguments are rejected.
- Selectors must name existing columns.

## Returns

A table containing selected columns in selector order.

## Example

```jbs
grid = product(t(id = (1,2)), t(replica = (0,1)))
view = select(grid, id, replica)
```
