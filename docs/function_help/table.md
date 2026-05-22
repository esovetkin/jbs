# `table(...)`

## Arguments

- One or more named column arguments such as `id = values`.
- Or one positional dictionary argument: `table(dict_value)`.
- Or one positional list of row dictionaries: `table(rows_value)`.
- `table(**columns)` expands a dictionary into named columns.
- Column values may be scalar, list, or tuple values.
- Scalar values are treated as one-row columns and broadcast when needed.
- Shorter non-empty columns are cyclically broadcast to the longest column.
- Non-divisible broadcasts emit `W101`.
- Empty columns can only be used when all columns are empty.
- Dictionary keys must be valid shell variable names such as `x`, `system_name`, or `_tmp`.
- Row dictionaries must all have the same key set.
- Row dictionary values must be scalar: int, float, string, or bool.
- `table([])` creates an empty table with no columns.
- `table(rows(table_value))` recreates scalar-cell tables, including zero-row tables.
- Duplicate column names and mixed positional/named arguments are rejected.
- `t(...)` is an alias of `table(...)`.

## Returns

A table value with columns in argument order.

## Example

```jbs
cases = table(id = (1,2), label = ("a","b"))
short = t(id = (1,2), label = ("a","b"))
from_dict = table(dict(id = range(5), label = range(10)))
from_kwargs = table(**dict(id = range(5), label = range(10)))
from_rows = table([dict(id = 1, label = "a"), dict(id = 2, label = "b")])
roundtrip = table(rows(cases))
```
