# jbs help functions

## `function(...) { ... }`

Function literals are ordinary expressions.

```jbs
add = function(a, b = 1) {
        a + b
}

add(2)
add(2, b = 3)
```

Rules:

- function values are first-class: they can be assigned, returned, passed to calls, stored in lists/tuples, and imported from modules
- the result is the value from `return expr`, or the last expression if execution reaches the end of the body
- parameters are comma-separated and may have defaults
- call sites may mix positional and named arguments, but positional arguments must come first
- local assignments inside the body are local to that function and shadow captured or global names
- local assignments may still use compound operators; that mutability is function-local, not top-level
- nested functions capture outer locals lexically

Examples:

```jbs
make_adder = function(delta) {
        function(x) {
                x + delta
        }
}

add2 = make_adder(2)
add2(3) == 5
```

Imported function-valued globals behave like ordinary globals in expression contexts:

```jbs
use "./lib.jbs" as lib
lib.add(1, 2)

use add from "./lib.jbs"
add(1, 2)
```

Data-only boundary:

- function-valued globals are expression-visible
- they are not valid `with` sources
- they are not valid `submit ... use ...` sources
- they are not valid `analyse with ...` imports

When printed in REPL or via `str(...)`, a function value renders as `<function>`.

## `tuple()`, `list()`

treat a list as a tuple, and vice versa

```jbs
tuple([0,1,2]) * 2 == (0,1,2,0,1,2)
list((0,1,2)) * 2 == [0,2,4]
```

## `int()`, `float()`, `str()`

scalar conversions:

```jbs
int("42") == 42
int(3.9) == 3
float("1e3") == 1000.0
str((1,2)) == "(1,2)"
str([1,2]) == "[1,2]"
```

Rules:

- `int(...)` accepts `int`, `float`, `bool`, and integer strings
- `int(...)` truncates float input toward zero
- `float(...)` accepts `int`, `float`, `bool`, and finite numeric strings
- `int(...)` and `float(...)` reject list/tuple/comb values
- `str(...)` stringifies the whole value
- `str([1,2])` returns one string value, not a list of strings

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

## `names()`, `names(<module>)`, `names(<comb>)`

`names(...)` returns a list of strings.

- `names()` returns visible variable names in the current scope
- `names(module)` returns direct variable names in that module namespace
- `names(comb)` returns comb column names

```jbs
use jsc

a = range(2)
b = rev(range(2))
params = comb(a as x * b as y)

names()
names(jsc)
names(params)
names(params[x])
```

Rules:

- `names()` returns only variable names, not step names or module aliases
- `names(module)` returns direct variable members only, not nested descendants
- `names(comb)` preserves comb column order when available

## `read_csv(<path>)`

`read_csv(...)` reads a CSV or TSV file and returns one `comb` value.

```jbs
cases = read_csv("./cases.csv")
names(cases)
len(cases)

use "./lib/module.jbs" as lib
other = read_csv("./cases.tsv")
```

Rules:

- `read_csv(...)` takes exactly one string argument
- the first row is the header row
- header names must be unique valid comb column names such as `x`, `system_name`, or `ns.value`
- both quoted and unquoted fields are supported
- `.csv` files use `,`
- `.tsv` files use a tab
- other filenames are sniffed from the first non-empty line: tab-without-comma means TSV, otherwise CSV
- every data row must have the same number of fields as the header row
- type inference is per column across all rows:
  - `bool` if every value is `true` or `false`
  - otherwise `int` if every value is a base-10 integer
  - otherwise `float` if every value is a finite float
  - otherwise `string`
- empty cells force that column to become `string`
- relative paths resolve from the source file that contains the call:
  - main file calls resolve relative to the main `.jbs` file
  - imported module calls resolve relative to that module file
  - REPL calls resolve relative to the current working directory

## `comb(<expr>)`

`comb(...)` builds a table-like value from combination algebra.

```jbs
x = [1,2]
y = [3,4]
# XXX x+y == (4,6)
x+y == [4,6]
len(comb(x + y)) == 2
```

Use a top-level assignment to keep the result and import it into steps:

```jbs
cases = comb(x * y)

do run
        with cases
{
        echo "${x} ${y}"
}
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

## `map(<function>, <list/tuple>)`

`map(...)` applies one function to each element of a list or tuple.

```jbs
inc = function(x) {
        x + 1
}

map(inc, [1,2,3]) == [2,3,4]
map(inc, (1,2,3)) == (2,3,4)
```

Rules:

- `map(...)` takes exactly two positional arguments
- the first argument must evaluate to a function value
- the second argument must be a list or tuple
- each element is passed as one positional argument to the callback
- result kind is preserved:
  - list in -> list out
  - tuple in -> tuple out
- empty input returns an empty list or tuple of the same outer kind
- callback errors stop the map immediately

Builtin names are not first-class callback values in this feature, so use:

```jbs
map(function(x) {
        int(x)
}, ["1","2"])
```

not:

```jbs
map(int, ["1","2"])
```

## `reduce(<function>, <list/tuple>)`

`reduce(...)` folds a list or tuple from left to right.

```jbs
sum2 = function(acc, x) {
        acc + x
}

reduce(sum2, [1,2,3,4]) == 10
reduce(sum2, (1,2,3,4)) == 10
```

Rules:

- `reduce(...)` takes exactly two positional arguments
- the first argument must evaluate to a function value
- the second argument must be a list or tuple
- reduction uses left-fold semantics:
  - first accumulator is the first sequence element
  - each next step calls `fn(acc, item)`
- singleton input returns that element unchanged
- empty input is an error
- callback errors stop the reduction immediately

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
