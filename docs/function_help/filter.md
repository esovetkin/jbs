# `filter(<list/tuple/table>, <mask>)`

Take subsets of a list, tuple, or a table

## Arguments

- `values`: a list, tuple, or table.
- `mask`: a scalar, list, or tuple used as a boolean mask.
- The mask is broadcast cyclically. A length-mismatch warning is emitted when the value count is not divisible by the mask count.
- Non-boolean mask values are tested by truthiness and emit a warning.

## Returns

The same outer kind for list and tuple input. Table input returns a table with the same columns and only matching rows.

## Example

```jbs
x = range(10)

filter(x, 0 == x%2) == [0,2,4,6,8]
# broadcasting applies
filter(x, [true, false]) == [0,2,4,6,8]
# boolean casting applies
filter(x, ["a", "", 1, 0]) == [0,2,4,6,8]

a = table(x = x, y = ("a","b","c","a","b","c","a","b","c","a"))
filter(a, a.y == "a") == [0,3,6,9]
```
