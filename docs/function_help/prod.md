# `prod(...)`

`prod` folds values from left to right using `*`.

## Arguments

- With no arguments, returns `1`.
- With one list or tuple argument, folds the sequence elements.
- With one non-sequence argument, returns that value unchanged.
- With multiple positional arguments, folds those arguments as-is.
- `values = ...` is accepted as a named form for one fold source.
- `*values` can spread a list or tuple into variadic arguments.

## Returns

One reduced value. Impossible `*` operations produce an error.

## Example

```jbs
prod() == 1
prod(10) == 10
prod([2,3,4]) == 24
prod(values = [2,3,4]) == 24
prod(2,3,4) == 24
prod(*[2,3,4]) == 24
prod(("a",3)) == "aaa"
```
