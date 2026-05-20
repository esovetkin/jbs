# `setseed(seed)`

Set the random generator seed used by `sample(...)`.

## Arguments

- `seed`: integer seed.

## Returns

`None`.

## Example

```jbs
setseed(42)
a = sample(range(10), size = 3)

setseed(42)
b = sample(range(10), size = 3)

all(a == b)
```
