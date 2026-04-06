# Diagnostics Catalog

JBS diagnostics are centrally defined in [`internal/diag/codes.go`](../internal/diag/codes.go).

All compiler stages emit diagnostics through `diag.Code` constants, and all codes must be registered in the central catalog.

## Format

- Error codes: `E###`
- Warning codes: `W###`

## Owners

- `lexer`: tokenization errors
- `parser`: syntax and block-shape errors
- `eval`: expression/combination evaluation diagnostics
- `sema`: semantic validation diagnostics
- `lower`: YAML lowering diagnostics
- `printparam`: printparam expansion diagnostics
- `imports`: module/use import diagnostics

## Collision Remap

The following collisions were split into unique codes:

| Previous code | Old meaning | New code |
|---|---|---|
| `E022` | parser dependency-list parse error | `E028` |
| `E022` | sema ambiguous `with` source (param vs let) | `E218` |
| `E216` | sema invalid `max_async` | `E216` (unchanged) |
| `E216` | lower mode value variance across indexed rows | `E231` |
| `E020` | parser param name parse error | `E082` |
| `E021` | parser param body `{` parse error | `E083` |
| `E076` | parser malformed submit statement parse error | `E077` |

## Source of Truth

For the full per-code registry (severity, owner, summary), use:

- [`internal/diag/codes.go`](../internal/diag/codes.go)

The test suite enforces:

- catalog well-formedness (`^[EW][0-9]{3}$` + severity consistency)
- no raw `"E###"`/`"W###"` literals in `AddError`/`AddWarning` calls outside `internal/diag`
