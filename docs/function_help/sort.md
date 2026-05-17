# `sort(values, by = None, inplace = False)`

Sort a list or tuple.

## Arguments

- `values`: a list or tuple.
- `by`: optional comparator function. It is called as `by(a, b)` and must return a boolean. Defaults to JBS `<`.
- `inplace`: optional boolean. When `True`, `values` must be a list and is reordered in place.

## Returns

For non-in-place calls, a sorted list for list input and a sorted tuple for tuple
input. For `inplace = True`, returns `None` and changes the list itself. Equal
items keep their original relative order.

## Example

```jbs
sort([3, 1, 2]) == [1, 2, 3]
sort(("b", "a")) == ("a", "b")

desc = function(a, b) {
        b < a
}
sort([1, 3, 2], by = desc) == [3, 2, 1]

x = [3, 1, 2]
sort(x, inplace = True)
x == [1, 2, 3]
```
