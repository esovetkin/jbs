# `env(...)`

Reads environment variables from the running `jbs` process.

## Arguments

- `env()` takes no arguments.
- `env(name)` takes a string environment variable name.
- `env(name, default_value)` takes a string name and a fallback value.
- Named form: `env(name = "NAME", default = fallback)`.

## Returns

- `env()` returns a dictionary of environment variables.
- `env(name)` returns the variable value as a string, or `None`.
- `env(name, default_value)` returns the variable value as a string, or the fallback value.

## Example

```jbs
threads = int(env("THREADS", "4"))
home = env(name = "HOME", default = "")
all_env = env()
home_from_dict = get(all_env, "HOME", "")
```

`env(...)` reads the environment of the running `jbs` process.
