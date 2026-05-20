# `sample(values, size = len(values), replace = false)`

Randomly sample elements or rows.

## Arguments

- `values`: a list, tuple, or table.
- `size`: optional non-negative integer. Defaults to the input length.
- `replace`: optional boolean. Defaults to `false`. When `true`, values may be selected more than once.

## Returns

A value with the same outer kind as `values`: list input returns a list, tuple
input returns a tuple, and table input returns a new table with the same columns
and sampled rows.

## Example

```jbs
setseed(42)

sample(range(10), size = 3)
sample(("a", "b", "c"), size = 2)

cases = table(id = range(10), group = ["a", "b"])
sample(cases, size = 2)
sample([1], size = 3, replace = true) == [1, 1, 1]
```
