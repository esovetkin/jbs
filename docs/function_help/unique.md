# `unique(values)`

Return the first occurrence of each distinct element or table row.

## Arguments

- `values`: a list, tuple, or table.

## Returns

For list input, `unique` returns a list. For tuple input, it returns a tuple.
For table input, it returns a new table with the same columns and the first
visible occurrence of each distinct row.

Table rows are compared by visible column values in table column order. Hidden
table projection metadata is preserved for retained rows.

## Example

```jbs
unique([1, 2, 1])
# [1, 2]

unique((1, 2, 1))
# (1, 2)

cases = table(id = [1, 2, 1], label = ["a", "b", "a"])
unique(cases)
# id label
#  1     a
#  2     b
```
