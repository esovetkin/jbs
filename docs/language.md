# JBS Language

JBS is a domain-specific scripting language for benchmark parameter-space construction, DAG-based workpackage execution, and structured analysis of generated outputs. JBS has separate evaluation and run stages. During evaluation, declared variables are evaluated and the benchmark directory structure is created. At runtime, scripts are executed in parallel and results are processed.

## Program shape

```jbs
# import data and functions from other .jbs files
use ...

# define global configuration and parameter sets
# parameter sets can be built with function calls, loops, and imports from other JBS modules
<variable> = ...

# define one or more execution steps
do <step_name>
    with <variable> [as <name>], ...
    after <other_step_name> ...
    fsub ...
{
# shell commands
...
}

# define analysis blocks
analyse <step_name>
{
    <pattern> = "<regex>" in "<filename>"

    (<variable>, <pattern>)
}
```

## Types

JBS supports:

- `None`
- scalars: `int`, `float`, `str`, `bool`
- lists: `[1, 2, 3]`
- tuples: `(1, 2, 3)`; one-item tuples require a comma: `(1,)`
- dictionaries: `{"name": "case-a", 1: "one"}` or `dict(name = "case-a")`
- tables, created with `table(...)` or `t(...)`
- functions: `function(x) { x + 1 }`

### Scalars

Scalar values are the atomic values that can be used as workpackage shell variables. JBS supports `int`, `float`, `string`, and `bool` scalar values.

- Integers are 64-bit signed values.

  **Not supported** syntax: `1_000`, `0xff`, `0b1010`

- Floats are 64-bit floating-point values.

  `1.0`, `.5`, `1e-3`, and `2.5E6` are all supported forms.
  Print and REPL output render floats with the shortest decimal representation
  that round-trips to the same `float64`.

  **Not supported** syntax: `1.`

- Strings can use single or double quotes.

  Quote/backslash escapes: `"quote: \""`, `'quote: \''`, `"slash: \\"`.
  Unknown escapes are preserved literally; for example, `"\n"` remains backslash plus `n`.
  Strings can be appended: `"a" + "b" == "ab"`, and replicated: `"a" * 3 == "aaa"`.

- Booleans can be written as `true`, `True`, `TRUE`, `false`, `False`, `FALSE`.

  Booleans work with logical operators and comparisons: `true && (threads > 1) and !enabled`.
  Conjunction can be written as `&`, `&&`, or `and`. Disjunction can be written as `|`, `||`, or `or`.
  All spellings use JBS's eager, vector-aware logical semantics: both operands are evaluated, and list/tuple operands are combined element-wise. They are not short-circuit operators.

### Lists / tuples

Lists and tuples are ordered sequence values.

```jbs
xs = [1, 2, 3]      # list
ys = (1, 2, 3)      # tuple
one = (1,)          # one-item tuple; comma is required
empty_tuple = ()
empty_list = []
# lists and tuples can contain arbitrary JBS values
mixed = [1, "x", true, 1e-10]
```

Lists and tuples are similar sequence containers, but they differ in some vector arithmetic operations.

```jbs
jbs> [1, 2, 3] + 10
[11, 12, 13]
jbs> [1, 2, 3] * 2
[2, 4, 6]
jbs> [1, 2, 3] == 2
[false, true, false]
jbs> [1, 2, 3] + [10, 20, 30]
[11, 22, 33]
jbs> # cyclic broadcast rules apply; the shorter sequence is indexed by modulo length
jbs> [1, 2, 3] + [10]
[11, 12, 13]
jbs> [1, 2] + [10, 20, 30, 40]
[11, 22, 31, 42]
jbs> # if either side is empty, the result is an empty list
jbs> [] + [1, 2, 3]
[]
jbs> [1,2,3] == [1,2,4]
[true, true, false]
jbs> ![0, 1, ""]
[true, false, true]
```

Tuples behave differently for `+` and `*` operations:

```jbs
jbs> (1, 2) + (3, 4)
(1, 2, 3, 4)
jbs> ("a", "b") * 2
("a", "b", "a", "b")
```

