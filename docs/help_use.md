# jbs help use

`use` imports values and step declarations from another `.jbs` file.

## Forms

```jbs
use value from "./params.jbs"
use value, cases from "./params.jbs"
use "./params.jbs" as params
```

Selective imports add the named values directly to the current scope. Namespaced imports keep the imported symbols under the alias.

```jbs
use "./params.jbs" as p

do run with p.cases {
        echo "${x}"
}
```

## Step Imports

Importing a `do` step also imports the `after` dependencies needed by that step.

```jbs
use run from "./steps.jbs"
```

If `run` depends on `prepare`, both declarations are available to the current benchmark.

## Data Imports

Imported tables can be used in `with` clauses:

```jbs
use cases from "./params.jbs"

do run with cases["x", "label"] {
        echo "${x} ${label}"
}
```

Functions and scalars can be imported for expressions:

```jbs
use scale from "./math.jbs"

values = table(x = map(scale, range(4)))
```

Function-valued globals are not valid `with` sources because `with` expects row data.

## Paths

Import paths are quoted file paths. Relative paths are resolved from the importing file's directory.
