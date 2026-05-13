# `read_csv(<path>)`

`read_csv(...)` reads a CSV or TSV file and returns one table value.

## Arguments

- `path`: a string path to a CSV or TSV file. Named form: `read_csv(path = "./cases.csv")`.
- The first row must be a header row.
- Header names must be unique valid table column names.
- Relative paths resolve from the source file that contains the call. In REPL they resolve from the REPL working directory.
- type inference is per column across all rows:
  - `bool` if every value is `true` or `false`
  - otherwise `int` if every value is a base-10 integer
  - otherwise `float` if every value is a finite float
  - otherwise `string`
- empty cells force that column to become `string`
- every data row must have the same number of fields as the header row

## Returns

A table value. Column types are inferred as bool, int, float, or string across the full file.

## Example

```jbs
cases = read_csv("./cases.csv")
same = read_csv(path = "./cases.csv")
names(cases)
```
