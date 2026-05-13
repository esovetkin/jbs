# `map(fn, values)`

`map` one function to each element of a list or tuple.

## Arguments

- `fn`: a user-defined or built-in function value.
- `values`: a list or tuple.
- empty input returns an empty list or tuple of the same outer kind

## Returns

A list for list input and a tuple for tuple input. Each element is the callback result for one input item.

## Example

```jbs
inc = function(x) {
        x + 1
}

map(inc, [1,2,3]) == [2,3,4]
map(fn = inc, values = [1,2,3]) == [2,3,4]
map(inc, (1,2,3)) == (2,3,4)
map(int, ["1","2"]) == [1,2]
```
