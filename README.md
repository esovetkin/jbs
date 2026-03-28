# jbs

Standalone compiler from `.jbs` scripts to JUBE-compatible YAML.

## Build

```bash
go build ./cmd/jbs
```

## Test

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./...
```

## CLI

```bash
# compile to stdout (default)
jbs input.jbs

# compile to file
jbs input.jbs -o JUBE.yaml

# parse + semantic check only
jbs --check input.jbs

# show help
jbs
jbs help

# list built-in jbs_* globals and mapping
jbs help globals
```

## Top-Level Globals

You can assign known globals outside `param`, `do`, and `submit` blocks:

```jbs
jbs_name = "demo"
jbs_outpath = "test"
jbs_queue = python("__import__('os').environ.get('JUBE_QUEUE', 'devel')")
```

Rules:

- only known globals are allowed (`jbs help globals`)
- unknown names are compile errors
- `jbs_name` and `jbs_outpath` must be plain string literals
- other globals accept scalar values or `shell("...")` / `python("...")`

## Language Blocks

- `param <name> ... { ... }`
- `do <name> ... { ... }`
- `submit <name> ... { ... } { ... }`

See [docs/language.md](docs/language.md) for full grammar and semantics.

## Generated YAML Comments

Generated YAML includes explanatory comments for major sections and synthetic blocks, for example:

```yaml
# Parameter sets used to create workpackage combinations
parameterset:
  # Synthetic submit parameters for submit block 'run' (init_with: platform.xml:systemParameter).
  - name: run__submit_params
    init_with: platform.xml:systemParameter
```

## Troubleshooting

- `E300`: unknown top-level global variable.
- `E301`/`E302`: `jbs_name` / `jbs_outpath` must be plain string literals.
- `E303`: `jbs_name` / `jbs_outpath` cannot use `shell()` / `python()`.
- `E304`: top-level global value must be scalar (no tuple/list/dict).
- `E036`: the same identifier is used twice in one combination expression (`A + A`, `(A+B)*A`).
- `E042`: two rows merged by `+`/`*` provide different values for the same key.
- `E053`: value contains reserved separator `####`.
- `W101`: `+` zipped lists of different lengths; cyclic broadcast to max length was applied.

## Known Limitations (V1)

- YAML output only.
- No XML output.
- `analyser`, `patternset`, and `result` are intentionally not auto-generated.

## License

GPL-3.0. See [LICENSE](LICENSE).
