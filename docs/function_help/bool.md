# `bool(...)`

## Arguments

- `value`: any JBS value.
- Values are converted with the same truthiness rules used by logical operators, `filter(...)`, `all(...)`, and `any(...)`.
- Empty strings, zero numeric values, `null`, empty lists, empty tuples, empty dictionaries, and empty tables are false.
- Non-empty strings are true, so `bool("false")` is true.

## Returns

One bool value.

## Example

```jbs
bool(0) == false
bool("x") == true
bool([]) == false
```
