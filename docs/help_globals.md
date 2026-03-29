# jbs help globals

```jbs
# JBS global defaults

# Top-level assignments are allowed only outside param/do/submit blocks.
# Unknown globals cause compile errors.
# jbs_name and jbs_outpath must be plain string literals.

# Benchmark name (root `name` field). maps_to: root:name. mode: -
jbs_name = "jbs_benchmark"

# Benchmark output path (root `outpath` field). maps_to: root:outpath. mode: -
jbs_outpath = "out"
```
