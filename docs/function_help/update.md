# `update(...)`

## Arguments

- `dict`: dictionary to copy.
- `key = value, ...`: zero or more named updates.
- Named base form: `update(dict = d, key = value)`.

Positional update arguments are not accepted.

## Returns

A new dictionary with the named keys replaced or added.

## Example

```jbs
base = dict(a = 1, b = 2)
next = update(base, b = 3, c = 4)
same = update(dict = base, b = 3, c = 4)
next == dict(a = 1, b = 3, c = 4)
base == dict(a = 1, b = 2)
```

`left + right` also merges dictionaries and prefers values from `right`.
