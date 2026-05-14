# `names()`, `names(<module>)`, `names(<table>)`, `names(<dict>)`

- `names()` returns visible variable names in the current scope
- `names(module)` returns direct variable names in that module namespace
- `names(table)` returns table column names
- `names(dictionary)` returns dictionary keys

## Arguments

- No argument returns variable names visible in the current scope.
- A module namespace argument returns direct member names in that namespace.
- A table argument returns table column names.
- A dictionary argument returns dictionary keys in dictionary iteration order.
- Dictionary string, int, and bool keys are returned as their original JBS value types.
- Named form: `names(values = [table_value])` or `names(values = [dictionary_value])`.
- Scope metadata is required, which is available in normal file and REPL expression contexts.

## Returns

A list. Scope, module, and table names are strings. Dictionary keys are returned as their original string, int, or bool key values.

## Example

```jbs
cases = table(x = (1,2))
names(cases) == ["x"]
names(values = [cases]) == ["x"]

settings = {"x": 1, 2: "two", true: "enabled"}
names(settings) == ["x", 2, true]
```
