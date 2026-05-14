# `tail(values, n = 5)`

Return the last `n` items or rows.

## Arguments

- `values`: a list, tuple, or table.
- `n`: optional non-negative integer. Defaults to `5`.

## Returns

The same outer kind for list and tuple input. Table input returns a new table
with the same columns and only the last `n` rows.

## Example

```jbs
tail([1, 2, 3, 4, 5, 6]) == [2, 3, 4, 5, 6]
tail([1, 2, 3], n = 2) == [2, 3]

cases = table(id = [1, 2, 3], label = ["a", "b", "c"])
last = tail(cases, 2)
# id label
#  2     b
#  3     c
```
