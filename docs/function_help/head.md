# `head(values, n = 5)`

Return the first `n` items or rows.

## Arguments

- `values`: a list, tuple, or table.
- `n`: optional non-negative integer. Defaults to `5`.

## Returns

The same outer kind for list and tuple input. Table input returns a new table
with the same columns and only the first `n` rows.

## Example

```jbs
head([1, 2, 3, 4, 5, 6]) == [1, 2, 3, 4, 5]
head([1, 2, 3], n = 2) == [1, 2]

cases = table(id = [1, 2, 3], label = ["a", "b", "c"])
first = head(cases, 2)
# id label
#  1     a
#  2     b
```