Vector arithmetic operations for lists: `+`, `-`, `*`, `/`, `%`, `&`/`&&`/`and`, `|`/`||`/`or`, `!`.
Comparison operations: `==`, `!=`, `<`, `<=`, `>`, `>=`.

Useful functions:

- `len(...)` gets the number of elements
- `list` and `tuple` convert between sequence types
- `rev` reverses a list/tuple
```jbs
jbs> rev([1, 2, 3])
[3, 2, 1]
```
- `sample` randomly selects elements from a list/tuple
```jbs
jbs> sample([1], size = 3, replace = true)
[1, 1, 1]
```
- `filter` filters elements
```jbs
jbs> filter([0, 1, 2, 3], function(x) { x > 1 })
[2, 3]
```
- `range` generates a sequence
```jbs
jbs> range(5)
[0, 1, 2, 3, 4]
jbs> range(0,1,0.2)
[0, 0.2, 0.4, 0.6000000000000001, 0.8]
jbs> # `:` is an equivalent syntax
jbs> 5:1
[5, 4, 3, 2]
jbs> 0:2 * 4
[0, 4]
jbs> 0:10[0:10:2]
[0, 2, 4, 6, 8]
```

### Dictionaries

Dictionaries in JBS are ordered key-value maps. Dictionaries can store arbitrary JBS values:

```jbs
{
        "name": "case-a",
        "threads": 8,
        "flags": ["fast", "debug"],
        "meta": {"host": "node01"},
}
```

Dictionary keys must be hashable scalar values: `string`, `int`, or `bool`.

Use `{...}` or the `dict(...)` function syntax:

```jbs
jbs> name = "key"
# dict(...) named arguments always create string keys
jbs> dict(name = 1)
{"name": 1}
jbs> {name: 1}
{"key": 1}
```

Indexing syntax:

```jbs
jbs> d = dict(name = "case-a", threads = 8)
jbs> d["name"]
"case-a"
jbs> d["threads"]
8
jbs> # if the key is missing, JBS reports an error and returns null
jbs> get(d, "missing", "fallback")
"fallback"
```

Operations on dictionaries:

```jbs
jbs> base = dict(a = 1, b = 2)
jbs> # only dict + dict is valid
jbs> base + dict(b = 3, c = 4)
{"a": 1,
 "b": 3,
 "c": 4}
jbs> # the original dictionary is not modified
jbs> update(base, b = 3, c = 4)
{"a": 1,
 "b": 3,
 "c": 4}
```

Looping:
```jbs
jbs> d = dict(a = 1, b = 2)
jbs> # for loops over a dictionary iterate its keys in insertion order
jbs> keys = ()
jbs> for k in d {
...>         keys += (k,)
...> }
jbs> keys
("a", "b")
```

Conversion from `table` to `dict`:

```jbs
jbs> cases = table(x = [1, 2], y = ["a", "b"])
jbs> # each table column becomes a string key, and each dictionary value is a list of column values
jbs> d = dict(cases)
{"x": [1, 2],
 "y": ["a", "b"]}
```

A dictionary can be converted to a table:

```jbs
table(dict(x = [1, 2], y = ["a", "b"]))
```

- keys must be strings
- string keys must be valid shell variable names, such as `x`, `system_name`, or `_tmp`
- values may be scalars, lists, or tuples
- shorter non-empty columns are cyclically broadcast
- empty columns are allowed only if all columns are empty

### Tables

Tables are JBS's main parameter-space data type. A table is an ordered set of named columns, where each row represents one parameter combination. Because column names are exported into shell scripts, table column names must be valid shell variable names such as `x`, `system_name`, or `_tmp`.

The main constructor is `table(...)`, also available as `t(...)`.

