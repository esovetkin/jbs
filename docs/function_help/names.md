# `names(...)`

## Arguments

- No argument returns variable names visible in the current scope.
- A module namespace argument returns direct member names in that namespace.
- A table argument returns table column names.
- Scope metadata is required, which is available in normal file and REPL expression contexts.

## Returns

A list of strings.

## Example

```jbs
cases = table(x = (1,2))
names(cases) == ["x"]
```
