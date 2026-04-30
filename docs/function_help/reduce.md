# `reduce(...)`

## Arguments

- `function`: a function value called as `function(acc, item)`.
- `values`: a non-empty list or tuple.
- Arguments must be positional.
- Builtin names are not first-class callback values; wrap builtin calls in a function literal.

## Returns

One reduced value. Singleton input returns its only item unchanged.

## Example

```jbs
sum2 = function(acc, x) { acc + x }
reduce(sum2, [1,2,3]) == 6
```