```jbs
jbs> # column values can be scalars, lists, or tuples
jbs> # when columns have different non-empty lengths, JBS broadcasts shorter columns cyclically to the longest column
jbs> table(x = [1, 2], y = ("a", "b"), z = 0.1)
| x | y   | z   |
|---|-----|-----|
| 1 | "a" | 0.1 |
| 2 | "b" | 0.1 |
jbs> # tables can also be built from row dictionaries
jbs> cases = table([dict(x = 1, y = "a"), dict(x = 2, y = "b")])
jbs> cases
| x | y   |
|---|-----|
| 1 | "a" |
| 2 | "b" |
jbs> # `rows` converts a table to row dictionaries, and table(rows(...)) converts it back
jbs> rows(cases)
[{"x": 1, "y": "a"}, {"x": 2, "y": "b"}]
jbs> table(rows(cases))
| x | y   |
|---|-----|
| 1 | "a" |
| 2 | "b" |
jbs> # if a longer column length is not divisible by a shorter one, JBS emits a warning because the cycling is uneven
jbs> # warning: cyclic broadcast 3 -> 10
jbs> x = table(x = [1, 2, 3], y = range(10))
jbs> filter(x, function(x) {x["x"] == 1})
| x | y |
|---|---|
| 1 | 0 |
| 1 | 3 |
| 1 | 6 |
| 1 | 9 |
```

Reading CSV/TSV tables:

```jbs
# The first row is the header.
# Column names must be valid shell variable names.
# JBS infers column types as bool, int, float, or string.
cases = read_csv("cases.csv")
```

`+` and `*` operations, column access, and parameter-space slices:

```jbs
jbs> t(x = [1, 2]) + t(y = ["a", "b"]) # row-wise merge
| x | y   |
|---|-----|
| 1 | "a" |
| 2 | "b" |
jbs> x = range(5)
jbs> t(y = range(10)) + x # simple variables contribute their name as a column
| y | x |
|---|---|
| 0 | 0 |
| 1 | 1 |
| 2 | 2 |
| 3 | 3 |
| 4 | 4 |
| 5 | 0 |
| 6 | 1 |
| 7 | 2 |
| 8 | 3 |
... 1 more rows
jbs> t(x = [1]) * ([10, 20] as y) # anonymous values need an alias
| x | y  |
|---|----|
| 1 | 10 |
| 1 | 20 |
jbs> cases = t(x = [1, 2]) * t(y = [3, 4]) # Cartesian product
jbs> cases
| x | y |
|---|---|
| 1 | 3 |
| 1 | 4 |
| 2 | 3 |
| 2 | 4 |
jbs> cases["x"] # projection
| x |
|---|
| 1 |
| 2 |
jbs> cases.x
[1,1,2,2]
```

`filter(table_value, function)` keeps rows where the predicate function returns true.

```jbs
jbs> cases = table(id = [1, 2, 3], group = ["a", "b", "a"])
jbs> filter(cases, function(row) { row["group"] == "a" })
| id | group |
|----|-------|
| 1  | "a"   |
| 3  | "a"   |
```

Useful functions:

- `table`/`t` construct a table
- `read_csv(...)` import CSV/TSV as a table
- `table["col", ...]` parameter projection syntax
- `rename(table, {"old": "new"})` renames columns
- `a + b` row-wise merge in table context
- `a * b` Cartesian product in table context
- simple variable operands in table algebra contribute their variable name as a column
- anonymous non-table operands in table algebra require `as`, for example `table(x = [1]) * ([10, 20] as y)`
- `filter(table, function)` filter rows
- `head(table, n = 5)`/`tail(table, n = 5)` first/last rows
- `sample(table, size = n)` random rows
- `len(table)` row count
- `names(table)` column names
- `rows(table)` table rows as list of dictionaries

### Functions

JBS has two kinds of functions: user functions, defined with `function(...) { ... }`, and built-in functions, such as `map(...)` and `reduce(...)`.

#### User functions

Function values are first-class: they can be assigned, returned, passed to calls, stored in lists/tuples/dictionaries, and imported from modules.

