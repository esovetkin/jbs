# `sum(<list/tuple>)`

`sum` folds a non-empty list or tuple from left to right using `+`.

## Arguments

- `values`: a non-empty list or tuple.
- Equivalent to `reduce(function(a, b) { a + b }, values)`.
- Empty input is an error.
- Singleton input returns its only item unchanged.

## Returns

One reduced value.

## Example

```jbs
sum([1,2,3,4]) == 10
sum(("a","b","c")) == "abc"
```
