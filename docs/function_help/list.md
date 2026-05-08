# `list(...)`

## Arguments

- `value`: a scalar, list, or tuple.
- Scalar values become one-item lists.
- Table values are rejected.

## Returns

A list value.

## Example

```jbs
list((1,2)) == [1,2]
list("x") == ["x"]
list((0,1,2)) * 2 == [0,2,4]
```
