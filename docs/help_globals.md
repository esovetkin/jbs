# jbs help globals

```jbs
# JBS global defaults

# Top-level assignments define all variables used by the script.
# Variables become visible in do/submit only when imported with `with`.
# Use comb(...) to create parameter-space objects.
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
```
