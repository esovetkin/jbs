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

# format in place
jbs fmt input.jbs

# show help
jbs
jbs help

# list built-in globals and mapping
jbs help globals
```

`jbs fmt` rewrites the target file in place using canonical JBS layout.

## Formatter

Canonical formatter behavior:

- opening `{` on its own line
- `after` / `with` clauses on dedicated continuation lines
- block body indentation normalized to 8 spaces
- one blank line between top-level statements
- output always ends with a newline

Example:

```jbs
# before
do task with a from p{echo ${a}}

# after
do task
        with a from p
{
        echo ${a}
}
```

## Top-Level Globals

You can assign known globals outside `param`, `do`, and `submit` blocks:

```jbs
jbs_name = "demo"
jbs_outpath = "test"
```

Rules:

- only known globals are allowed (`jbs help globals`)
- unknown names are compile errors
- `jbs_name` and `jbs_outpath` must be plain string literals

## Language Blocks

- `param <name> ... { ... }`
- `do <name> ... { ... }`
- `submit <name> ... { key = value ... }`
- `patterns <group> { name = "regex-with-%d/%f/%w" ... }`
- `analyse <step> { alias = group.name in "file" ... (columns...) }`

`with` imports support:

- full parametersets: `with p1, p2`
- variable imports: `with a from p1`
- tuple imports: `with (a,b) from p1`
- mixed form: `with a from p1, p2` and `with (a,b) from p1, p2`

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
- `E036`: the same identifier is used twice in one combination expression (`A + A`, `(A+B)*A`).
- `E042`: two rows merged by `+`/`*` provide different values for the same key.
- `E072`-`E076`: invalid submit key/value syntax and structure in `submit` blocks.
- `W101`: `+` zipped lists of different lengths; cyclic broadcast to max length was applied.
- `W310`: exposed param variable is never referenced in any `do`/`submit` body (`$name`/`${name}`).
- `W311`: step references a known param variable in body text but does not import it via `with`.

Warnings are non-fatal and do not cause `jbs --check` to fail.

## Known Limitations (V1)

- YAML output only.
- No XML output.

## License

GPL-3.0. See [LICENSE](LICENSE).