```jbs
jbs> # parameters are comma-separated and may have defaults
jbs> add = function(a, b = 1) {
...>         # the last expression defines the result
...>         # the result can also be returned with `return a + b`
...>         a + b
...> }
jbs> add(2)
3
jbs> # positional and named arguments are allowed; positional arguments must come first
jbs> add(2, b = 3)
5
jbs> collect = function(prefix, *args, **kwargs) {
...>         [prefix, args, kwargs]
...> }
jbs> collect("run", *[1, 2], mode = "fast", **{"queue": "debug"})
["run", [1, 2], {"mode": "fast", "queue": "debug"}]
jbs> # top-level globals are captured live, so a function sees later reassignment of a global it reads
jbs> x = 1
jbs> f = function() {
        # local names shadow captured/global names
        x
}
jbs> f()
1
jbs> x=2
jbs> f()
2
jbs> x=1
jbs> # a default captures that selected value at function definition time
jbs> f = function(x=x) {x}
jbs> f()
1
jbs> x=2
jbs> f()
1
```

Defaults that refer to earlier parameters, such as `function(a, b = a + 1)`, are evaluated when the function is called.

Recursion is allowed. JBS has a maximum function-call depth guard to prevent runaway recursion. Note that JBS does not yet have tail-call optimization or memoization, so recursive calls are not very efficient.

```jbsrepl
jbs> factorial = function(n) {1 if 0 == n else n * factorial(n-1)}
jbs> factorial(5)
120
```

#### Built-In Functions

JBS provides several built-in functions. Use `?` for a full list.

Use `?<function_name>` in the REPL for focused help on a specific built-in function.

Built-in functions are function values too: they can be assigned to variables, passed to `map()` or `reduce()`, stored in containers, returned from functions, and imported from modules. Sequence folds such as `sum()` and `prod()` are ordinary built-ins as well. For example:

```jbs
values = map(int, ["1", "2"])
to_text = str
labels = map(to_text, (1, 2))
total = sum([1, 2, 3])
also_total = sum(1, 2, 3)
from_reduce = reduce(sum, [0, 1, 2, 3])
scaled = prod((2, 3, 4))
first_rows = head(table(id = range(10)))
last_rows = tail(table(id = range(10)), n = 3)
setseed(42)
random_rows = sample(table(id = range(10)), size = 2)
```

## Built-In Globals

- `jbs_name="jbs_benchmark"` defines the name of the benchmark directory.
- `jbs_benchmarks={}` splits one script into named benchmarks. Use `jbs --benchmark ...` to run individual benchmarks. In a non-empty `jbs_benchmarks` dictionary, target `"*"` selects every `do` step and every `analyse` block for that component.
- `jbs_nproc=0` sets the global concurrency limit. The default `0` uses all available CPUs.
- `jbs_database=""` uses CSV analysis output. A non-empty value enables SQLite database output.

Read more in [help_globals.md](help_globals.md).

## `for`, `while` loops

JBS supports `for` and `while` loops. `for` syntax:

```jbs
for name in iterable {
        ...
}
```

The iterable must be a list, tuple, or dictionary. Dictionaries iterate over their keys in insertion order. Scalars, strings, and tables are not directly iterable in `for`.

```jbs
jbs> sum = 0
jbs> for x in range(5) { sum += x }
jbs> sum
10
jbs> x
4
```

`while` syntax:

```jbs
while condition {
        ...
}
```

The condition is converted with the same truthiness rules as `bool(condition)`. For example, `0`, `""`, `None`, empty lists, empty tuples, empty dictionaries, and empty tables are false; non-empty containers and non-zero numbers are true.

```jbs
jbs> i = 0
jbs> while i < 3 {
...>         print(i)
...>         i += 1
...> }
0
1
2
jbs> i
3
```

`break` exits the nearest enclosing loop. `continue` skips to the next iteration.

Loops do not introduce a new scope. At the top level, variables assigned in the body are globals. In functions, variables assigned in the body are function locals. The loop target remains visible after the loop if the loop ran at least once. Inside loop bodies, `do`, `analyse`, and `use` declarations are not allowed. Loops are for computing values, not dynamically declaring benchmark steps.

## `if`/`else`

JBS has two conditional forms: statement conditionals and inline conditional expressions.

```jbs
if condition {
        ...
} elif other_condition {
        ...
} else {
        ...
}
```

