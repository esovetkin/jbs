# `any(...)`

## Arguments

- `values`: a scalar, list, or tuple.
- Table values are rejected.
- Non-boolean values are tested by truthiness and emit a warning.

## Returns

One boolean value. Empty list and tuple inputs return `false`.

## Example

```jbs
any([false, 0, "x"]) == true
any([false, 0, ""]) == false
```
