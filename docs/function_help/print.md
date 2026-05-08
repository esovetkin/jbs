# `print(...)`

## Arguments

- `print()` writes a blank line.
- `value, ...`: zero or more values. Multiple arguments are separated by one space.
- `jbs run file.jbs` and `jbs file.jbs` print during source evaluation before benchmark work starts.
- Generated `run.sh` stdout is still captured separately in workpackage `stdout` files.

## Returns

`null`.

## Example

```jbs
print("case", [1, 2, 3, 4])
```
