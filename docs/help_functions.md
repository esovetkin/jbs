# jbs help functions

## `tuple()`, `list()`

treat a list as a tuple, and vice versa

```jbs
tuple([0,1,2]) * 2 == (0,1,2,0,1,2)
list((0,1,2)) * 2 == [0,2,4]
```

## `range(...)`

return a list, similar to Python's `range`

```jbs
range(3) == [0,1,2]
range(1,3) == [1,2]
range(0,10,2) == [0,2,4,6,8]
range(0,1,0.02) == [0,0.02,0.04,0.06,0.08]
```

`range(stop)` and `range(start, stop)` are integer forms.
`range(start, stop, step)` accepts numeric arguments (int/float).

## `rev(<list/tuple>)`

reverse a list or a tuple

```jbs
rev(range(3)) == [2,1,0]
rev((0,1,2)) == (2,1,0)
```

## `len(<string/tuple/list/comb>)`

`len` returns the length of a string/tuple/list and number of rows in a comb

```jbs
3 == len((1,2,3))
10 == len(range(10))
# one unicode character is one character
1 == len("😛")

x = (1,2,3)
y = ("a","b","c","d")
12 == len(comb(x*y))
```

## `comb(<expr>)`

XXX link to docs/help_combinations.md
treat an expression as a combination expression

```jbs
x = [1,2]
y = [3,4]
# XXX x+y == (4,6)
x+y == [4,6]
len(comb(x + y)) == 2
```

## `filter(<list/tuple/comb>, <mask>)`

take subsets of a list, tuple, or a comb

```jbs
x = range(10)

filter(x, 0 == x%2) == [0,2,4,6,8]
# broadcasting applies
filter(x, [true, false]) == [0,2,4,6,8]
# boolean casting applies
filter(x, ["a", "", 1, 0]) == [0,2,4,6,8]

a = comb(x + ("a","b","c") as y)
filter(a, a.y == "a") == [0,3,6,9]
```

Broadcast warning rule for `filter(values, mask)`:

- no mismatch warning when `len(values) % len(mask) == 0`
- warning `W101` when `len(values) % len(mask) != 0`

## `all(...)`, `any(...)`, and vectorized boolean operators

Boolean operators are:

- `!` (negation)
- `&` (and)
- `|` (or)

They work for scalar and list/tuple values.
Truthiness casting and broadcasting apply.

```jbs
true == (1 & "x")
false == (0 | "")
!true == false
![1,0,""] == [false,true,true]
[true,false] & true == [true,false]
```

`all(...)` and `any(...)` reduce list/tuple values to one boolean.
