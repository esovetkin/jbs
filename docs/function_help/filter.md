# `filter(...)`

## Arguments

- `values`: a list, tuple, or table.
- `mask`: a scalar, list, or tuple used as a boolean mask.
- The mask is broadcast cyclically. A length-mismatch warning is emitted when the value count is not divisible by the mask count.
- Non-boolean mask values are tested by truthiness and emit a warning.

## Returns

The same outer kind for list and tuple input. Table input returns a table with the same columns and only matching rows.

## Example

```jbs
x = range(5)
filter(x, [true, false]) == [0,2,4]
```
