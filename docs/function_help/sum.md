# `sum(...)`

`sum` folds values from left to right using `+`.

## Arguments

- With no arguments, returns `0`.
- With one list or tuple argument, folds the sequence elements.
- With one non-sequence argument, returns that value unchanged.
- With multiple positional arguments, folds those arguments as-is.
- `values = ...` is accepted as a named form for one fold source.
- `*values` can spread a list or tuple into variadic arguments.

## Returns

One reduced value. Impossible `+` operations produce an error.

## Example

```jbs
sum() == 0
sum(10) == 10
sum([1,2,3,4]) == 10
sum(values = [1,2,3,4]) == 10
sum(1,2,3,4) == 10
sum(*[1,2,3,4]) == 10
sum(("a","b","c")) == "abc"
reduce(sum, [0,1,2,3,4]) == 10
```
