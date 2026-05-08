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
range(1,3) == [1,2]
range(0,10,2) == [0,2,4,6,8]
range(0,1,0.02) == [0,0.02,0.04,0.06,0.08]
```
