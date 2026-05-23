# `print(...)`

Prints its arguments to command stdout using print value rendering:

## Arguments

- `print()` writes a blank line.
- `value, ...`: zero or more values. Multiple arguments are separated by one space.
- Named form: `print(values = [a, b])`.
- `nrow = 10`: maximum number of screen rows used for row-oriented previews. Use `nrow = 0` for no row limit.
- `jbs run file.jbs` and `jbs file.jbs` print during source evaluation before benchmark work starts.
- Generated `run.sh` stdout is still captured separately in workpackage `stdout` files.

## Returns

`None`.

## Example

```jbs
print("case", [1, 2, 3, 4])
print(values = ["case", [1, 2, 3, 4]])
print(range(100), nrow = 2)
```

```text
jbs> print("case", [1, 2, 3, 4])
"case" [1, 2, 3, 4]
```

`print(...)` writes explicit output using print value rendering. Strings are rendered as double-quoted literals, floats use the shortest decimal representation that round-trips to the same `float64`, lists and tuples are wrapped to screen rows, dictionaries are printed one key/value pair per line, and tables are printed as aligned tables. Range values produced by repeated floating-point addition may still show binary floating-point artifacts when those digits are required for exact round-trip rendering. During `jbs run file.jbs`, print output is written to command stdout before benchmark work starts; shell stdout from `run.sh` is still captured in workpackage `stdout` files.
