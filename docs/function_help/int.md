# `int(...)`

## Arguments

- `value`: an int, float, bool, or base-10 integer string.
- Float input is truncated toward zero.
- Lists, tuples, tables, and non-integer strings are rejected.

## Returns

One int value.

## Example

```jbs
int(3.9) == 3
int("42") == 42
```
