# jbs help globals

```jbs
# JBS global defaults

# Top-level assignments are allowed only outside `param`, `do`, and `submit` blocks.
# Unknown globals are compile-time errors.
# jbs_name and jbs_outpath must be plain string literals.
# Statements can be separated by a newline or ';'.
# Multiline expressions require explicit backslash-newline continuation.
# Implicit operator-based newline continuation is not supported.

# Benchmark name (root `name` field). maps_to: root:name. mode: -
jbs_name = "jbs_benchmark"

# Benchmark output path (root `outpath` field). maps_to: root:outpath. mode: -
jbs_outpath = "out"

# Benchmark comment (root `comment` field). maps_to: root:comment. mode: -
jbs_comment = ""

# Example multiline global assignment:
# jbs_name = "bench_" + \
#            "v1"
```
