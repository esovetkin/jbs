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

# list built-in jbs_* globals and mapping
jbs
```

## Language Blocks

- `param <name> ... { ... }`
- `do <name> ... { ... }`
- `submit <name> ... { ... } { ... }`

See [docs/language.md](docs/language.md) for full grammar and semantics.

## Troubleshooting

- `E036`: the same identifier is used twice in one combination expression (`A + A`, `(A+B)*A`).
- `E042`: two rows merged by `+`/`*` provide different values for the same key.
- `E053`: value contains reserved separator `####`.
- `W101`: `+` zipped lists of different lengths; cyclic broadcast to max length was applied.

## Known Limitations (V1)

- YAML output only.
- No XML output.
- `analyser`, `patternset`, and `result` are intentionally not auto-generated.
