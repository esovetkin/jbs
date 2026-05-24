// defines types and what equality means
//
// ... the universe and everything (according to HoTT)
package eval

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"

	"gitlab.jsc.fz-juelich.de/sdlaml/jbs/internal/diag"
)

type Kind string

const (
	KindNull     Kind = "null"
	KindInt      Kind = "int"
	KindFloat    Kind = "float"
	KindString   Kind = "string"
	KindBool     Kind = "bool"
	KindList     Kind = "list"
	KindTuple    Kind = "tuple"
	KindDict     Kind = "dict"
	KindComb     Kind = "comb"
	KindFunction Kind = "function"
)

type DictKeyKind string

const (
	DictKeyString DictKeyKind = "string"
	DictKeyInt    DictKeyKind = "int"
	DictKeyBool   DictKeyKind = "bool"
)

type DictKey struct {
	Kind DictKeyKind
	S    string
	I    int64
	B    bool
}

type DictEntry struct {
	Key   DictKey
	Value Value
}

type Dict struct {
	Order   []DictKey
	Entries map[DictKey]Value
}

type Comb struct {
	Order []string
	Rows  []Row
}

type Value struct {
	Kind Kind
	I    int64
	F    float64
	S    string
	B    bool
	L    []Value
	C    *Comb
	Fn   *FunctionValue
	D    *Dict
}

func Null() Value           { return Value{Kind: KindNull} }
func Int(v int64) Value     { return Value{Kind: KindInt, I: v} }
func Float(v float64) Value { return Value{Kind: KindFloat, F: v} }
func String(v string) Value { return Value{Kind: KindString, S: v} }
func Bool(v bool) Value     { return Value{Kind: KindBool, B: v} }
func List(v []Value) Value  { return Value{Kind: KindList, L: v} }
func Tuple(v []Value) Value { return Value{Kind: KindTuple, L: v} }
func DictValue(entries []DictEntry) Value {
	d := &Dict{Entries: make(map[DictKey]Value)}
	for _, entry := range entries {
		d.Set(entry.Key, entry.Value)
	}
	return Value{Kind: KindDict, D: d}
}
func CombValue(v *Comb) Value {
	return Value{Kind: KindComb, C: v}
}
func Function(v *FunctionValue) Value { return Value{Kind: KindFunction, Fn: v} }

func (d *Dict) Set(key DictKey, value Value) {
	if d == nil {
		return
	}
	if d.Entries == nil {
		d.Entries = make(map[DictKey]Value)
	}
	if _, exists := d.Entries[key]; !exists {
		d.Order = append(d.Order, key)
	}
	d.Entries[key] = CloneValue(value)
}

func (key DictKey) StableString() string {
	switch key.Kind {
	case DictKeyString:
		return "s:" + strconv.Quote(key.S)
	case DictKeyInt:
		return "i:" + strconv.FormatInt(key.I, 10)
	case DictKeyBool:
		if key.B {
			return "b:true"
		}
		return "b:false"
	default:
		return "u:"
	}
}

func ValueFromDictKey(key DictKey) Value {
	switch key.Kind {
	case DictKeyString:
		return String(key.S)
	case DictKeyInt:
		return Int(key.I)
	case DictKeyBool:
		return Bool(key.B)
	default:
		return Null()
	}
}

func CloneValue(v Value) Value {
	switch v.Kind {
	case KindList:
		return List(CloneValues(v.L))
	case KindTuple:
		return Tuple(CloneValues(v.L))
	case KindDict:
		return cloneDictValue(v)
	case KindComb:
		if v.C == nil {
			return CombValue(nil)
		}
		return CombValue(&Comb{
			Order: append([]string(nil), v.C.Order...),
			Rows:  cloneRows(v.C.Rows),
		})
	default:
		return v
	}
}

func cloneDictValue(v Value) Value {
	if v.D == nil {
		return Value{Kind: KindDict}
	}
	out := &Dict{
		Order:   append([]DictKey(nil), v.D.Order...),
		Entries: make(map[DictKey]Value, len(v.D.Entries)),
	}
	for key, value := range v.D.Entries {
		out.Entries[key] = CloneValue(value)
	}
	return Value{Kind: KindDict, D: out}
}

func CloneValues(values []Value) []Value {
	if len(values) == 0 {
		return nil
	}
	out := make([]Value, len(values))
	for i, value := range values {
		out[i] = CloneValue(value)
	}
	return out
}

func IsTuple(v Value) bool {
	return v.Kind == KindTuple
}

func (v Value) IsScalar() bool {
	return v.Kind == KindInt || v.Kind == KindFloat || v.Kind == KindString || v.Kind == KindBool
}

