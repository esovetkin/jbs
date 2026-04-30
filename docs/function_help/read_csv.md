# `read_csv(...)`

## Arguments

- `path`: a string path to a CSV or TSV file.
- The first row must be a header row.
- Header names must be unique valid table column names.
- Relative paths resolve from the source file that contains the call. In REPL they resolve from the REPL working directory.

## Returns

A table value. Column types are inferred as bool, int, float, or string across the full file.

## Example

```jbs
cases = read_csv("./cases.csv")
names(cases)
```
