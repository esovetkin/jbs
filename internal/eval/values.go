package eval

import (
	"fmt"
	"math"
	"strings"
)

type Kind string

const (
	KindNull   Kind = "null"
	KindInt    Kind = "int"
	KindFloat  Kind = "float"
	KindString Kind = "string"
	KindBool   Kind = "bool"
	KindList   Kind = "list"
	KindTuple  Kind = "tuple"
)

type Value struct {
	Kind Kind
	I    int64
	F    float64
	S    string
	B    bool
	L    []Value
}

func Null() Value           { return Value{Kind: KindNull} }
func Int(v int64) Value     { return Value{Kind: KindInt, I: v} }
func Float(v float64) Value { return Value{Kind: KindFloat, F: v} }
func String(v string) Value { return Value{Kind: KindString, S: v} }
func Bool(v bool) Value     { return Value{Kind: KindBool, B: v} }
func List(v []Value) Value  { return Value{Kind: KindList, L: v} }
func Tuple(v []Value) Value { return Value{Kind: KindTuple, L: v} }

func IsTuple(v Value) bool {
	return v.Kind == KindTuple
}

func (v Value) IsScalar() bool {
	return v.Kind == KindInt || v.Kind == KindFloat || v.Kind == KindString || v.Kind == KindBool
}

func (v Value) String() string {
	switch v.Kind {
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
	default:
		return true
	}
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
