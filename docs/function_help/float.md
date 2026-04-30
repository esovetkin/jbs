# `float(...)`

## Arguments

- `value`: an int, float, bool, or finite numeric string.
- Lists, tuples, tables, invalid strings, `NaN`, and infinite values are rejected.

## Returns

One float value.

## Example

```jbs
float("1e3") == 1000.0
float(true) == 1.0
```
