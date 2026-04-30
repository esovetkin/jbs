# `shell(...)`

## Arguments

- `expr`: a string expression, or a string list/tuple expression in assignment-style contexts.
- `shell(...)` is a mode form, not a first-class function value.

## Returns

The inner string or string list value annotated for lowering as JUBE `mode: shell`.

## Example

```jbs
host = shell("hostname")
```
