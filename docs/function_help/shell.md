# `shell(...)`

Runs a shell command while JBS is being evaluated and returns stdout as a string:

## Arguments

- `command`: string shell command executed during JBS evaluation.
- `strip`: optional boolean named argument. Defaults to `true`.

Scalar JBS variables referenced as `$name` or `${name}` are exported to the shell command. Unknown or currently unassigned names remain ordinary shell variables. Non-scalar JBS variables referenced this way produce a warning and are exported as empty strings.

`shell(...)` runs in the source file's directory, captures stdout as a string, and can export currently assigned scalar JBS variables referenced as shell variables.

If the command cannot start or exits non-zero, JBS raises an error during evaluation. Non-zero exit diagnostics include the exit code and stderr.

## Returns

A string containing stdout. By default, one trailing newline is removed. Use `strip=false` to preserve stdout exactly.

## Example

```jbs
hostname = shell("hostname")
hostname = shell(command = "hostname")
x = 1
y = shell("echo $x")
raw = shell("printf 'a\n'", strip = false)
```
