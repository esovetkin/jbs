# jbs help globals

Built-in globals:

```jbs
# Benchmark directory name.
jbs_name = "jbs_benchmark"

# Global concurrency limit. 0 uses available CPU count.
jbs_nproc = 0
```

User globals can hold scalars, lists, tuples, tables, and functions. `do` and `analyse` blocks see the globals visible at the point where the block is declared.
