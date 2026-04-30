# `map(...)`

## Arguments

- `function`: a function value.
- `values`: a list or tuple.
- Arguments must be positional.
- Builtin names are not first-class callback values; wrap builtin calls in a function literal.

## Returns

A list for list input and a tuple for tuple input. Each element is the callback result for one input item.

## Example

```jbs
inc = function(x) { x + 1 }
map(inc, [1,2]) == [2,3]
```
