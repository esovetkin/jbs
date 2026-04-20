# jbs help globals

```jbs
# Canonical top-level forms:
# - use
# - top-level assignment
# - top-level expression statement
# - do
# - submit
# - analyse
#
# Globals are introduced only by top-level assignments.
# Top-level bindings are immutable:
# - use plain '='
# - define each top-level name once
# - introduce a new name instead of rebinding or using += / -= / *= / /= / %=
#
# A global may hold scalar data, tuple/list data, table data,
# or a function value.

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

# Ordinary globals:
sizes = (1, 2, 4)
labels = ("small", "medium", "large")
cases = table(label = labels, size = sizes)

# Preferred derived-value style:
seed0 = 1
seed1 = seed0 + 1
seed2 = seed1 + 1
```
