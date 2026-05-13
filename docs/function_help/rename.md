# `rename(...)`

## Arguments

- `table`: a table value.
- Keyword arguments map existing column names to replacement column names, for example `rename(cases, x = "id")`.
- `**mapping` expands a dictionary whose string keys are existing column names and whose string values are replacement column names.
- Old column names must exist.
- New column names must be valid table column names.
- Output column names must be unique.

## Returns

A new table with renamed columns.

## Example

```jbs
cases = table(x = [1, 2], label = ["a", "b"])
renamed = rename(cases, x = "id", label = "name")
dotted = rename(cases, **{"old.col": "new.col"})
```
