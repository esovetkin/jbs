# `get(...)`

## Arguments

- `dict_value`: a dictionary.
- `key`: a string, int, or bool key.
- `default_value`: value returned when the key is missing.

Named arguments are not accepted.

## Returns

The stored value for `key`, or `default_value` when the key is missing.

## Example

```jbs
d = dict(name = "case-a")
get(d, "name", "fallback") == "case-a"
get(d, "missing", "fallback") == "fallback"
```

Missing keys do not produce diagnostics.
