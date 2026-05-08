# `reduce(<function>, <list/tuple>)`

`reduce` folds a list or tuple from left to right.

## Arguments

- `function`: a function value called as `function(acc, item)`.
- `values`: a non-empty list or tuple.
- reduction uses left-fold semantics:
- first accumulator is the first sequence element
- each next step calls `fn(acc, item)`
- singleton input returns that element unchanged
- empty input is an error
- callback errors stop the reduction immediately

## Returns

One reduced value. Singleton input returns its only item unchanged.

## Example

```jbs
sum2 = function(acc, x) {
        acc + x
}

reduce(sum2, [1,2,3,4]) == 10
reduce(sum2, (1,2,3,4)) == 10
```
