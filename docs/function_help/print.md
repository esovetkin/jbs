# `print(...)`

Prints its arguments to command stdout using REPL value rendering:

## Arguments

- `print()` writes a blank line.
- `value, ...`: zero or more values. Multiple arguments are separated by one space.
- Named form: `print(values = [a, b])`.
- `nrow = 10`: maximum number of screen rows used for row-oriented previews. Use `nrow = 0` for no row limit.
- `jbs run file.jbs` and `jbs file.jbs` print during source evaluation before benchmark work starts.
- Generated `run.sh` stdout is still captured separately in workpackage `stdout` files.

## Returns

`null`.

## Example

```jbs
print("case", [1, 2, 3, 4])
print(values = ["case", [1, 2, 3, 4]])
print(range(100), nrow = 2)
```

`print(...)` writes explicit output using the same value rendering as the REPL. Lists and tuples are wrapped to screen rows, dictionaries are printed one key/value pair per line, and tables are printed as aligned tables. During `jbs run file.jbs`, print output is written to command stdout before benchmark work starts; shell stdout from `run.sh` is still captured in workpackage `stdout` files.
