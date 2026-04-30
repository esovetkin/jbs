# `all(...)`

## Arguments

- `values`: a scalar, list, or tuple.
- Table values are rejected.
- Non-boolean values are tested by truthiness and emit a warning.

## Returns

One boolean value. Empty list and tuple inputs return `true`.

## Example

```jbs
all([true, 1, "x"]) == true
all([true, 0]) == false
```
