# `order(values, by = None)`

Return the permutation that would sort a list or tuple.

## Arguments

- `values`: a list or tuple.
- `by`: optional comparator function. It is called as `by(a, b)` and must return a boolean. Defaults to JBS `<`.

## Returns

A list of zero-based integer indexes. Applying those indexes to `values`
rearranges it into sorted order. Equal items keep their original relative order.
The result can also reorder table rows, for example `cases[order(cases.x)]`.

## Example

```jbs
order([3, 1, 2]) == [1, 2, 0]
order(["b", "a", "c"]) == [1, 0, 2]

desc = function(a, b) {
        b < a
}
order([1, 3, 2], by = desc) == [1, 2, 0]
```
