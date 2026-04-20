# Why Write Benchmarks in JBS Instead of Raw JUBE YAML

JBS is designed to make JUBE benchmark configurations shorter, easier to reason about, and safer.

## 1) JBS is much more compact

The table below compares all paired files in `tests/` using `wc -l -w -c` (lines, words, bytes), excluding empty lines and comment lines (`#...`) in both `.jbs` and `.yaml` files.

| Base                             |                jbs |                yaml | char reduction |
|----------------------------------|-------------------:|--------------------:|---------------:|
| `after_inherit_product_expand`   |        `11 36 134` |       `55 116 1117` |        `88.0%` |
| `after_inherit_transitive_chain` |        `15 55 219` |       `78 167 1658` |        `86.8%` |
| `after_inherit_zip_preserve`     |        `11 34 127` |       `55 116 1085` |        `88.3%` |
| `backslash_continuation`         |        `12 34 180` |         `22 45 373` |        `51.7%` |
| `basic`                          |        `14 46 270` |       `73 170 1832` |        `85.3%` |
| `fmt_basic`                      |        `20 46 305` |         `26 56 450` |        `32.2%` |
| `fmt_clauses`                    |        `20 33 170` |         `22 37 244` |        `30.3%` |
| `fmt_results`                    |        `12 30 138` |         `19 35 199` |        `30.7%` |
| `results_basic`                  |        `27 79 389` |       `63 144 1293` |        `69.9%` |
| `semicolon`                      |        `14 67 337` |       `55 120 1080` |        `68.8%` |
| **total**                        | **`156 460 2269`** | **`468 1006 9331`** |    **`75.7%`** |

Across these tests, JBS uses about 67% fewer lines and about 76% fewer bytes.

## 2) Results definition is local in JBS

In raw JUBE YAML, result extraction usually requires three connected sections:
- `patternset`
- `analyser`
- `result`

You must keep names aligned across distant sections.

Example (raw JUBE YAML style): to extract **one** matched value, you must define `p`, `runtime`, `ana_run`, and `result_run`, then keep all names aligned in the result table.

```yaml
patternset:
  - name: p
    pattern:
      - name: runtime
        type: float
        _: 'Runtime: $jube_pat_fp'

analyser:
  - name: ana_run
    use: p
    analyse:
      - step: run
        file:
          - use: p
            _: job.out

result:
  use:
    - ana_run
  table:
    - name: result_run
      style: csv
      column:
        - title: runtime_s
          _: runtime
```

The same intent in JBS is one local block:

```jbs
analyse run {
        t = "Runtime: %f" in "job.out"
        (t as "runtime_s")
}
```

This reduces cross-section name wiring and keeps extraction logic in one place.

## 3) Complex parameter logic and dependent slices are hard in YAML

A simple Cartesian product is easy in YAML. The real difficulty starts when you need mixed algebra (`+` and `*`), step dependencies, and different variable slices in each step.

Example JBS parameter logic:

```jbs
a = (1,2)
b = ("b0","b1","b2")
c = ("c0","c1","c2")
d = (true,false)
pm = comb(a * (b + c) * d)

do step0 with (a,b) from pm {
  echo "a=${a} b=${b}" > s0.out
}

do step1 after step0 with (c,d) from pm {
  echo "a=${a} b=${b} c=${c} d=${d}" > s1.out
}
```

JBS compiles this to YAML with index tracking and subset propagation so `after` preserves row semantics instead of accidentally exploding combinations.

Excerpt from generated YAML:

```yaml
  # Synthetic subset parameterset for step 'step0' derived from 'pm' for variable-only imports
  - name: _js__step0__pm__a_b
    parameter:
      - name: _ji__step0__pm__a_b
        type: int
        mode: text
        _: 0,2,4,6,8,10
      # Internal helper: grouped source row IDs stay opaque with separator #### for after-step narrowing
      - name: _jr__step0__pm__a_b
        mode: python
        separator: '####'
        _: '{"0":"0,1","2":"2,3","4":"4,5","6":"6,7","8":"8,9","10":"10,11"}["${_ji__step0__pm__a_b}"]'
      - name: a
        mode: python
        _: '[1,1,1,1,1,1,2,2,2,2,2,2][$_ji__step0__pm__a_b]'
      - name: b
        mode: python
        _: '["b0","b0","b1","b1","b2","b2","b0","b0","b1","b1","b2","b2"][$_ji__step0__pm__a_b]'

  # Synthetic subset parameterset for step 'step1' derived from 'pm' for variable-only imports
  - name: _js__step1__pm__c_d
    parameter:
      - name: _ji__step1__pm__c_d
        type: int
        mode: text
        separator: ','
        _: $_jr__step0__pm__a_b
      - name: _jr__step1__pm__c_d
        mode: text
        _: ${_ji__step1__pm__c_d}
      - name: c
        mode: python
        _: '["c0","c0","c1","c1","c2","c2","c0","c0","c1","c1","c2","c2"][$_ji__step1__pm__c_d]'
      - name: d
        mode: python
        _: '[True,False,True,False,True,False,True,False,True,False,True,False][$_ji__step1__pm__c_d]'
```

Writing this wiring by hand in raw YAML is possible, but it is much harder to get right and maintain. In JBS, this is a single local formula, and you can run `jbs printparam` to verify that the produced values match your expectations.

## 4) YAML syntax pitfalls are easy to miss

A common pitfall is block-scalar syntax.

For example, I made this mistake while copying and pasting:

```yaml
separator: '|'
_: |
```

This can expand in an unexpected way. In this case, the mistake was mine. The `separator` was not needed at all. However, the following also achieves the intended result:

```yaml
separator: |
_: |
```

The worst part is that there are no warnings for these small mistakes, even though they can break your job runs.

![](logo_facepalm.png)

## 5) JBS adds static checks and warnings before launch

JBS catches issues early.

### Circular dependency

```jbs
do a after b { echo a }
do b after a { echo b }
```

Diagnostic:

```text
ERROR E213
dependency cycle detected: a -> b -> a
```

### Missing import for used variable

```jbs
a = (1,2)
do s { echo ${a} }
```

Diagnostic:

```text
WARNING W311
variable 'a' is referenced in step 's' but not imported via with-clause
```

### Exposed imported variable never used

```jbs
p_a = (1,2)
p_b = ("x","y")
p = comb(p_a + p_b)
do s with a from p { echo ${a} }
```

Diagnostic:

```text
WARNING W310
exposed variable 'b' from global 'p' is never used in any do/submit/analyse block
```

These checks prevent many mistakes before `jube-autorun` starts creating workpackages or submitting jobs.

## Summary

JBS improves benchmark authoring in three ways:
- Less code to write and review
- Local, readable declarations for analysis and step logic
- Stronger compile-time checks for common job configuration mistakes

![](logo.png "Go gopher kicking JUBE into exascale")
