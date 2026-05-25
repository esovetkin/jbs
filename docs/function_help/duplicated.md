# `duplicated(values)`

Return a boolean mask identifying repeated elements or table rows.

## Arguments

- `values`: a list, tuple, or table.

## Returns

A list of booleans with the same length as `values`.

The first occurrence of a value is `false`. Later equal occurrences are `true`.
For table input, rows are compared by visible column values in table column
order.

## Example

```jbs
x = [1, 2, 1]
duplicated(x)
# [false, false, true]

unique(x) == x[!duplicated(x)]
# true
```
