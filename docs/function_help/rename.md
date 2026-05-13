# `rename(...)`

## Arguments

- `table`: a table value.
- `mapping`: a dictionary whose string keys are existing column names and whose string values are replacement column names.
- Named arguments are rejected.
- Old column names must exist.
- New column names must be valid table column names.
- Output column names must be unique.

## Returns

A new table with renamed columns.

## Example

```jbs
cases = table(x = [1, 2], label = ["a", "b"])
renamed = rename(cases, {"x": "id", "label": "name"})
```
