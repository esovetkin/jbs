# `len(<string/tuple/list/table/dict>)`

`len` returns the length of a string/tuple/list/dict and the number of rows in a table

## Arguments

- `value`: a string, list, tuple, or table.
- Strings are counted as Unicode code points.
- Tables are counted by row count.

## Returns

One int value.

## Example

```jbs
3 == len((1,2,3))
10 == len(range(10))
# one unicode character is one character
1 == len("😛")
2 == len({1:1, 2:2})

grid = table(x = (1,2,3)) * table(y = ("a","b","c","d"))
12 == len(grid)
len(value = grid) == 12
```