`if` and `elif` conditions are converted with `bool(...)` truthiness. This means `if values { ... }` runs when `values` is a non-empty list, tuple, dictionary, or table.

```jbs
mode = "small"
if mode == "small" {
        cases = table(x = range(2))
} elif mode == "large" {
        cases = table(x = range(10))
} else {
        cases = table(x = range(1))
}
```

Branches are checked in order. The first true branch runs, the remaining branches are skipped. `else` is optional and runs only if no previous condition matched.

Inline conditionals are expressions:

```jbs
value_if_true if condition else value_if_false
```

Only the selected expression is evaluated after the condition is checked. This makes inline conditionals useful for choosing values directly in assignments or function returns.

There is no inline `elif`. Use nested conditional expressions for multiple cases:

```jbs
label = "small" if n < 10 else "medium" if n < 100 else "large"
```

## `use`: import other JBS files

`use` imports symbols from another `.jbs` module during JBS evaluation. It is an evaluation-time module-system feature, not a shell command.

The main supported forms are:

```jbs
use value from "./params.jbs"
use value, cases from "./params.jbs"
use "./params.jbs" as params
```

A selective import brings named globals directly into the current scope:

```jbs
# params.jbs
# cases = table(x = range(4))
# scale = function(x) { x * 2 }

use cases, scale from "./params.jbs"

do run with cases {
        echo "${x}"
}

values = map(scale, range(3))
```

A namespace import keeps the imported module under an alias:

```jbs
use "./params.jbs" as p

do run with p.cases {
        echo "${x}"
}

values = map(p.scale, range(3))
```

Selective imports import globals only: scalars, lists, tuples, dictionaries, tables, and functions defined as global variables. They do not selectively import `do` or `analyse` blocks.

Namespace imports bring the module's visible globals and step declarations under the alias. Imported `do` steps are prefixed with the alias, and their internal `after` and `with` references are prefixed consistently.

`with` imports can rename a single visible variable with `as`. For example, `with p.cases["very_long_column"] as short` expands rows from `p.cases["very_long_column"]`, but the generated workpackage exports only `short`. The alias is also the name visible to `fsub`, `analyse`, and dependent steps that inherit the variable through `after`.

## JBS's EBNF

