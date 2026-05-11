package eval

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

type StableNamedValuePart struct {
	Name  string
	Value Value
}

// StableValueKey returns a deterministic key for a JBS value within this process.
func StableValueKey(v Value) string {
	var b strings.Builder
	appendStableValueKey(&b, v)
	return b.String()
}

// StableValueTupleKey returns a deterministic key for an ordered value tuple within this process.
func StableValueTupleKey(values []Value) string {
	var b strings.Builder
	b.WriteByte('R')
	b.WriteString(strconv.Itoa(len(values)))
	b.WriteByte('[')
	for _, value := range values {
		appendKeyPart(&b, StableValueKey(value))
	}
	b.WriteByte(']')
	return b.String()
}

// StableNamedValueTupleKey returns a deterministic key for named ordered values.
func StableNamedValueTupleKey(parts []StableNamedValuePart) string {
	var b strings.Builder
	b.WriteByte('M')
	b.WriteString(strconv.Itoa(len(parts)))
	b.WriteByte('[')
	for _, part := range parts {
		appendKeyPart(&b, part.Name)
		appendKeyPart(&b, StableValueKey(part.Value))
	}
	b.WriteByte(']')
	return b.String()
}

func appendStableValueKey(b *strings.Builder, v Value) {
	switch v.Kind {
	case KindNull:
		b.WriteByte('N')
	case KindInt:
		b.WriteByte('I')
		appendKeyPart(b, strconv.FormatInt(v.I, 10))
	case KindFloat:
		b.WriteByte('F')
		appendKeyPart(b, strconv.FormatFloat(v.F, 'g', -1, 64))
	case KindString:
		b.WriteByte('S')
		appendKeyPart(b, v.S)
	case KindBool:
		if v.B {
			b.WriteString("B1")
		} else {
			b.WriteString("B0")
		}
	case KindList:
		appendStableSequenceKey(b, 'L', v.L)
	case KindTuple:
		appendStableSequenceKey(b, 'T', v.L)
	case KindDict:
		appendStableDictKey(b, v.D)
	case KindComb:
		appendStableCombKey(b, v.C)
	case KindFunction:
		b.WriteByte('X')
		appendKeyPart(b, fmt.Sprintf("%p", v.Fn))
	default:
		b.WriteByte('U')
		appendKeyPart(b, string(v.Kind))
	}
}

func appendStableSequenceKey(b *strings.Builder, tag byte, values []Value) {
	b.WriteByte(tag)
	b.WriteString(strconv.Itoa(len(values)))
	b.WriteByte('[')
	for _, value := range values {
		appendKeyPart(b, StableValueKey(value))
	}
	b.WriteByte(']')
}

type stableDictKeyPart struct {
	key   string
	value string
}

func appendStableDictKey(b *strings.Builder, d *Dict) {
	if d == nil || len(d.Entries) == 0 {
		b.WriteString("D0{}")
		return
	}
	parts := make([]stableDictKeyPart, 0, len(d.Entries))
	for key, value := range d.Entries {
		parts = append(parts, stableDictKeyPart{
			key:   stableDictKeyKey(key),
			value: StableValueKey(value),
		})
	}
	slices.SortFunc(parts, func(a, b stableDictKeyPart) int {
		return strings.Compare(a.key, b.key)
	})
	b.WriteByte('D')
	b.WriteString(strconv.Itoa(len(parts)))
	b.WriteByte('{')
	for _, part := range parts {
		appendKeyPart(b, part.key)
		appendKeyPart(b, part.value)
	}
	b.WriteByte('}')
}

func stableDictKeyKey(key DictKey) string {
	var b strings.Builder
	switch key.Kind {
	case DictKeyString:
		b.WriteByte('S')
		appendKeyPart(&b, key.S)
	case DictKeyInt:
		b.WriteByte('I')
		appendKeyPart(&b, strconv.FormatInt(key.I, 10))
	case DictKeyBool:
		if key.B {
			b.WriteString("B1")
		} else {
			b.WriteString("B0")
		}
	default:
		b.WriteByte('U')
		appendKeyPart(&b, string(key.Kind))
	}
	return b.String()
}

func appendStableCombKey(b *strings.Builder, c *Comb) {
	b.WriteByte('C')
	if c == nil {
		b.WriteByte('0')
		return
	}
	b.WriteByte('1')
	appendKeyPart(b, strconv.Itoa(len(c.Order)))
	for _, col := range c.Order {
		appendKeyPart(b, col)
	}
	appendKeyPart(b, strconv.Itoa(len(c.Rows)))
	for _, row := range c.Rows {
		names := make([]string, 0, len(row.Values))
		for name := range row.Values {
			names = append(names, name)
		}
		slices.Sort(names)
		appendKeyPart(b, strconv.Itoa(len(names)))
		for _, name := range names {
			appendKeyPart(b, name)
			appendKeyPart(b, StableValueKey(row.Values[name].Value))
		}
	}
}

func appendKeyPart(b *strings.Builder, s string) {
	b.WriteString(strconv.Itoa(len(s)))
	b.WriteByte(':')
	b.WriteString(s)
}