func (v Value) String() string {
	switch v.Kind {
	case KindNull:
		return ""
	case KindInt:
		return fmt.Sprintf("%d", v.I)
	case KindFloat:
		return trimFloat(v.F)
	case KindString:
		return v.S
	case KindBool:
		if v.B {
			return "true"
		}
		return "false"
	case KindList:
		parts := make([]string, 0, len(v.L))
		for _, x := range v.L {
			parts = append(parts, x.String())
		}
		return "[" + strings.Join(parts, ",") + "]"
	case KindTuple:
		parts := make([]string, 0, len(v.L))
		for _, x := range v.L {
			parts = append(parts, x.String())
		}
		return "(" + strings.Join(parts, ",") + ")"
	case KindDict:
		return dictString(v.D)
	case KindComb:
		if v.C == nil {
			return "table()"
		}
		return fmt.Sprintf("table(rows=%d,cols=%d)", len(v.C.Rows), len(v.C.Order))
	case KindFunction:
		return "<function>"
	default:
		return ""
	}
}

func trimFloat(f float64) string {
	if math.Trunc(f) == f {
		return fmt.Sprintf("%.1f", f)
	}
	return fmt.Sprintf("%g", f)
}

func Equal(a, b Value) bool {
	if a.Kind != b.Kind {
		if isNumeric(a) && isNumeric(b) {
			return toFloat(a) == toFloat(b)
		}
		return false
	}
	switch a.Kind {
	case KindInt:
		return a.I == b.I
	case KindFloat:
		return a.F == b.F
	case KindString:
		return a.S == b.S
	case KindBool:
		return a.B == b.B
	case KindList, KindTuple:
		if len(a.L) != len(b.L) {
			return false
		}
		for i := range a.L {
			if !Equal(a.L[i], b.L[i]) {
				return false
			}
		}
		return true
	case KindDict:
		return equalDicts(a.D, b.D)
	case KindComb:
		if (a.C == nil) != (b.C == nil) {
			return false
		}
		if a.C == nil && b.C == nil {
			return true
		}
		if len(a.C.Order) != len(b.C.Order) {
			return false
		}
		for i := range a.C.Order {
			if a.C.Order[i] != b.C.Order[i] {
				return false
			}
		}
		if len(a.C.Rows) != len(b.C.Rows) {
			return false
		}
		for i := range a.C.Rows {
			ar := a.C.Rows[i]
			br := b.C.Rows[i]
			if len(ar.Values) != len(br.Values) {
				return false
			}
			for k, ac := range ar.Values {
				bc, ok := br.Values[k]
				if !ok {
					return false
				}
				if !Equal(ac.Value, bc.Value) {
					return false
				}
			}
		}
		return true
	case KindFunction:
		return a.Fn == b.Fn
	default:
		return true
	}
}

func equalDicts(a, b *Dict) bool {
	if dictLen(a) != dictLen(b) {
		return false
	}
	if dictLen(a) == 0 {
		return true
	}
	for key, av := range a.Entries {
		bv, ok := b.Entries[key]
		if !ok {
			return false
		}
		if !Equal(av, bv) {
			return false
		}
	}
	return true
}

func dictLen(d *Dict) int {
	if d == nil {
		return 0
	}
	return len(d.Entries)
}

func isNumeric(v Value) bool {
	return v.Kind == KindInt || v.Kind == KindFloat
}

func toFloat(v Value) float64 {
	if v.Kind == KindFloat {
		return v.F
	}
	if v.Kind == KindInt {
		return float64(v.I)
	}
	return 0
}

func ToSeries(v Value) []Value {
	if v.Kind == KindList || v.Kind == KindTuple {
		out := make([]Value, len(v.L))
		copy(out, v.L)
		return out
	}
	return []Value{v}
}

func IterableElements(v Value, at diag.Span, diags *diag.Diagnostics) ([]Value, bool) {
	switch v.Kind {
	case KindList, KindTuple:
		out := make([]Value, len(v.L))
		copy(out, v.L)
		return out, true
	case KindDict:
		if v.D == nil || len(v.D.Order) == 0 {
			return nil, true
		}
		out := make([]Value, 0, len(v.D.Order))
		for _, key := range v.D.Order {
			if _, ok := v.D.Entries[key]; ok {
				out = append(out, ValueFromDictKey(key))
			}
		}
		return out, true
	default:
		diags.AddError(
			diag.CodeE106,
			"for loop expects list, tuple, or dictionary value",
			at,
			"use range(...), list(...), tuple(...), a list/tuple expression, or a dictionary",
		)
		return nil, false
	}
}

func dictString(d *Dict) string {
	if d == nil || len(d.Entries) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(d.Order))
	for _, key := range d.Order {
		value, ok := d.Entries[key]
		if !ok {
			continue
		}
		parts = append(parts, dictKeyString(key)+":"+value.String())
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func dictKeyString(key DictKey) string {
	switch key.Kind {
	case DictKeyString:
		if simpleDictStringKey(key.S) {
			return key.S
		}
		return strconv.Quote(key.S)
	case DictKeyInt:
		return strconv.FormatInt(key.I, 10)
	case DictKeyBool:
		if key.B {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func simpleDictStringKey(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 {
			if r != '_' && !unicode.IsLetter(r) {
				return false
			}
			continue
		}
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}
