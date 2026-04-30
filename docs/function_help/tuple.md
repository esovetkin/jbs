# `tuple(...)`

## Arguments

- `value`: a scalar, list, or tuple.
- Scalar values become one-item tuples.
- Table values are rejected.

## Returns

A tuple value.

## Example

```jbs
tuple([1,2]) == (1,2)
tuple("x") == ("x")
```
