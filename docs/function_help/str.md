# `str(...)`

## Arguments

- `value`: any JBS value.
- List, tuple, and table input is converted to one string value, not mapped element-by-element.

## Returns

One string using JBS display formatting.

## Example

```jbs
str([1,2]) == "[1,2]"
str((1,2)) == "(1,2)"
```
