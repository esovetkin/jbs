# `shell(...)`

## Arguments

- `command`: string shell command executed during JBS evaluation.
- `strip`: optional boolean named argument. Defaults to `true`.

Scalar JBS variables referenced as `$name` or `${name}` are exported to the shell command. Unknown or currently unassigned names remain ordinary shell variables. Non-scalar JBS variables referenced this way produce a warning and are exported as empty strings.

## Returns

A string containing stdout. By default, one trailing newline is removed. Use `strip=false` to preserve stdout exactly.

## Errors

If the command cannot start or exits non-zero, JBS raises an error during evaluation. Non-zero exit diagnostics include the exit code and stderr.

## Example

```jbs
hostname = shell("hostname")
x = 1
y = shell("echo $x")
raw = shell("printf 'a\n'", strip=false)
```
