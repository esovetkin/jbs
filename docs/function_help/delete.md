# `delete(...)`

`delete(...)` removes one or more variables from the current mutable scope.

## Arguments

- One or more bare variable names.
- Evaluated forms: `delete(*names)` or `delete(names = ["x", "y"])`, where names are strings.

Direct string arguments, qualified names, and other expressions are not accepted.

## Returns

`None`.

## Example

```jbs
x = 1
y = 2
delete(x, y)
delete(names = ["x", "y"])
```

At top level, `delete(...)` can remove user-defined globals. It cannot remove
built-in global variables such as `jbs_name` or built-in functions such as
`range`.

Inside a user function, `delete(...)` only removes variables from the current
function call scope. It does not delete captured variables from outer scopes.
