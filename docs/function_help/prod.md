# `prod(<list/tuple>)`

`prod` folds a non-empty list or tuple from left to right using `*`.

## Arguments

- `values`: a non-empty list or tuple.
- Equivalent to `reduce(function(a, b) { a * b }, values)`.
- Empty input is an error.
- Singleton input returns its only item unchanged.

## Returns

One reduced value.

## Example

```jbs
prod([2,3,4]) == 24
prod(values = [2,3,4]) == 24
prod(("a",3)) == "aaa"
```
