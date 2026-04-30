# `range(...)`

## Arguments

- `range(stop)`: integer exclusive upper bound.
- `range(start, stop)`: integer lower bound and exclusive upper bound.
- `range(start, stop, step)`: numeric bounds and positive numeric step.
- One-argument and two-argument forms require integers.
- `range(...)` is only allowed in top-level global assignment expressions.

## Returns

A list of numbers from `start` inclusive to `stop` exclusive.

## Example

```jbs
range(3) == [0,1,2]
range(0, 1, 0.25) == [0.0,0.25,0.5,0.75]
```
