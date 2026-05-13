# `filter(values, fn)`

Keep items or rows for which a predicate function returns true.

## Arguments

- `values`: a list, tuple, or table.
- `fn`: a predicate function called once for each item or row.
- For lists and tuples, the predicate receives the item value.
- For tables, the predicate receives the row as a dictionary with string keys matching the table column names.
- Non-boolean predicate results are tested by truthiness and emit a warning.

## Returns

The same outer kind for list and tuple input. Table input returns a table with the same columns and only matching rows.

## Example

```jbs
x = range(10)

filter(x, function(v) { v % 2 == 0 }) == [0, 2, 4, 6, 8]
filter(values = x, fn = function(v) { v > 7 }) == [8, 9]

filter(("a", "", "b"), bool) == ("a", "b")

cases = table(id = [1, 2, 3], group = ["a", "b", "a"])
filtered = filter(cases, function(row) { row["group"] == "a" })
# id group
#  1     a
#  3     a
```
