# `rev(<list/tuple>)`

reverse a list or a tuple

## Arguments

- `values`: a list or tuple.
- Other value kinds are rejected.

## Returns

The same outer kind with items in reverse order.

## Example

```jbs
rev((1,2,3)) == (3,2,1)
rev([1,2,3]) == [3,2,1]
```