```ebnf
program         = { sep | top_stmt } EOF ;

sep             = NEWLINE | ";" | COMMENT ;

top_stmt        = assignment
                | use_stmt
                | do_block
                | analyse_block
                | if_stmt
                | for_stmt
                | while_stmt
                | break_stmt
                | continue_stmt
                | expr_stmt ;

control_stmt    = assignment
                | if_stmt
                | for_stmt
                | while_stmt
                | break_stmt
                | continue_stmt
                | expr_stmt ;

block           = "{" { sep | control_stmt } "}" ;

assignment      = IDENT assign_op expr ;
assign_op       = "=" | "+=" | "-=" | "*=" | "/=" | "%=" ;

expr_stmt       = expr ;
break_stmt      = "break" ;
continue_stmt   = "continue" ;

use_stmt        = "use" ( path_import | namespace_import | selective_import ) ;
path_import     = STRING "as" IDENT ;
namespace_import = IDENT ;
selective_import = ident_list "from" use_source ;
use_source      = IDENT | STRING ;
ident_list      = IDENT { "," IDENT } ;

if_stmt         = "if" expr block { "elif" expr block } [ "else" block ] ;
for_stmt        = "for" IDENT "in" expr block ;
while_stmt      = "while" expr block ;

do_block        = "do" IDENT { do_header_clause } "{" raw_shell_body "}" ;
do_header_clause = after_clause | with_clause | nproc_clause | fsub_clause ;
after_clause    = "after" ident_list ;
with_clause     = "with" with_item { "," with_item } ;
with_item       = with_source [ "as" IDENT ] ;
with_source     = qualified_name [ "[" expr { "," expr } [ "," ] "]" ] ;
nproc_clause    = "nproc" signed_integer ;
fsub_clause     = "fsub" STRING "{" [ fsub_rule { "," fsub_rule } [ "," ] ] "}" ;
fsub_rule       = STRING ":" expr ;

analyse_block   = "analyse" IDENT { with_clause } "{" analyse_body "}" ;
analyse_body    = { sep | analyse_assign } analyse_result_tuple { sep } ;
regex_string    = "re" STRING ;
analyse_file    = STRING | regex_string ;
analyse_assign  = IDENT assign_op expr [ "in" analyse_file ] ;
analyse_result_tuple = "(" [ analyse_column { "," analyse_column } [ "," ] ] ")" ;
analyse_column  = analyse_named_column | analyse_inline_pattern ;
analyse_named_column = IDENT [ "." IDENT ] [ "as" STRING ] ;
analyse_inline_pattern = expr "in" analyse_file [ "as" STRING ] ;

expr            = conditional ;

conditional     = disjunction [ "if" disjunction "else" conditional ] ;
disjunction     = conjunction { "|" conjunction } ;
conjunction     = comparison { "&" comparison } ;
comparison      = sum [ comp_op sum ] ;
comp_op         = "==" | "!=" | "<" | ">" | "<=" | ">=" ;
sum             = product { ( "+" | "-" ) product } ;
product         = range_expr { ( "*" | "/" | "%" ) range_expr } ;
range_expr      = unary [ ":" range_bound [ ":" range_bound ] ] { postfix_suffix } ;
range_bound     = ( "+" | "-" | "!" ) range_bound
                | primary { "." IDENT | call | alias } ;
unary           = ( "+" | "-" | "!" ) unary | postfix ;

postfix         = primary { postfix_suffix } ;
postfix_suffix  = "." IDENT | call | index | alias ;
call            = "(" [ call_args [ "," ] ] ")" ;
call_args       = positional_args [ "," named_args ] | named_args ;
positional_args = expr { "," expr } ;
named_args      = named_arg { "," named_arg } ;
named_arg       = IDENT "=" expr ;
index           = "[" [ expr { "," expr } [ "," ] ] "]" ;
alias           = "as" IDENT ;

primary         = literal
                | IDENT
                | function_expr
                | group
                | tuple
                | list
                | dict ;

literal         = NUMBER | STRING | bool_literal ;
bool_literal    = "true" | "True" | "TRUE"
                | "false" | "False" | "FALSE" ;

group           = "(" expr ")" ;
tuple           = "(" ")"
                | "(" expr "," [ expr { "," expr } [ "," ] ] ")" ;
list            = "[" [ expr { "," expr } [ "," ] ] "]" ;
dict            = "{" [ dict_entry { "," dict_entry } [ "," ] ] "}" ;
dict_entry      = expr ":" expr ;

function_expr   = "function" "(" [ function_params [ "," ] ] ")" function_block ;
function_params = function_param { "," function_param } ;
function_param  = IDENT [ "=" expr ] ;

function_block  = "{" { sep | function_stmt } "}" ;
function_stmt   = local_assignment
                | return_stmt
                | func_if_stmt
                | func_for_stmt
                | func_while_stmt
                | break_stmt
                | continue_stmt
                | expr_stmt ;

local_assignment = IDENT assign_op expr ;
return_stmt      = "return" expr ;
func_if_stmt     = "if" expr function_block { "elif" expr function_block } [ "else" function_block ] ;
func_for_stmt    = "for" IDENT "in" expr function_block ;
func_while_stmt  = "while" expr function_block ;

qualified_name  = IDENT { "." IDENT } ;
signed_integer  = [ "+" | "-" ] DIGIT { DIGIT } ;

IDENT           = ( LETTER | "_" ) { LETTER | DIGIT | "_" } ;
NUMBER          = DIGIT { DIGIT } [ "." DIGIT { DIGIT } ] [ exponent ]
                | "." DIGIT { DIGIT } [ exponent ] ;
exponent        = ( "e" | "E" ) [ "+" | "-" ] DIGIT { DIGIT } ;
STRING          = single_quoted_string | double_quoted_string ;
COMMENT         = "#" { any_character_except_newline } ;
```
