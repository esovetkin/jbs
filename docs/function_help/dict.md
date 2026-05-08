# `dict(...)`

## Arguments

- `key = value, ...`: zero or more named arguments.
- Argument names become string keys.
- Values may be any JBS value.

Literal syntax is also available. Literal keys are expressions and must evaluate to string, int, or bool.

## Returns

One dictionary value.

## Example

```jbs
d = dict(name = "case-a", threads = 8)
same = {"name": "case-a", 1: "one", true: "enabled"}
d["name"] == "case-a"
```

Duplicate keys are allowed; the last value wins.
