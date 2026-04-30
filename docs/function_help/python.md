# `python(...)`

## Arguments

- `expr`: a string expression, or a string list/tuple expression in assignment-style contexts.
- `python(...)` is a mode form, not a first-class function value.

## Returns

The inner string or string list value annotated for lowering as JUBE `mode: python`.

## Example

```jbs
name = "job"
cmd = python("'" + name + "-' + str(1)")
```
