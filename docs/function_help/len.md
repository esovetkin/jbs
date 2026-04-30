# `len(...)`

## Arguments

- `value`: a string, list, tuple, or table.
- Strings are counted as Unicode code points.
- Tables are counted by row count.

## Returns

One int value.

## Example

```jbs
len("ab") == 2
len(t(x = (1,2,3))) == 3
```
