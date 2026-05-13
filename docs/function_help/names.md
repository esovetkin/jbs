# `names()`, `names(<module>)`, `names(<table>)`

- `names()` returns visible variable names in the current scope
- `names(module)` returns direct variable names in that module namespace
- `names(table)` returns table column names

## Arguments

- No argument returns variable names visible in the current scope.
- A module namespace argument returns direct member names in that namespace.
- A table argument returns table column names.
- Named form: `names(values = [table_value])`.
- Scope metadata is required, which is available in normal file and REPL expression contexts.

## Returns

A list of strings.

## Example

```jbs
cases = table(x = (1,2))
names(cases) == ["x"]
names(values = [cases]) == ["x"]
```
