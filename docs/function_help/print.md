# `print(...)`

Prints its arguments to command stdout using REPL value rendering:

## Arguments

- `print()` writes a blank line.
- `value, ...`: zero or more values. Multiple arguments are separated by one space.
- Named form: `print(values = [a, b])`.
- `jbs run file.jbs` and `jbs file.jbs` print during source evaluation before benchmark work starts.
- Generated `run.sh` stdout is still captured separately in workpackage `stdout` files.

## Returns

`null`.

## Example

```jbs
print("case", [1, 2, 3, 4])
print(values = ["case", [1, 2, 3, 4]])
```

`print(...)` writes explicit output using the same value rendering as the REPL. During `jbs run file.jbs`, print output is written to command stdout before benchmark work starts; shell stdout from `run.sh` is still captured in workpackage `stdout` files.
